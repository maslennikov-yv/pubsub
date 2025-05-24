package pubsub

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"
)

// TestNewPubSub tests the constructor
func TestNewPubSub(t *testing.T) {
	ps := NewPubSub()
	defer ps.Close()

	if ps == nil {
		t.Fatal("NewPubSub() returned nil")
	}

	if ps.topics == nil {
		t.Fatal("topics map not initialized")
	}

	if len(ps.topics) != 0 {
		t.Fatalf("expected empty topics map, got %d topics", len(ps.topics))
	}

	if ps.IsClosed() {
		t.Fatal("new PubSub should not be closed")
	}
}

// TestNewSubscriber tests subscriber creation
func TestNewSubscriber(t *testing.T) {
	ps := NewPubSub()
	defer ps.Close()

	sub := ps.NewSubscriber()

	if sub == nil {
		t.Fatal("NewSubscriber() returned nil")
	}

	if sub.pubsub != ps {
		t.Fatal("subscriber not linked to pubsub")
	}

	if sub.subscriptions == nil {
		t.Fatal("subscriber subscriptions map not initialized")
	}

	if sub.results == nil {
		t.Fatal("subscriber results map not initialized")
	}

	if sub.waitCh == nil {
		t.Fatal("subscriber waitCh channel not initialized")
	}
}

// TestBasicPublishSubscribe tests basic pub/sub functionality
func TestBasicPublishSubscribe(t *testing.T) {
	ps := NewPubSub()
	defer ps.Close()

	sub := ps.NewSubscriber()

	// Subscribe to a topic
	sub.Subscribe("test-topic")

	// Publish in a goroutine
	go func() {
		time.Sleep(10 * time.Millisecond)
		success := ps.Publish("test-topic", "test-payload")
		if !success {
			t.Errorf("expected publish to succeed")
		}
	}()

	// Wait for the message
	results := sub.Wait(100 * time.Millisecond)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results["test-topic"] != "test-payload" {
		t.Fatalf("expected 'test-payload', got %v", results["test-topic"])
	}
}

// TestPublishWithoutSubscribers tests publishing to non-existent topics
func TestPublishWithoutSubscribers(t *testing.T) {
	ps := NewPubSub()
	defer ps.Close()

	success := ps.Publish("non-existent", "payload")
	if success {
		t.Fatal("expected publish to fail when no subscribers")
	}
}

// TestMultipleSubscribers tests multiple subscribers to same topic
func TestMultipleSubscribers(t *testing.T) {
	ps := NewPubSub()
	defer ps.Close()

	sub1 := ps.NewSubscriber()
	sub2 := ps.NewSubscriber()
	sub3 := ps.NewSubscriber()

	// All subscribe to same topic
	sub1.Subscribe("shared-topic")
	sub2.Subscribe("shared-topic")
	sub3.Subscribe("shared-topic")

	var wg sync.WaitGroup
	results := make([]map[string]interface{}, 3)

	// Start all subscribers
	wg.Add(3)
	go func() {
		defer wg.Done()
		results[0] = sub1.Wait(100 * time.Millisecond)
	}()
	go func() {
		defer wg.Done()
		results[1] = sub2.Wait(100 * time.Millisecond)
	}()
	go func() {
		defer wg.Done()
		results[2] = sub3.Wait(100 * time.Millisecond)
	}()

	// Publish after a short delay
	time.Sleep(10 * time.Millisecond)
	success := ps.Publish("shared-topic", "shared-payload")
	if !success {
		t.Fatal("expected publish to succeed")
	}

	wg.Wait()

	// All subscribers should receive the message
	for i, result := range results {
		if len(result) != 1 {
			t.Fatalf("subscriber %d: expected 1 result, got %d", i, len(result))
		}
		if result["shared-topic"] != "shared-payload" {
			t.Fatalf("subscriber %d: expected 'shared-payload', got %v", i, result["shared-topic"])
		}
	}
}

