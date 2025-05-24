package pubsub

import (
	"crypto/md5"
	"encoding/hex"
	"log"
	"sync"
	"time"
)

type PubSub struct {
	mutex     sync.RWMutex
	topics    map[string]*Topic
	closed    bool
	closeCh   chan struct{}
	closeOnce sync.Once
}

func NewPubSub() *PubSub {
	return &PubSub{
		topics:  make(map[string]*Topic),
		closed:  false,
		closeCh: make(chan struct{}),
	}
}

type Subscriber struct {
	ps            *PubSub
	mutex         sync.RWMutex
	subscriptions map[string]bool
	results       map[string]interface{}
	waitCh        chan struct{}
	completed     bool
	cleanedUp     bool
	expectedCount int
	receivedCount int
}

type Topic struct {
	mutex       sync.RWMutex
	subscribers map[*Subscriber]bool
}

// Close gracefully shuts down the PubSub system
func (ps *PubSub) Close() error {
	var closeErr error

	ps.closeOnce.Do(func() {
		ps.mutex.Lock()
		defer ps.mutex.Unlock()

		if ps.closed {
			return
		}

		log.Println("PubSub: Starting graceful shutdown...")

		// Mark as closed to prevent new operations
		ps.closed = true

		// Close the close channel to signal shutdown - this will interrupt all Wait() calls
		close(ps.closeCh)

		// Give a very short time for subscribers to react to the close signal
		time.Sleep(5 * time.Millisecond)

		// Collect all active subscribers
		var allSubscribers []*Subscriber
		subscriberMap := make(map[*Subscriber]bool)

		for topicKey, topic := range ps.topics {
			topic.mutex.RLock()
			for sub := range topic.subscribers {
				if !subscriberMap[sub] {
					allSubscribers = append(allSubscribers, sub)
					subscriberMap[sub] = true
				}
			}
			topic.mutex.RUnlock()
			log.Printf("PubSub: Found %d subscribers for topic '%s'", len(topic.subscribers), topicKey)
		}

		log.Printf("PubSub: Cleaning up %d unique subscribers across %d topics", len(allSubscribers), len(ps.topics))

		// Force cleanup of all subscribers with a much shorter timeout
		var wg sync.WaitGroup
		for _, sub := range allSubscribers {
			wg.Add(1)
			go func(subscriber *Subscriber) {
				defer wg.Done()
				defer func() {
					if r := recover(); r != nil {
						log.Printf("PubSub: Panic during subscriber cleanup: %v", r)
					}
				}()

				subscriber.forceCleanup()
			}(sub)
		}

		// Wait for all subscriber cleanups to complete with a very short timeout
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			log.Println("PubSub: All subscribers cleaned up successfully")
		case <-time.After(100 * time.Millisecond): // Very short timeout - 100ms
			log.Println("PubSub: Warning - subscriber cleanup timed out after 100ms")
			closeErr = &PubSubError{
				Op:  "Close",
				Err: "subscriber cleanup timeout",
			}
		}

		// Clear all topics regardless of cleanup timeout
		topicCount := len(ps.topics)
		for key := range ps.topics {
			delete(ps.topics, key)
		}

		log.Printf("PubSub: Cleared %d topics", topicCount)
		log.Println("PubSub: Shutdown completed")
	})

	return closeErr
}

// IsClosed returns true if the PubSub instance has been closed
func (ps *PubSub) IsClosed() bool {
	ps.mutex.RLock()
	defer ps.mutex.RUnlock()
	return ps.closed
}

// PubSubError represents an error that occurred during PubSub operations
type PubSubError struct {
	Op  string
	Err string
}

func (e *PubSubError) Error() string {
	return "pubsub " + e.Op + ": " + e.Err
}

func (ps *PubSub) NewSubscriber() *Subscriber {
	ps.mutex.RLock()
	defer ps.mutex.RUnlock()

	if ps.closed {
		log.Println("PubSub: Warning - attempted to create subscriber on closed PubSub instance")
		return nil
	}

	return &Subscriber{
		ps:            ps,
		subscriptions: make(map[string]bool),
		results:       make(map[string]interface{}),
		waitCh:        make(chan struct{}),
		completed:     false,
		cleanedUp:     false,
	}
}

