package pubsub

import (
	"sync"
	"testing"
	"time"
)

// Test API compatibility with README examples
func TestREADMEExample(t *testing.T) {
	// Exact code from README should work
	hub := NewPubSub()
	defer hub.Close()

	subscriber := hub.NewSubscriber()
	defer subscriber.Close()

	// Subscribe to events by string key
	subscriber.Subscribe("foo")
	subscriber.Subscribe("buz")

	// Start waiting in a goroutine
	var results map[string]interface{}
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		// Wait up to 1 second for events
		results = subscriber.Wait(time.Second * 1)
	}()

	// Give subscriber time to start waiting
	time.Sleep(10 * time.Millisecond)

	// Publish events from other goroutines
	success1 := hub.Publish("foo", map[string]int{"foo": 90}) // should return true
	if !success1 {
		t.Error("Expected success1 to be true")
	}

	success2 := hub.Publish("foo", map[string]int{"foo": 100}) // should return true (overwrites previous)
	if !success2 {
		t.Error("Expected success2 to be true")
	}

	success3 := hub.Publish("bar", map[string]int{"bar": 50}) // should return false (no subscribers)
	if success3 {
		t.Error("Expected success3 to be false")
	}

	wg.Wait()

	// Results should contain: {"foo": {"foo": 100}}
	if results == nil {
		t.Fatal("Results should not be nil")
	}

	// Should have foo with latest value (100, not 90) - event overwriting
	fooData, exists := results["foo"]
	if !exists {
		t.Error("Expected foo to exist in results")
	}

	fooMap, ok := fooData.(map[string]int)
	if !ok {
		t.Error("Expected foo data to be map[string]int")
	}

	if fooMap["foo"] != 100 {
		t.Errorf("Expected foo value to be 100 (latest), got %d", fooMap["foo"])
	}

	// Should not have buz (nothing published to it)
	_, exists = results["buz"]
	if exists {
		t.Error("Expected buz not to exist in results (nothing published)")
	}

	// Should not have bar (not subscribed)
	_, exists = results["bar"]
	if exists {
		t.Error("Expected bar not to exist in results (not subscribed)")
	}
}

func TestTimeoutBehavior(t *testing.T) {
	hub := NewPubSub()
	defer hub.Close()

	subscriber := hub.NewSubscriber()
	defer subscriber.Close()

	subscriber.Subscribe("test")

	start := time.Now()
	results := subscriber.Wait(100 * time.Millisecond)
	elapsed := time.Since(start)

	// Should timeout around 100ms
	if elapsed < 100*time.Millisecond {
		t.Errorf("Expected timeout to be at least 100ms, got %v", elapsed)
	}

	// Should return empty results on timeout (as per README)
	if len(results) != 0 {
		t.Errorf("Expected empty results on timeout, got %v", results)
	}
}

func TestEarlyCompletion(t *testing.T) {
	hub := NewPubSub()
	defer hub.Close()

	subscriber := hub.NewSubscriber()
	defer subscriber.Close()

	subscriber.Subscribe("test1")
	subscriber.Subscribe("test2")

	var results map[string]interface{}
	var wg sync.WaitGroup
	wg.Add(1)

	start := time.Now()
	go func() {
		defer wg.Done()
		results = subscriber.Wait(time.Second * 1) // Long timeout
	}()

	// Give subscriber time to start waiting
	time.Sleep(10 * time.Millisecond)

	// Publish both events quickly
	hub.Publish("test1", "data1")
	hub.Publish("test2", "data2")

	wg.Wait()
	elapsed := time.Since(start)

	// Should complete much faster than 1 second (early completion)
	if elapsed > 100*time.Millisecond {
		t.Errorf("Expected early completion, took %v", elapsed)
	}

	// Should have both events
	if results["test1"] != "data1" {
		t.Errorf("Expected test1 to be 'data1', got %v", results["test1"])
	}
	if results["test2"] != "data2" {
		t.Errorf("Expected test2 to be 'data2', got %v", results["test2"])
	}
}

func TestEventOverwriting(t *testing.T) {
	hub := NewPubSub()
	defer hub.Close()

	subscriber := hub.NewSubscriber()
	defer subscriber.Close()

	subscriber.Subscribe("test")

	var results map[string]interface{}
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		results = subscriber.Wait(time.Second * 1)
	}()

	// Give subscriber time to start waiting
	time.Sleep(10 * time.Millisecond)

	// Publish multiple events with same key (should overwrite)
	hub.Publish("test", "first")
	hub.Publish("test", "second")
	hub.Publish("test", "third")

	wg.Wait()

	// Should only have the last event (overwriting behavior)
	if results["test"] != "third" {
		t.Errorf("Expected 'third' (latest event), got %v", results["test"])
	}
}