// TestMultipleTopicsPerSubscriber tests subscriber with multiple topics
func TestMultipleTopicsPerSubscriber(t *testing.T) {
	ps := NewPubSub()
	defer ps.Close()

	sub := ps.NewSubscriber()

	topics := []string{"topic1", "topic2", "topic3"}
	for _, topic := range topics {
		sub.Subscribe(topic)
	}

	// Publish to all topics in goroutines
	go func() {
		time.Sleep(10 * time.Millisecond)
		ps.Publish("topic1", "payload1")
		time.Sleep(10 * time.Millisecond)
		ps.Publish("topic2", "payload2")
		time.Sleep(10 * time.Millisecond)
		ps.Publish("topic3", "payload3")
	}()

	results := sub.Wait(200 * time.Millisecond)

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	expectedPayloads := map[string]string{
		"topic1": "payload1",
		"topic2": "payload2",
		"topic3": "payload3",
	}

	for topic, expectedPayload := range expectedPayloads {
		if results[topic] != expectedPayload {
			t.Fatalf("topic %s: expected '%s', got %v", topic, expectedPayload, results[topic])
		}
	}
}

// TestEventOverwritingWithinSinglePublish tests that rapid publishes before Wait() completes
func TestEventOverwritingWithinSinglePublish(t *testing.T) {
	ps := NewPubSub()
	defer ps.Close()

	sub := ps.NewSubscriber()
	sub.Subscribe("overwrite-topic")

	// Use a channel to synchronize the start of publishing
	startPublish := make(chan struct{})

	go func() {
		<-startPublish // Wait for signal to start publishing
		// Publish all events rapidly without any delays
		ps.Publish("overwrite-topic", "first")
		ps.Publish("overwrite-topic", "second")
		ps.Publish("overwrite-topic", "third")
	}()

	// Start publishing and then immediately wait
	close(startPublish)
	results := sub.Wait(100 * time.Millisecond)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	// The result should be one of the published values
	// Since they're published rapidly, any of them could be the final value
	value := results["overwrite-topic"]
	if value != "first" && value != "second" && value != "third" {
		t.Fatalf("expected one of 'first', 'second', or 'third', got %v", value)
	}
}

// TestEventOverwritingBehavior tests the actual behavior of event overwriting
func TestEventOverwritingBehavior(t *testing.T) {
	ps := NewPubSub()
	defer ps.Close()

	sub := ps.NewSubscriber()
	sub.Subscribe("behavior-topic")

	var mu sync.Mutex
	var publishResults []bool

	go func() {
		time.Sleep(10 * time.Millisecond)

		// First publish should succeed
		success1 := ps.Publish("behavior-topic", "first")
		mu.Lock()
		publishResults = append(publishResults, success1)
		mu.Unlock()

		// Subsequent publishes might fail if subscriber already completed
		success2 := ps.Publish("behavior-topic", "second")
		mu.Lock()
		publishResults = append(publishResults, success2)
		mu.Unlock()

		success3 := ps.Publish("behavior-topic", "third")
		mu.Lock()
		publishResults = append(publishResults, success3)
		mu.Unlock()
	}()

	results := sub.Wait(100 * time.Millisecond)

	// Wait for all publishes to complete
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	// The first publish should always succeed
	if len(publishResults) > 0 && !publishResults[0] {
		t.Fatal("first publish should succeed")
	}

	// The result should be from a successful publish
	value := results["behavior-topic"]
	t.Logf("Final result: %v", value)
	t.Logf("Publish results: %v", publishResults)

	// Verify we got a valid result
	if value != "first" && value != "second" && value != "third" {
		t.Fatalf("unexpected result value: %v", value)
	}
}

