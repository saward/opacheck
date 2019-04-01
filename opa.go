package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"reflect"
	"sync"

	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/loader"
	"github.com/open-policy-agent/opa/rego"
	"github.com/open-policy-agent/opa/storage"
	"github.com/open-policy-agent/opa/storage/inmem"
	opentracing "github.com/opentracing/opentracing-go"
)

var unsafeCompiler = ast.NewCompiler()
var unsafeDocuments = map[string]interface{}{}
var unsafeStore storage.Store
var mutex = &sync.RWMutex{}
var dMutex = &sync.RWMutex{}

// Authorised Returns a simple true/false answer to the question of whether or not the item is authorised.  If policy does not exist, it returns false and no error, but logs it
func Authorised(ctx context.Context, policy string, data map[string]interface{}) (bool, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "Authorised")
	defer span.Finish()

	var allowed bool

	// Call the policy, and get our response
	rs, err := runRego(ctx, policy, data)

	if err != nil {
		return allowed, err
	}

	// Explicitly convert to array of interfaces, and all of those interfaces should be strings though we cannot cast directly to []string
	var ok bool

	// No such policy, but we just count that as false?
	if (len(rs) < 1) || (len(rs[0].Expressions) < 1) {
		log.Printf("No such policy %s", policy)
		return false, nil
	}
	allowed, ok = rs[0].Expressions[0].Value.(bool)

	if !ok {
		return allowed, fmt.Errorf("Could not authorise action.  Return type: %s", reflect.TypeOf(rs[0].Expressions[0].Value))
	}

	return allowed, nil
}

// GetCompiler Returns compiler object in thread-safe manner since we sometimes update the compiler in a separate thread
func GetCompiler(ctx context.Context) *ast.Compiler {
	span, ctx := opentracing.StartSpanFromContext(ctx, "GetCompiler")
	defer span.Finish()

	var copy *ast.Compiler

	mutex.RLock()
	copy = unsafeCompiler
	mutex.RUnlock()
	return copy
}

func LoadBundle(path string) error {
	return loadCompiler(path)
}

func loadCompiler(path string) error {
	log.Printf("Loading path %s", path)
	newCompiler := ast.NewCompiler()

	pwd, err := os.Getwd()
	if err != nil {
		return err
	}
	defer os.Chdir(pwd)

	log.Printf("Current dir: %s", pwd)
	err = os.Chdir(path)
	if err != nil {
		return err
	}

	result, err := loader.Filtered([]string{"."}, nil)
	if err != nil {
		return fmt.Errorf("Error loading all path: %s", err)
	}

	// Create map from all values for compiling:
	modules := make(map[string]*ast.Module)
	for k, v := range result.Modules {
		log.Printf("* %s", k)
		modules[k] = v.Parsed
	}

	// Compile the loaded modules:
	newCompiler.Compile(modules)

	if newCompiler.Failed() {
		return newCompiler.Errors
	}

	setCompiler(newCompiler, result.Documents)

	return nil
}

func runRego(ctx context.Context, query string, input map[string]interface{}) (rego.ResultSet, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "runRego")
	defer span.Finish()
	// Fetch user from context, if exists.  If not, we don't mind -- some actions will be publicly possible:

	compiler := GetCompiler(ctx)

	rego := rego.New(
		rego.Query(query),
		rego.Compiler(compiler),
		rego.Input(input),
	)

	return rego.Eval(ctx)
}

func setCompiler(compiler *ast.Compiler, documents map[string]interface{}) {
	mutex.Lock()
	unsafeCompiler = compiler
	mutex.Unlock()
	dMutex.Lock()
	unsafeDocuments = documents
	unsafeStore = inmem.NewFromObject(unsafeDocuments)
	dMutex.Unlock()
}
