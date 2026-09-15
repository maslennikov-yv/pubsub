// Command sensors demonstrates collecting readings from several sensors with
// a single timeout using github.com/maslennikov-yv/pubsub.
package main

import (
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/maslennikov-yv/pubsub"
)

func main() {
	// Create a new pub-sub hub. Close is synchronous and always returns nil.
	hub := pubsub.NewPubSub()
	defer hub.Close()

	// Create a subscriber and register the topics we are interested in.
	// NewSubscriber returns nil only after Close, so no check is needed here.
	subscriber := hub.NewSubscriber()
	subscriber.Subscribe("temperature")
	subscriber.Subscribe("humidity")

	// Start publishing data in a separate goroutine
	go func() {
		// Simulate sensor data arriving at different times
		time.Sleep(50 * time.Millisecond)
		success := hub.Publish("temperature", map[string]any{
			"value": 25.5,
			"unit":  "°C",
		})
		if !success {
			fmt.Println("Failed to publish temperature data")
		}

		time.Sleep(100 * time.Millisecond)
		success = hub.Publish("humidity", map[string]any{
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

	// Display the results in a stable order (map iteration order is random).
	fmt.Printf("Received %d events:\n", len(results))
	for _, topic := range slices.Sorted(maps.Keys(results)) {
		fmt.Printf("  %s: %v\n", topic, results[topic])
	}
}