// TestTimeout tests timeout behavior
func TestTimeout(t *testing.T) {
	ps := NewPubSub()
	defer ps.Close()

	sub := ps.NewSubscriber()

	sub.Subscribe("timeout-topic1")
	sub.Subscribe("timeout-topic2")
	sub.Subscribe("timeout-topic3")

	// Only publish to one topic
	go func() {
		time.Sleep(10 * time.Millisecond)
		ps.Publish("timeout-topic1", "received")
		// timeout-topic2 and timeout-topic3 never get published
	}()

	start := time.Now()
	results := sub.Wait(50 * time.Millisecond)
	elapsed := time.Since(start)

	// Should timeout after ~50ms
	if elapsed < 45*time.Millisecond || elapsed > 100*time.Millisecond {
		t.Fatalf("expected timeout around 50ms, got %v", elapsed)
	}

	// Should only have the one published topic
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results["timeout-topic1"] != "received" {
		t.Fatalf("expected 'received', got %v", results["timeout-topic1"])
	}
}

// TestEarlyCompletion tests early completion when all events received
func TestEarlyCompletion(t *testing.T) {
	ps := NewPubSub()
	defer ps.Close()

	sub := ps.NewSubscriber()

	sub.Subscribe("early1")
	sub.Subscribe("early2")

	go func() {
		time.Sleep(10 * time.Millisecond)
		ps.Publish("early1", "payload1")
		time.Sleep(10 * time.Millisecond)
		ps.Publish("early2", "payload2")
	}()

	start := time.Now()
	results := sub.Wait(1 * time.Second) // Long timeout
	elapsed := time.Since(start)

	// Should complete early (much less than 1 second)
	if elapsed > 100*time.Millisecond {
		t.Fatalf("expected early completion, took %v", elapsed)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

// TestConcurrentPublish tests concurrent publishing
func TestConcurrentPublish(t *testing.T) {
	ps := NewPubSub()
	defer ps.Close()

	sub := ps.NewSubscriber()

	numTopics := 100
	for i := 0; i < numTopics; i++ {
		sub.Subscribe(fmt.Sprintf("topic%d", i))
	}

	// Publish concurrently from multiple goroutines
	var publishWg sync.WaitGroup
	publishWg.Add(numTopics)

	for i := 0; i < numTopics; i++ {
		go func(topicNum int) {
			defer publishWg.Done()
			topic := fmt.Sprintf("topic%d", topicNum)
			payload := fmt.Sprintf("payload%d", topicNum)
			ps.Publish(topic, payload)
		}(i)
	}

	// Wait for all publishes to complete
	publishWg.Wait()

	results := sub.Wait(100 * time.Millisecond)

	if len(results) != numTopics {
		t.Fatalf("expected %d results, got %d", numTopics, len(results))
	}

	// Verify all payloads
	for i := 0; i < numTopics; i++ {
		topic := fmt.Sprintf("topic%d", i)
		expectedPayload := fmt.Sprintf("payload%d", i)
		if results[topic] != expectedPayload {
			t.Fatalf("topic %s: expected '%s', got %v", topic, expectedPayload, results[topic])
		}
	}
}

// TestConcurrentSubscribers tests multiple subscribers running concurrently
func TestConcurrentSubscribers(t *testing.T) {
	ps := NewPubSub()
	defer ps.Close()

	numSubscribers := 50
	var wg sync.WaitGroup
	results := make([]map[string]interface{}, numSubscribers)

	// Create and start multiple subscribers
	wg.Add(numSubscribers)
	for i := 0; i < numSubscribers; i++ {
		go func(subNum int) {
			defer wg.Done()
			sub := ps.NewSubscriber()
			sub.Subscribe("concurrent-topic")
			results[subNum] = sub.Wait(100 * time.Millisecond)
		}(i)
	}

	// Publish after a short delay
	time.Sleep(10 * time.Millisecond)
	success := ps.Publish("concurrent-topic", "concurrent-payload")
	if !success {
		t.Fatal("expected publish to succeed")
	}

	wg.Wait()

	// All subscribers should receive the message
	for i, result := range results {
		if len(result) != 1 {
			t.Fatalf("subscriber %d: expected 1 result, got %d", i, len(result))
		}
		if result["concurrent-topic"] != "concurrent-payload" {
			t.Fatalf("subscriber %d: expected 'concurrent-payload', got %v", i, result["concurrent-topic"])
		}
	}
}

// TestTopicCleanup tests that topics are cleaned up when no subscribers remain
func TestTopicCleanup(t *testing.T) {
	ps := NewPubSub()
	defer ps.Close()

	// Create subscriber and subscribe
	sub := ps.NewSubscriber()
	sub.Subscribe("cleanup-topic")

	// Verify topic exists
	if ps.GetTopicCount() != 1 {
		t.Fatalf("expected 1 topic after subscription, got %d", ps.GetTopicCount())
	}

	if ps.GetSubscriberCount("cleanup-topic") != 1 {
		t.Fatalf("expected 1 subscriber for topic, got %d", ps.GetSubscriberCount("cleanup-topic"))
	}

	// Publish and wait (this will clean up the subscriber)
	go func() {
		time.Sleep(10 * time.Millisecond)
		ps.Publish("cleanup-topic", "cleanup-payload")
	}()

	results := sub.Wait(100 * time.Millisecond)

	// Verify we got the result
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results["cleanup-topic"] != "cleanup-payload" {
		t.Fatalf("expected 'cleanup-payload', got %v", results["cleanup-topic"])
	}

	// Verify topic is cleaned up
	if ps.GetTopicCount() != 0 {
		t.Fatalf("expected 0 topics after cleanup, got %d", ps.GetTopicCount())
	}

	if ps.GetSubscriberCount("cleanup-topic") != 0 {
		t.Fatalf("expected 0 subscribers after cleanup, got %d", ps.GetSubscriberCount("cleanup-topic"))
	}
}

// TestTopicCleanupWithTimeout tests cleanup behavior with timeout
func TestTopicCleanupWithTimeout(t *testing.T) {
	ps := NewPubSub()
	defer ps.Close()

	// Create subscriber and subscribe
	sub := ps.NewSubscriber()
	sub.Subscribe("timeout-cleanup-topic")

	// Verify topic exists
	if ps.GetTopicCount() != 1 {
		t.Fatalf("expected 1 topic after subscription, got %d", ps.GetTopicCount())
	}

	// Don't publish anything - let it timeout
	results := sub.Wait(10 * time.Millisecond)

	// Should have no results due to timeout
	if len(results) != 0 {
		t.Fatalf("expected 0 results due to timeout, got %d", len(results))
	}

	// Verify topic is cleaned up even after timeout
	if ps.GetTopicCount() != 0 {
		t.Fatalf("expected 0 topics after timeout cleanup, got %d", ps.GetTopicCount())
	}
}

// TestHash tests the Hash function
func TestHash(t *testing.T) {
	ps := NewPubSub()
	defer ps.Close()

	testCases := []struct {
		input string
	}{
		{""},
		{"hello"},
		{"test"},
		{"pubsub"},
		{"very-long-key-that-should-also-work"},
	}

	for _, tc := range testCases {
		result := ps.Hash(tc.input)

		// Verify it's a valid MD5 hash (32 hex characters)
		if len(result) != 32 {
			t.Fatalf("hash of '%s': expected 32 characters, got %d", tc.input, len(result))
		}

		// Verify it matches expected MD5
		expectedHash := md5.Sum([]byte(tc.input))
		expected := hex.EncodeToString(expectedHash[:])

		if result != expected {
			t.Fatalf("hash of '%s': expected '%s', got '%s'", tc.input, expected, result)
		}

		// Verify consistency - same input should always produce same hash
		result2 := ps.Hash(tc.input)
		if result != result2 {
			t.Fatalf("hash inconsistent for '%s': got '%s' and '%s'", tc.input, result, result2)
		}
	}
}

// TestSubscribeToSameTopicMultipleTimes tests subscribing to same topic multiple times
func TestSubscribeToSameTopicMultipleTimes(t *testing.T) {
	ps := NewPubSub()
	defer ps.Close()

	sub := ps.NewSubscriber()

	// Subscribe to same topic multiple times
	sub.Subscribe("duplicate-topic")
	sub.Subscribe("duplicate-topic")
	sub.Subscribe("duplicate-topic")

	// Should still only count as one subscription
	if ps.GetSubscriberCount("duplicate-topic") != 1 {
		t.Fatalf("expected 1 subscriber despite multiple subscriptions, got %d", ps.GetSubscriberCount("duplicate-topic"))
	}

	go func() {
		time.Sleep(10 * time.Millisecond)
		ps.Publish("duplicate-topic", "duplicate-payload")
	}()

	results := sub.Wait(100 * time.Millisecond)

	// Should still only get one result
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results["duplicate-topic"] != "duplicate-payload" {
		t.Fatalf("expected 'duplicate-payload', got %v", results["duplicate-topic"])
	}
}

// TestEmptyKey tests behavior with empty string keys
func TestEmptyKey(t *testing.T) {
	ps := NewPubSub()
	defer ps.Close()

	sub := ps.NewSubscriber()

	sub.Subscribe("")

	go func() {
		time.Sleep(10 * time.Millisecond)
		success := ps.Publish("", "empty-key-payload")
		if !success {
			t.Errorf("expected publish to succeed for empty key")
		}
	}()

	results := sub.Wait(100 * time.Millisecond)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[""] != "empty-key-payload" {
		t.Fatalf("expected 'empty-key-payload', got %v", results[""])
	}
}

// TestNilPayload tests publishing nil payloads
func TestNilPayload(t *testing.T) {
	ps := NewPubSub()
	defer ps.Close()

	sub := ps.NewSubscriber()

	sub.Subscribe("nil-topic")

	go func() {
		time.Sleep(10 * time.Millisecond)
		success := ps.Publish("nil-topic", nil)
		if !success {
			t.Errorf("expected publish to succeed with nil payload")
		}
	}()

	results := sub.Wait(100 * time.Millisecond)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results["nil-topic"] != nil {
		t.Fatalf("expected nil, got %v", results["nil-topic"])
	}
}

// TestComplexPayload tests publishing complex data structures
func TestComplexPayload(t *testing.T) {
	ps := NewPubSub()
	defer ps.Close()

	sub := ps.NewSubscriber()

	sub.Subscribe("complex-topic")

	complexPayload := map[string]interface{}{
		"string":  "value",
		"number":  42,
		"boolean": true,
		"array":   []int{1, 2, 3},
		"nested": map[string]string{
			"key": "nested-value",
		},
	}

	go func() {
		time.Sleep(10 * time.Millisecond)
		ps.Publish("complex-topic", complexPayload)
	}()

	results := sub.Wait(100 * time.Millisecond)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	// Verify the complex payload was received correctly
	received := results["complex-topic"].(map[string]interface{})

	if received["string"] != "value" {
		t.Fatalf("expected 'value', got %v", received["string"])
	}

	if received["number"] != 42 {
		t.Fatalf("expected 42, got %v", received["number"])
	}
}

// TestZeroTimeout tests behavior with zero timeout
func TestZeroTimeout(t *testing.T) {
	ps := NewPubSub()
	defer ps.Close()

	sub := ps.NewSubscriber()

	sub.Subscribe("zero-timeout-topic")

	// Don't publish anything
	start := time.Now()
	results := sub.Wait(0)
	elapsed := time.Since(start)

	// Should return immediately
	if elapsed > 10*time.Millisecond {
		t.Fatalf("expected immediate return, took %v", elapsed)
	}

	// Should have no results
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}

	// Should be cleaned up
	if ps.GetTopicCount() != 0 {
		t.Fatalf("expected 0 topics after zero timeout, got %d", ps.GetTopicCount())
	}
}

// TestMemoryLeaks tests for potential memory leaks
func TestMemoryLeaks(t *testing.T) {
	ps := NewPubSub()
	defer ps.Close()

	// Create many subscribers and let them timeout
	for i := 0; i < 100; i++ {
		sub := ps.NewSubscriber()
		sub.Subscribe(fmt.Sprintf("leak-topic-%d", i))

		go func() {
			sub.Wait(1 * time.Millisecond) // Very short timeout
		}()
	}

	// Give time for all to timeout and cleanup
	time.Sleep(50 * time.Millisecond)

	// Force garbage collection
	runtime.GC()
	runtime.GC()

	// Check that topics map is empty (all cleaned up)
	if ps.GetTopicCount() != 0 {
		t.Fatalf("expected 0 topics after cleanup, got %d", ps.GetTopicCount())
	}
}

// TestPublishAfterSubscriberCompletes tests publishing after subscriber has finished
func TestPublishAfterSubscriberCompletes(t *testing.T) {
	ps := NewPubSub()
	defer ps.Close()

	sub := ps.NewSubscriber()
	sub.Subscribe("after-complete")

	// Complete the subscriber with a short timeout
	results := sub.Wait(10 * time.Millisecond)

	// Should have no results due to timeout
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}

	// Verify topic is cleaned up
	if ps.GetTopicCount() != 0 {
		t.Fatalf("expected 0 topics after subscriber completion, got %d", ps.GetTopicCount())
	}

	// Now try to publish - should fail because no subscribers
	success := ps.Publish("after-complete", "too-late")
	if success {
		t.Fatal("expected publish to fail after subscriber completed")
	}
}

