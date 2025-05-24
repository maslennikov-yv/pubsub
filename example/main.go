package main

import (
	"fmt"
	"time"

	"github.com/maslennikov-yv/pubsub"
)

func main() {
	// Create the pub-sub hub
	hub := pubsub.NewPubSub()
	defer hub.Close()

	subscriber := hub.NewSubscriber()
	defer subscriber.Close()

	// Subscribe to events by string key
	subscriber.Subscribe("foo")
	subscriber.Subscribe("buz")

	// Start waiting for events in a goroutine
	go func() {
		fmt.Println("Waiting for events (1 second timeout)...")
		results := subscriber.Wait(time.Second * 1)
		
		fmt.Println("Results received:")
		for key, value := range results {
			fmt.Printf("  %s: %v\n", key, value)
		}
	}()

	// Give subscriber time to start waiting
	time.Sleep(100 * time.Millisecond)

	// Publish events from main goroutine
	fmt.Println("Publishing events...")
	
	success1 := hub.Publish("foo", map[string]int{"foo": 90})
	fmt.Printf("Published foo=90: %t\n", success1)
	
	success2 := hub.Publish("foo", map[string]int{"foo": 100})
	fmt.Printf("Published foo=100: %t\n", success2)
	
	success3 := hub.Publish("bar", map[string]int{"bar": 50})
	fmt.Printf("Published bar=50: %t\n", success3)

	// Wait for completion
	time.Sleep(time.Second * 2)
}