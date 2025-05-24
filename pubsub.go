package pubsub

import (
	"crypto/md5"
	"encoding/hex"
	"log"
	"sync"
	"time"
)

// Constants for timeouts and configuration
const (
	// CleanupTimeout defines how long to wait for subscriber cleanup during shutdown
	CleanupTimeout = 100 * time.Millisecond

	// CleanupGracePeriod is a short delay to allow subscribers to react to close signal
	CleanupGracePeriod = 5 * time.Millisecond

	// ImmediateCleanupDelay is used when no active waiters are present
	ImmediateCleanupDelay = 10 * time.Millisecond
)

// PubSub represents a thread-safe publish-subscribe system
type PubSub struct {
	mutex     sync.RWMutex
	topics    map[string]*Topic
	closed    bool
	closeCh   chan struct{}
	closeOnce sync.Once
}

// NewPubSub creates a new PubSub instance
func NewPubSub() *PubSub {
	return &PubSub{
		topics:  make(map[string]*Topic),
		closed:  false,
		closeCh: make(chan struct{}),
	}
}

// Subscriber represents a subscriber that can wait for multiple events with timeout
type Subscriber struct {
	pubsub        *PubSub
	mutex         sync.RWMutex
	subscriptions map[string]bool
	results       map[string]interface{}
	waitCh        chan struct{}
	completed     bool
	cleanedUp     bool
	expectedCount int
	receivedCount int
	isWaiting     bool
}

// Topic represents a topic with its subscribers
type Topic struct {
	mutex       sync.RWMutex
	subscribers map[*Subscriber]bool
}

// PubSubError represents an error that occurred during PubSub operations
type PubSubError struct {
	Operation string
	Message   string
}

func (e *PubSubError) Error() string {
	return "pubsub " + e.Operation + ": " + e.Message
}

// Close gracefully shuts down the PubSub system
func (ps *PubSub) Close() error {
	var closeErr error

	ps.closeOnce.Do(func() {
		closeErr = ps.performShutdown()
	})

	return closeErr
}

// performShutdown handles the actual shutdown logic
func (ps *PubSub) performShutdown() error {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()

	if ps.closed {
		return nil
	}

	log.Println("PubSub: Starting graceful shutdown...")

	// Mark as closed to prevent new operations
	ps.closed = true

	// Signal shutdown to all waiting subscribers
	close(ps.closeCh)

	// Allow brief time for subscribers to react
	time.Sleep(CleanupGracePeriod)

	// Collect and categorize subscribers
	allSubscribers, waitingSubscribers := ps.collectSubscribers()

	log.Printf("PubSub: Cleaning up %d unique subscribers across %d topics (%d actively waiting)",
		len(allSubscribers), len(ps.topics), len(waitingSubscribers))

	// Perform cleanup based on whether there are active waiters
	var cleanupErr error
	if len(waitingSubscribers) == 0 {
		cleanupErr = ps.performImmediateCleanup(allSubscribers)
	} else {
		cleanupErr = ps.performTimeoutCleanup(allSubscribers)
	}

	// Clear all topics regardless of cleanup result
	topicCount := len(ps.topics)
	ps.clearAllTopics()

	log.Printf("PubSub: Cleared %d topics", topicCount)
	log.Println("PubSub: Shutdown completed")

	return cleanupErr
}

// collectSubscribers gathers all subscribers and identifies which are actively waiting
func (ps *PubSub) collectSubscribers() ([]*Subscriber, []*Subscriber) {
	var allSubscribers []*Subscriber
	var waitingSubscribers []*Subscriber
	subscriberMap := make(map[*Subscriber]bool)

	for topicKey, topic := range ps.topics {
		topic.mutex.RLock()
		for subscriber := range topic.subscribers {
			if !subscriberMap[subscriber] {
				allSubscribers = append(allSubscribers, subscriber)
				subscriberMap[subscriber] = true

				// Check if this subscriber is actively waiting
				subscriber.mutex.RLock()
				if subscriber.isWaiting && !subscriber.completed {
					waitingSubscribers = append(waitingSubscribers, subscriber)
				}
				subscriber.mutex.RUnlock()
			}
		}
		topic.mutex.RUnlock()
		log.Printf("PubSub: Found %d subscribers for topic '%s'", len(topic.subscribers), topicKey)
	}

	return allSubscribers, waitingSubscribers
}

