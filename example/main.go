package main

import (
	"fmt"
	"time"
	"pubsub"
)

func main() {
	// Create a new pub-sub hub
	hub := pubsub.NewPubSub()

	// Create a subscriber
	subscriber := hub.NewSubscriber()

	// Subscribe to topics we're interested in
	subscriber.Subscribe("temperature")
	subscriber.Subscribe("humidity")

	// Start publishing data in a separate goroutine
	go func() {
		// Simulate sensor data arriving at different times
		time.Sleep(50 * time.Millisecond)
		hub.Publish("temperature", map[string]interface{}{
			"value": 25.5,
			"unit":  "°C",
		})

		time.Sleep(100 * time.Millisecond)
		hub.Publish("humidity", map[string]interface{}{
			"value": 60.0,
			"unit":  "%",
		})
	}()

	// Wait for events with a 1-second timeout
	fmt.Println("Waiting for sensor data...")
	results := subscriber.Wait(1 * time.Second)

	// Display the results
	fmt.Printf("Received %d events:\n", len(results))
	for topic, data := range results {
		fmt.Printf("  %s: %v\n", topic, data)
	}
}