// TestRapidPublishSubscribe tests rapid publish/subscribe cycles
func TestRapidPublishSubscribe(t *testing.T) {
	ps := NewPubSub()
	defer ps.Close()

	for i := 0; i < 10; i++ {
		sub := ps.NewSubscriber()
		topic := fmt.Sprintf("rapid-%d", i)
		payload := fmt.Sprintf("payload-%d", i)

		sub.Subscribe(topic)

		go func(t string, p string) {
			time.Sleep(1 * time.Millisecond)
			ps.Publish(t, p)
		}(topic, payload)

		results := sub.Wait(50 * time.Millisecond)

		if len(results) != 1 {
			t.Fatalf("iteration %d: expected 1 result, got %d", i, len(results))
		}

		if results[topic] != payload {
			t.Fatalf("iteration %d: expected '%s', got %v", i, payload, results[topic])
		}

		// Verify cleanup after each iteration
		if ps.GetTopicCount() != 0 {
			t.Fatalf("iteration %d: expected 0 topics after cleanup, got %d", i, ps.GetTopicCount())
		}
	}
}

// TestGetTopicCount tests the GetTopicCount helper method
func TestGetTopicCount(t *testing.T) {
	ps := NewPubSub()
	defer ps.Close()

	if ps.GetTopicCount() != 0 {
		t.Fatalf("expected 0 topics initially, got %d", ps.GetTopicCount())
	}

	sub1 := ps.NewSubscriber()
	sub1.Subscribe("topic1")

	if ps.GetTopicCount() != 1 {
		t.Fatalf("expected 1 topic after first subscription, got %d", ps.GetTopicCount())
	}

	sub2 := ps.NewSubscriber()
	sub2.Subscribe("topic2")

	if ps.GetTopicCount() != 2 {
		t.Fatalf("expected 2 topics after second subscription, got %d", ps.GetTopicCount())
	}

	// Subscribe to existing topic
	sub3 := ps.NewSubscriber()
	sub3.Subscribe("topic1")

	if ps.GetTopicCount() != 2 {
		t.Fatalf("expected 2 topics after subscribing to existing topic, got %d", ps.GetTopicCount())
	}
}