// performImmediateCleanup handles cleanup when no subscribers are actively waiting
func (ps *PubSub) performImmediateCleanup(subscribers []*Subscriber) error {
	log.Println("PubSub: No active waiters, performing immediate cleanup")

	for _, subscriber := range subscribers {
		go ps.cleanupSubscriberSafely(subscriber)
	}

	time.Sleep(ImmediateCleanupDelay)
	return nil
}

// performTimeoutCleanup handles cleanup with timeout when there are active waiters
func (ps *PubSub) performTimeoutCleanup(subscribers []*Subscriber) error {
	var wg sync.WaitGroup

	for _, subscriber := range subscribers {
		wg.Add(1)
		go func(sub *Subscriber) {
			defer wg.Done()
			ps.cleanupSubscriberSafely(sub)
		}(subscriber)
	}

	// Wait for cleanup completion with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Println("PubSub: All subscribers cleaned up successfully")
		return nil
	case <-time.After(CleanupTimeout):
		log.Printf("PubSub: Warning - subscriber cleanup timed out after %v", CleanupTimeout)
		return &PubSubError{
			Operation: "Close",
			Message:   "subscriber cleanup timeout",
		}
	}
}

// cleanupSubscriberSafely performs subscriber cleanup with panic recovery
func (ps *PubSub) cleanupSubscriberSafely(subscriber *Subscriber) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("PubSub: Panic during subscriber cleanup: %v", r)
		}
	}()
	subscriber.forceCleanup()
}

// clearAllTopics removes all topics from the PubSub instance
func (ps *PubSub) clearAllTopics() {
	for key := range ps.topics {
		delete(ps.topics, key)
	}
}

// IsClosed returns true if the PubSub instance has been closed
func (ps *PubSub) IsClosed() bool {
	ps.mutex.RLock()
	defer ps.mutex.RUnlock()
	return ps.closed
}

// NewSubscriber creates a new subscriber for this PubSub instance
func (ps *PubSub) NewSubscriber() *Subscriber {
	ps.mutex.RLock()
	defer ps.mutex.RUnlock()

	if ps.closed {
		log.Println("PubSub: Warning - attempted to create subscriber on closed PubSub instance")
		return nil
	}

	return &Subscriber{
		pubsub:        ps,
		subscriptions: make(map[string]bool),
		results:       make(map[string]interface{}),
		waitCh:        make(chan struct{}),
		completed:     false,
		cleanedUp:     false,
		isWaiting:     false,
	}
}

