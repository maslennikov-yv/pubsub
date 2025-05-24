package pubsub

import (
	"crypto/md5"
	"encoding/hex"
	"sync"
	"time"
)

type PubSub struct {
	mutex  sync.RWMutex
	topics map[string]*Topic
}

func NewPubSub() *PubSub {
	return &PubSub{
		topics: make(map[string]*Topic),
	}
}

type Subscriber struct {
	ps            *PubSub
	mutex         sync.RWMutex
	subscriptions map[string]bool
	results       map[string]interface{}
	waitCh        chan struct{}
	completed     bool
	cleanedUp     bool // Add flag to track cleanup state
	expectedCount int
	receivedCount int
}

type Topic struct {
	mutex       sync.RWMutex
	subscribers map[*Subscriber]bool
}

func (ps *PubSub) NewSubscriber() *Subscriber {
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
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.completed {
		return // Don't allow subscription after completion
	}

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
	}

	s.cleanup()
	return s.getResults()
}

func (s *Subscriber) cleanup() {
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

func (s *Subscriber) removeFromTopic(key string) {
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
	if isEmpty {
		delete(s.ps.topics, key)
	}
	s.ps.mutex.Unlock()
}

func (s *Subscriber) getResults() map[string]interface{} {
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
