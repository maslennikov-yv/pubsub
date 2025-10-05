package main

import (
	"fmt"
	"github.com/maslennikov-yv/pubsub"
	"time"
)

func main() {
	// Create a new pub-sub hub
	hub := pubsub.NewPubSub()

	// Ensure proper cleanup when the program exits
	defer func() {
		fmt.Println("\nShutting down PubSub system...")
		if err := hub.Close(); err != nil {
			fmt.Printf("Error during shutdown: %v\n", err)
		} else {
			fmt.Println("PubSub system shut down successfully")
		}
	}()

	// Create a subscriber
	subscriber := hub.NewSubscriber()
	if subscriber == nil {
		fmt.Println("Failed to create subscriber - PubSub may be closed")
		return
	}

	// Subscribe to topics we're interested in
	subscriber.Subscribe("temperature")
	subscriber.Subscribe("humidity")

	// Start publishing data in a separate goroutine
	go func() {
		// Simulate sensor data arriving at different times
		time.Sleep(50 * time.Millisecond)
		success := hub.Publish("temperature", map[string]interface{}{
			"value": 25.5,
			"unit":  "°C",
		})
		if !success {
			fmt.Println("Failed to publish temperature data")
		}

		time.Sleep(100 * time.Millisecond)
		success = hub.Publish("humidity", map[string]interface{}{
			"value": 60.0,
			"unit":  "%",
		})
		if !success {
			fmt.Println("Failed to publish humidity data")
		}
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
