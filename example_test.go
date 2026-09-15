package pubsub_test

import (
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/maslennikov-yv/pubsub"
)

// Example reproduces the README scenario. Publishing happens before Wait so
// the output is deterministic; in real code publishers usually run in other
// goroutines while Wait blocks.
func Example() {
	hub := pubsub.NewPubSub()
	defer hub.Close()

	subscriber := hub.NewSubscriber()
	subscriber.Subscribe("foo")
	subscriber.Subscribe("buz")

	fmt.Println(hub.Publish("foo", 90))  // true: subscriber holds "foo"
	fmt.Println(hub.Publish("foo", 100)) // true: overwrites 90
	fmt.Println(hub.Publish("bar", 50))  // false: nobody listens to "bar"

	// "buz" never arrives, so Wait returns on timeout with the partial result.
	results := subscriber.Wait(10 * time.Millisecond)

	for _, k := range slices.Sorted(maps.Keys(results)) {
		fmt.Printf("%s=%v\n", k, results[k])
	}
	// Output:
	// true
	// true
	// false
	// foo=100
}

// ExampleSubscriber_Wait shows the partial result returned on timeout: only
// keys that received a value are present.
func ExampleSubscriber_Wait() {
	hub := pubsub.NewPubSub()
	defer hub.Close()

	sub := hub.NewSubscriber()
	sub.Subscribe("temperature")
	sub.Subscribe("humidity")

	hub.Publish("temperature", 21.5)
	// "humidity" never arrives.

	results := sub.Wait(10 * time.Millisecond)
	_, hasHumidity := results["humidity"]
	fmt.Println(len(results), results["temperature"], hasHumidity)
	// Output:
	// 1 21.5 false
}

// ExamplePubSub_Close shows that Close wakes a blocked Wait immediately and
// returns whatever the subscriber collected so far.
func ExamplePubSub_Close() {
	hub := pubsub.NewPubSub()

	sub := hub.NewSubscriber()
	sub.Subscribe("never-published")

	done := make(chan map[string]any, 1)
	go func() { done <- sub.Wait(time.Hour) }()

	err := hub.Close()
	results := <-done

	fmt.Println(err, len(results), hub.IsClosed(), hub.NewSubscriber() == nil)
	// Output:
	// <nil> 0 true true
}

// ExamplePubSub_Hash derives a fixed-length topic key from an arbitrary string.
func ExamplePubSub_Hash() {
	hub := pubsub.NewPubSub()
	defer hub.Close()

	fmt.Println(hub.Hash("sensor-1"))
	// Output:
	// ebf13c772710e5bc59e6f266271799d6
}