// Subscribe adds a subscription to the specified topic key
func (s *Subscriber) Subscribe(key string) {
	if s == nil {
		log.Println("PubSub: Warning - attempted to subscribe with nil subscriber")
		return
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.completed || s.subscriptions[key] {
		return // Don't allow subscription after completion or duplicate subscriptions
	}

	// Verify PubSub is still active
	if !s.isPubSubActive() {
		log.Printf("PubSub: Warning - attempted to subscribe to '%s' on closed PubSub instance", key)
		return
	}

	s.addSubscription(key)
	s.addToTopic(key)
}

// isPubSubActive checks if the PubSub instance is still active
func (s *Subscriber) isPubSubActive() bool {
	s.pubsub.mutex.RLock()
	defer s.pubsub.mutex.RUnlock()
	return !s.pubsub.closed
}

// addSubscription adds a subscription to the subscriber's internal tracking
func (s *Subscriber) addSubscription(key string) {
	s.subscriptions[key] = true
	s.expectedCount++
}

// addToTopic adds this subscriber to the specified topic
func (s *Subscriber) addToTopic(key string) {
	s.pubsub.mutex.Lock()
	topic, exists := s.pubsub.topics[key]
	if !exists {
		topic = &Topic{
			subscribers: make(map[*Subscriber]bool),
		}
		s.pubsub.topics[key] = topic
	}
	s.pubsub.mutex.Unlock()

	topic.mutex.Lock()
	topic.subscribers[s] = true
	topic.mutex.Unlock()
}

// Publish sends a message to all subscribers of the specified topic
func (ps *PubSub) Publish(key string, payload interface{}) bool {
	ps.mutex.RLock()
	if ps.closed {
		ps.mutex.RUnlock()
		log.Printf("PubSub: Warning - attempted to publish to '%s' on closed PubSub instance", key)
		return false
	}

	topic, exists := ps.topics[key]
	ps.mutex.RUnlock()

	if !exists {
		return false
	}

	subscribers := ps.getTopicSubscribers(topic)
	if len(subscribers) == 0 {
		return false
	}

	return ps.deliverToSubscribers(subscribers, key, payload)
}

// getTopicSubscribers safely retrieves all subscribers for a topic
func (ps *PubSub) getTopicSubscribers(topic *Topic) []*Subscriber {
	topic.mutex.RLock()
	defer topic.mutex.RUnlock()

	subscribers := make([]*Subscriber, 0, len(topic.subscribers))
	for subscriber := range topic.subscribers {
		subscribers = append(subscribers, subscriber)
	}
	return subscribers
}

// deliverToSubscribers attempts to deliver a message to all provided subscribers
func (ps *PubSub) deliverToSubscribers(subscribers []*Subscriber, key string, payload interface{}) bool {
	delivered := false
	for _, subscriber := range subscribers {
		if subscriber.deliverMessage(key, payload) {
			delivered = true
		}
	}
	return delivered
}

// deliverMessage delivers a message to this subscriber
func (s *Subscriber) deliverMessage(key string, payload interface{}) bool {
	if s == nil {
		return false
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.completed || !s.subscriptions[key] {
		return false
	}

	wasNewMessage := s.updateResults(key, payload)

	// Check if all expected messages have been received
	if wasNewMessage && s.receivedCount >= s.expectedCount {
		s.markCompleted()
	}

	return true
}

// updateResults updates the results map with the new payload
func (s *Subscriber) updateResults(key string, payload interface{}) bool {
	wasNew := false
	if _, exists := s.results[key]; !exists {
		s.receivedCount++
		wasNew = true
	}

	// Always update the payload (implements overwrite behavior)
	s.results[key] = payload
	return wasNew
}

// markCompleted marks the subscriber as completed and signals waiting goroutines
func (s *Subscriber) markCompleted() {
	s.completed = true
	s.isWaiting = false
	close(s.waitCh)
}

// Wait waits for subscribed events with the specified timeout
func (s *Subscriber) Wait(timeout time.Duration) map[string]interface{} {
	if s == nil {
		log.Println("PubSub: Warning - Wait called on nil subscriber")
		return make(map[string]interface{})
	}

	s.setWaitingState(true)
	defer s.setWaitingState(false)

	// Check for immediate conditions
	if s.shouldReturnImmediately(timeout) {
		s.cleanup()
		return s.getResults()
	}

	// Wait for completion, timeout, or shutdown
	s.waitForCompletion(timeout)
	s.cleanup()
	return s.getResults()
}

// setWaitingState safely updates the waiting state
func (s *Subscriber) setWaitingState(waiting bool) {
	s.mutex.Lock()
	s.isWaiting = waiting
	s.mutex.Unlock()
}

// shouldReturnImmediately checks if Wait should return immediately
func (s *Subscriber) shouldReturnImmediately(timeout time.Duration) bool {
	// Check if PubSub is closed
	select {
	case <-s.pubsub.closeCh:
		log.Println("PubSub: Wait interrupted due to PubSub shutdown")
		return true
	default:
	}

	return timeout <= 0
}

// waitForCompletion waits for one of several completion conditions
func (s *Subscriber) waitForCompletion(timeout time.Duration) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-s.waitCh:
		// All messages received
	case <-timer.C:
		// Timeout occurred
	case <-s.pubsub.closeCh:
		// PubSub is being closed
		log.Println("PubSub: Wait interrupted due to PubSub shutdown")
	}
}