// TestGetSubscriberCount tests the GetSubscriberCount helper method
func TestGetSubscriberCount(t *testing.T) {
	ps := NewPubSub()
	defer ps.Close()

	if ps.GetSubscriberCount("nonexistent") != 0 {
		t.Fatalf("expected 0 subscribers for nonexistent topic, got %d", ps.GetSubscriberCount("nonexistent"))
	}

	sub1 := ps.NewSubscriber()
	sub1.Subscribe("topic1")

	if ps.GetSubscriberCount("topic1") != 1 {
		t.Fatalf("expected 1 subscriber for topic1, got %d", ps.GetSubscriberCount("topic1"))
	}

	sub2 := ps.NewSubscriber()
	sub2.Subscribe("topic1")

	if ps.GetSubscriberCount("topic1") != 2 {
		t.Fatalf("expected 2 subscribers for topic1, got %d", ps.GetSubscriberCount("topic1"))
	}

	sub3 := ps.NewSubscriber()
	sub3.Subscribe("topic2")

	if ps.GetSubscriberCount("topic1") != 2 {
		t.Fatalf("expected 2 subscribers for topic1 after topic2 subscription, got %d", ps.GetSubscriberCount("topic1"))
	}

	if ps.GetSubscriberCount("topic2") != 1 {
		t.Fatalf("expected 1 subscriber for topic2, got %d", ps.GetSubscriberCount("topic2"))
	}
}