func (s *Subscriber) Subscribe(key string) {
	if s == nil {
		log.Println("PubSub: Warning - attempted to subscribe with nil subscriber")
		return
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.completed {
		return // Don't allow subscription after completion
	}

	// Check if PubSub is closed
	s.ps.mutex.RLock()
	if s.ps.closed {
		s.ps.mutex.RUnlock()
		log.Printf("PubSub: Warning - attempted to subscribe to '%s' on closed PubSub instance", key)
		return
	}
	s.ps.mutex.RUnlock()

	if s.subscriptions[key] {
		return // Already subscribed
	}

	s.subscriptions[key] = true
	s.expectedCount++

	// Add to topic
	s.ps.mutex.Lock()
	topic, exists := s.ps.topics[key]
	if !exists {
		topic = &Topic{
			subscribers: make(map[*Subscriber]bool),
		}
		s.ps.topics[key] = topic
	}
	s.ps.mutex.Unlock()

	topic.mutex.Lock()
	topic.subscribers[s] = true
	topic.mutex.Unlock()
}

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

	topic.mutex.RLock()
	subscribers := make([]*Subscriber, 0, len(topic.subscribers))
	for sub := range topic.subscribers {
		subscribers = append(subscribers, sub)
	}
	topic.mutex.RUnlock()

	if len(subscribers) == 0 {
		return false
	}

	delivered := false
	for _, sub := range subscribers {
		if sub.deliverMessage(key, payload) {
			delivered = true
		}
	}

	return delivered
}

func (s *Subscriber) deliverMessage(key string, payload interface{}) bool {
	if s == nil {
		return false
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.completed {
		return false
	}

	if !s.subscriptions[key] {
		return false
	}

	// Check if this is a new message for this key
	wasNew := false
	if _, exists := s.results[key]; !exists {
		s.receivedCount++
		wasNew = true
	}

	// Always update the payload (overwrite behavior)
	s.results[key] = payload

	// If we've received all expected messages, signal completion
	if wasNew && s.receivedCount >= s.expectedCount {
		s.completed = true
		close(s.waitCh)
	}

	return true
}

func (s *Subscriber) Wait(timeout time.Duration) map[string]interface{} {
	if s == nil {
		log.Println("PubSub: Warning - Wait called on nil subscriber")
		return make(map[string]interface{})
	}

	// Check if PubSub is closed
	select {
	case <-s.ps.closeCh:
		log.Println("PubSub: Wait interrupted due to PubSub shutdown")
		s.cleanup()
		return s.getResults()
	default:
	}

	if timeout <= 0 {
		s.cleanup()
		return s.getResults()
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-s.waitCh:
		// All messages received
	case <-timer.C:
		// Timeout occurred
	case <-s.ps.closeCh:
		// PubSub is being closed
		log.Println("PubSub: Wait interrupted due to PubSub shutdown")
	}

	s.cleanup()
	return s.getResults()
}

func (s *Subscriber) cleanup() {
	if s == nil {
		return
	}

	s.mutex.Lock()

	// Check if already cleaned up
	if s.cleanedUp {
		s.mutex.Unlock()
		return
	}

	// Mark as cleaned up first
	s.cleanedUp = true

	// Mark as completed if not already
	if !s.completed {
		s.completed = true
		// Close waitCh if not already closed
		select {
		case <-s.waitCh:
			// Already closed
		default:
			close(s.waitCh)
		}
	}

	subscriptions := make([]string, 0, len(s.subscriptions))
	for key := range s.subscriptions {
		subscriptions = append(subscriptions, key)
	}
	s.mutex.Unlock()

	// Remove subscriber from all topics
	for _, key := range subscriptions {
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

	// Mark as cleaned up and completed
	s.cleanedUp = true
	s.completed = true

	// Close waitCh if not already closed
	select {
	case <-s.waitCh:
		// Already closed
	default:
		close(s.waitCh)
	}

	subscriptions := make([]string, 0, len(s.subscriptions))
	for key := range s.subscriptions {
		subscriptions = append(subscriptions, key)
	}
	s.mutex.Unlock()

	// Remove subscriber from all topics
	for _, key := range subscriptions {
		s.removeFromTopic(key)
	}
}

func (s *Subscriber) removeFromTopic(key string) {
	if s == nil || s.ps == nil {
		return
	}

	s.ps.mutex.Lock()
	topic, exists := s.ps.topics[key]
	if !exists {
		s.ps.mutex.Unlock()
		return
	}

	topic.mutex.Lock()
	delete(topic.subscribers, s)
	isEmpty := len(topic.subscribers) == 0
	topic.mutex.Unlock()

	// Clean up empty topics immediately while holding the pubsub lock
	// But only if PubSub is not being closed (to avoid interference with Close method)
	if isEmpty && !s.ps.closed {
		delete(s.ps.topics, key)
	}
	s.ps.mutex.Unlock()
}

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