// cleanup performs subscriber cleanup after Wait completes
func (s *Subscriber) cleanup() {
	if s == nil {
		return
	}

	s.mutex.Lock()

	if s.cleanedUp {
		s.mutex.Unlock()
		return
	}

	s.performCleanup()
	subscriptionKeys := s.getSubscriptionKeys()
	s.mutex.Unlock()

	// Remove from topics outside the lock to avoid deadlock
	s.removeFromTopics(subscriptionKeys)
}

// performCleanup handles the internal cleanup state changes
func (s *Subscriber) performCleanup() {
	s.cleanedUp = true
	s.isWaiting = false

	if !s.completed {
		s.completed = true
		s.closeWaitChannelSafely()
	}
}

// getSubscriptionKeys returns a copy of all subscription keys
func (s *Subscriber) getSubscriptionKeys() []string {
	keys := make([]string, 0, len(s.subscriptions))
	for key := range s.subscriptions {
		keys = append(keys, key)
	}
	return keys
}

// closeWaitChannelSafely closes the wait channel if not already closed
func (s *Subscriber) closeWaitChannelSafely() {
	select {
	case <-s.waitCh:
		// Already closed
	default:
		close(s.waitCh)
	}
}

// removeFromTopics removes this subscriber from all specified topics
func (s *Subscriber) removeFromTopics(keys []string) {
	for _, key := range keys {
		s.removeFromTopic(key)
	}
}

// forceCleanup is used during PubSub shutdown to forcefully clean up subscribers
func (s *Subscriber) forceCleanup() {
	if s == nil {
		return
	}

	defer func() {
		if r := recover(); r != nil {
			log.Printf("PubSub: Panic during force cleanup: %v", r)
		}
	}()

	s.mutex.Lock()
	s.performCleanup()
	subscriptionKeys := s.getSubscriptionKeys()
	s.mutex.Unlock()

	s.removeFromTopics(subscriptionKeys)
}

// removeFromTopic removes this subscriber from the specified topic
func (s *Subscriber) removeFromTopic(key string) {
	if s == nil || s.pubsub == nil {
		return
	}

	s.pubsub.mutex.Lock()
	topic, exists := s.pubsub.topics[key]
	if !exists {
		s.pubsub.mutex.Unlock()
		return
	}

	topic.mutex.Lock()
	delete(topic.subscribers, s)
	isEmpty := len(topic.subscribers) == 0
	topic.mutex.Unlock()

	// Clean up empty topics if PubSub is not being closed
	if isEmpty && !s.pubsub.closed {
		delete(s.pubsub.topics, key)
	}
	s.pubsub.mutex.Unlock()
}

// getResults returns a copy of the current results
func (s *Subscriber) getResults() map[string]interface{} {
	if s == nil {
		return make(map[string]interface{})
	}

	s.mutex.RLock()
	defer s.mutex.RUnlock()

	// Return a copy to avoid race conditions
	results := make(map[string]interface{})
	for key, value := range s.results {
		results[key] = value
	}
	return results
}

// Hash generates an MD5 hash of the provided key
func (ps *PubSub) Hash(key string) string {
	hash := md5.Sum([]byte(key))
	return hex.EncodeToString(hash[:])
}

// GetTopicCount returns the number of active topics (for testing)
func (ps *PubSub) GetTopicCount() int {
	ps.mutex.RLock()
	defer ps.mutex.RUnlock()
	return len(ps.topics)
}

// GetSubscriberCount returns the number of subscribers for a topic (for testing)
func (ps *PubSub) GetSubscriberCount(key string) int {
	ps.mutex.RLock()
	topic, exists := ps.topics[key]
	ps.mutex.RUnlock()

	if !exists {
		return 0
	}

	topic.mutex.RLock()
	defer topic.mutex.RUnlock()
	return len(topic.subscribers)
}