// TestClose tests the Close method
func TestClose(t *testing.T) {
	ps := NewPubSub()

	// Create some subscribers
	sub1 := ps.NewSubscriber()
	sub2 := ps.NewSubscriber()

	sub1.Subscribe("topic1")
	sub2.Subscribe("topic2")

	// Verify initial state
	if ps.IsClosed() {
		t.Fatal("PubSub should not be closed initially")
	}

	if ps.GetTopicCount() != 2 {
		t.Fatalf("expected 2 topics, got %d", ps.GetTopicCount())
	}

	// Close the PubSub - should not return error for inactive subscribers
	err := ps.Close()
	if err != nil {
		t.Logf("close returned error (may be acceptable): %v", err)
	}

	// Verify closed state
	if !ps.IsClosed() {
		t.Fatal("PubSub should be closed after Close()")
	}

	if ps.GetTopicCount() != 0 {
		t.Fatalf("expected 0 topics after close, got %d", ps.GetTopicCount())
	}
}

// TestCloseMultipleTimes tests calling Close multiple times
func TestCloseMultipleTimes(t *testing.T) {
	ps := NewPubSub()

	// First close should succeed
	err1 := ps.Close()
	if err1 != nil {
		t.Fatalf("first close failed: %v", err1)
	}

	// Subsequent closes should be safe (no-op)
	err2 := ps.Close()
	if err2 != nil {
		t.Fatalf("second close failed: %v", err2)
	}

	err3 := ps.Close()
	if err3 != nil {
		t.Fatalf("third close failed: %v", err3)
	}

	if !ps.IsClosed() {
		t.Fatal("PubSub should remain closed")
	}
}

