package main

import (
	"context"
	"fmt"
	"log"
)

type Object struct {
	ID   int
	Name string
}

func main() {
	err := LoadBundle("policies/bundle")

	if err != nil {
		panic(err)
	}

	itemCount := 200
	items := make([]Object, itemCount)

	for i := range items {
		items[i] = Object{ID: i, Name: fmt.Sprintf("Number %d", i)}
	}

	// Loops over all the items, performing a check on each in a goroutine
	opaLoop(&items)

	log.Printf("Done")
}

func opaLoop(items *[]Object) {
	replies := make(chan bool, len(*items))

	// Check policy
	for _, t := range *items {
		go func(reply chan bool, item Object) {
			_, err := Authorised(context.Background(), "data.api.entity.object.viewField", map[string]interface{}{"field": "name", "entity": t})
			if err != nil {
				log.Printf("Error: %s", err)
			}

			reply <- true

		}(replies, t)

	}

	count := 0
	for count < len(*items) {
		count++
		<-replies
	}

}