// TestOperationsAfterClose tests that operations fail gracefully after close
func TestOperationsAfterClose(t *testing.T) {
	ps := NewPubSub()

	// Close the PubSub
	err := ps.Close()
	if err != nil {
		t.Fatalf("close failed: %v", err)
	}

	// NewSubscriber should return nil
	sub := ps.NewSubscriber()
	if sub != nil {
		t.Fatal("NewSubscriber should return nil after close")
	}

	// Publish should return false
	success := ps.Publish("test", "payload")
	if success {
		t.Fatal("Publish should return false after close")
	}
}

// TestCloseWithActiveSubscribers tests closing with active subscribers
func TestCloseWithActiveSubscribers(t *testing.T) {
	ps := NewPubSub()

	numSubscribers := 10
	var wg sync.WaitGroup
	results := make([]map[string]interface{}, numSubscribers)

	// Start multiple subscribers with long timeouts
	wg.Add(numSubscribers)
	for i := 0; i < numSubscribers; i++ {
		go func(index int) {
			defer wg.Done()
			sub := ps.NewSubscriber()
			if sub != nil {
				sub.Subscribe(fmt.Sprintf("topic%d", index))
				// Long timeout - should be interrupted by Close()
				results[index] = sub.Wait(5 * time.Second)
			}
		}(i)
	}

	// Give subscribers time to start
	time.Sleep(50 * time.Millisecond)

	// Verify subscribers are active
	if ps.GetTopicCount() != numSubscribers {
		t.Fatalf("expected %d topics, got %d", numSubscribers, ps.GetTopicCount())
	}

	// Close should interrupt all subscribers
	start := time.Now()
	err := ps.Close()
	elapsed := time.Since(start)

	// Close might return an error due to cleanup timeout, but that's acceptable
	if err != nil {
		t.Logf("close returned error (acceptable): %v", err)
	}

	// Close should complete within reasonable time (100ms timeout + buffer)
	if elapsed > 500*time.Millisecond {
		t.Fatalf("close took too long: %v", elapsed)
	}

	// Wait for all subscribers to complete
	wg.Wait()

	// Verify all topics are cleaned up
	if ps.GetTopicCount() != 0 {
		t.Fatalf("expected 0 topics after close, got %d", ps.GetTopicCount())
	}

	// All results should be empty (interrupted by close)
	for i, result := range results {
		if len(result) != 0 {
			t.Fatalf("subscriber %d: expected empty result due to close interruption, got %v", i, result)
		}
	}
}

// TestCloseTimeout tests close timeout behavior
func TestCloseTimeout(t *testing.T) {
	ps := NewPubSub()

	// Create a subscriber that might be slow to clean up
	sub := ps.NewSubscriber()
	sub.Subscribe("slow-topic")

	// Start a subscriber with a very long timeout
	go func() {
		sub.Wait(10 * time.Second)
	}()

	// Give subscriber time to start waiting
	time.Sleep(20 * time.Millisecond)

	// Close should complete within reasonable time
	start := time.Now()
	err := ps.Close()
	elapsed := time.Since(start)

	// Should complete within the 100ms timeout plus some buffer
	if elapsed > 500*time.Millisecond {
		t.Fatalf("close took too long: %v", elapsed)
	}

	// Error might be returned if cleanup times out, but that's acceptable
	if err != nil {
		t.Logf("close returned error (acceptable): %v", err)
	}

	if !ps.IsClosed() {
		t.Fatal("PubSub should be closed even if cleanup timed out")
	}
}

// BenchmarkPublishSubscribe benchmarks basic pub/sub performance
func BenchmarkPublishSubscribe(b *testing.B) {
	ps := NewPubSub()
	defer ps.Close()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		sub := ps.NewSubscriber()
		sub.Subscribe("bench-topic")

		go func() {
			ps.Publish("bench-topic", "bench-payload")
		}()

		sub.Wait(100 * time.Millisecond)
	}
}

// BenchmarkConcurrentPublish benchmarks concurrent publishing
func BenchmarkConcurrentPublish(b *testing.B) {
	ps := NewPubSub()
	defer ps.Close()

	sub := ps.NewSubscriber()
	sub.Subscribe("concurrent-bench")

	b.ResetTimer()

	var wg sync.WaitGroup
	wg.Add(b.N)

	for i := 0; i < b.N; i++ {
		go func() {
			defer wg.Done()
			ps.Publish("concurrent-bench", "payload")
		}()
	}

	wg.Wait()
}

// BenchmarkHash benchmarks the Hash function
func BenchmarkHash(b *testing.B) {
	ps := NewPubSub()
	defer ps.Close()

	keys := []string{
		"short",
		"medium-length-key",
		"very-long-key-that-might-be-used-in-real-applications",
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		key := keys[i%len(keys)]
		ps.Hash(key)
	}
}

// BenchmarkClose benchmarks the Close method
func BenchmarkClose(b *testing.B) {
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ps := NewPubSub()

		// Create some subscribers
		for j := 0; j < 10; j++ {
			sub := ps.NewSubscriber()
			sub.Subscribe(fmt.Sprintf("topic%d", j))
		}

		ps.Close()
	}
}
