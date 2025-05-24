package pubsub

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// PubSub represents the main pub-sub hub - API compatible version
type PubSub struct {
	mu          sync.RWMutex
	subscribers map[string]map[*Subscriber]*subscriberInfo
	closed      int32
	logger      *logger
}

// Simple internal logger to avoid external dependencies
type logger struct {
	enabled bool
}

func (l *logger) info(msg string, args ...interface{}) {
	if l.enabled {
		log.Printf("[INFO] "+msg, args...)
	}
}

func (l *logger) error(msg string, args ...interface{}) {
	if l.enabled {
		log.Printf("[ERROR] "+msg, args...)
	}
}

func (l *logger) warn(msg string, args ...interface{}) {
	if l.enabled {
		log.Printf("[WARN] "+msg, args...)
	}
}

// subscriberInfo holds channel and context for lifecycle management
type subscriberInfo struct {
	ch     chan interface{}
	cancel context.CancelFunc
	ctx    context.Context
}

// Subscriber represents a subscriber - API compatible
type Subscriber struct {
	hub         *PubSub
	mu          sync.RWMutex
	subscribed  map[string]bool
	events      map[string]interface{}
	eventsMu    sync.RWMutex
	closed      int32
	eventNotify chan struct{}
	wg          sync.WaitGroup
}

// NewPubSub creates a new pub-sub hub - EXACT API match
func NewPubSub() *PubSub {
	return &PubSub{
		subscribers: make(map[string]map[*Subscriber]*subscriberInfo),
		logger:      &logger{enabled: false}, // Disabled by default for API compatibility
	}
}

// NewSubscriber creates a new subscriber - EXACT API match
func (ps *PubSub) NewSubscriber() *Subscriber {
	// Always return a valid subscriber for API compatibility
	return &Subscriber{
		hub:         ps,
		subscribed:  make(map[string]bool),
		events:      make(map[string]interface{}),
		eventNotify: make(chan struct{}, 1),
	}
}

// Subscribe subscribes to events by string key - EXACT API match (no error return)
func (s *Subscriber) Subscribe(key string) {
	// Validate input silently for API compatibility
	if key == "" || atomic.LoadInt32(&s.closed) == 1 {
		return
	}

	s.mu.Lock()
	s.subscribed[key] = true
	s.mu.Unlock()

	// Create context for goroutine lifecycle management
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan interface{}, 1)

	info := &subscriberInfo{
		ch:     ch,
		cancel: cancel,
		ctx:    ctx,
	}

	// Add to hub subscribers with improved locking
	s.hub.mu.Lock()
	if atomic.LoadInt32(&s.hub.closed) == 1 {
		s.hub.mu.Unlock()
		cancel()
		return
	}

	if _, exists := s.hub.subscribers[key]; !exists {
		s.hub.subscribers[key] = make(map[*Subscriber]*subscriberInfo)
	}
	s.hub.subscribers[key][s] = info
	s.hub.mu.Unlock()

	// Start goroutine with proper tracking
	s.wg.Add(1)
	go s.handleEvents(key, info)
}

// handleEvents processes incoming events with improved error handling
func (s *Subscriber) handleEvents(key string, info *subscriberInfo) {
	defer func() {
		s.wg.Done()
		if r := recover(); r != nil {
			s.hub.logger.error("panic in handleEvents for key %s: %v", key, r)
		}
	}()

	for {
		select {
		case data, ok := <-info.ch:
			if !ok {
				return
			}

			s.eventsMu.Lock()
			if atomic.LoadInt32(&s.closed) == 0 {
				s.events[key] = data // Overwrite with latest event (as per README)
				// Notify waiters about new event (non-blocking)
				select {
				case s.eventNotify <- struct{}{}:
				default:
				}
			}
			s.eventsMu.Unlock()

		case <-info.ctx.Done():
			return
		}
	}
}

// Wait waits for events until timeout - EXACT API match
func (s *Subscriber) Wait(timeout time.Duration) map[string]interface{} {
	if atomic.LoadInt32(&s.closed) == 1 {
		return make(map[string]interface{})
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	s.mu.RLock()
	subscribedKeys := make([]string, 0, len(s.subscribed))
	for key := range s.subscribed {
		subscribedKeys = append(subscribedKeys, key)
	}
	s.mu.RUnlock()

	// If no subscriptions, return empty map immediately
	if len(subscribedKeys) == 0 {
		return make(map[string]interface{})
	}

	// Event-driven waiting instead of polling (performance improvement)
	for {
		// Check if we have all events (early completion as per README)
		if s.hasAllEvents(subscribedKeys) {
			return s.getEvents()
		}

		select {
		case <-ctx.Done():
			// Timeout occurred, return partial results (as per README)
			return s.getEvents()

		case <-s.eventNotify:
			// New event received, check again
			continue
		}
	}
}

// hasAllEvents checks if all subscribed events have been received
func (s *Subscriber) hasAllEvents(keys []string) bool {
	s.eventsMu.RLock()
	defer s.eventsMu.RUnlock()

	for _, key := range keys {
		if _, exists := s.events[key]; !exists {
			return false
		}
	}
	return true
}

// getEvents returns a copy of current events
func (s *Subscriber) getEvents() map[string]interface{} {
	s.eventsMu.RLock()
	defer s.eventsMu.RUnlock()

	result := make(map[string]interface{}, len(s.events))
	for key, value := range s.events {
		result[key] = value
	}
	return result
}

// Publish publishes an event - EXACT API match (returns only bool)
func (ps *PubSub) Publish(key string, data interface{}) bool {
	// Validate input silently for API compatibility
	if key == "" || atomic.LoadInt32(&ps.closed) == 1 {
		return false
	}

	ps.mu.RLock()
	subscribers, exists := ps.subscribers[key]
	if !exists || len(subscribers) == 0 {
		ps.mu.RUnlock()
		return false // No subscribers (as per README)
	}

	// Create a copy to avoid holding lock during delivery
	subscribersCopy := make([]*subscriberInfo, 0, len(subscribers))
	for _, info := range subscribers {
		subscribersCopy = append(subscribersCopy, info)
	}
	ps.mu.RUnlock()

	delivered := false
	for _, info := range subscribersCopy {
		select {
		case info.ch <- data:
			delivered = true
		default:
			// Channel is full, replace with new data (event overwriting)
			select {
			case <-info.ch:
				info.ch <- data
				delivered = true
			default:
				// Still full, skip this subscriber but don't fail completely
			}
		}
	}

	return delivered // Returns true if at least one subscriber received (as per README)
}

// Unsubscribe removes subscription from a key
func (s *Subscriber) Unsubscribe(key string) {
	if key == "" {
		return
	}

	s.mu.Lock()
	delete(s.subscribed, key)
	s.mu.Unlock()

	s.hub.mu.Lock()
	defer s.hub.mu.Unlock()

	if subscribers, exists := s.hub.subscribers[key]; exists {
		if info, exists := subscribers[s]; exists {
			info.cancel()
			close(info.ch)
			delete(subscribers, s)

			// Clean up empty subscriber maps
			if len(subscribers) == 0 {
				delete(s.hub.subscribers, key)
			}
		}
	}
}

// Close closes the subscriber and cleans up resources
func (s *Subscriber) Close() {
	if !atomic.CompareAndSwapInt32(&s.closed, 0, 1) {
		return // Already closed
	}

	s.mu.RLock()
	keys := make([]string, 0, len(s.subscribed))
	for key := range s.subscribed {
		keys = append(keys, key)
	}
	s.mu.RUnlock()

	// Unsubscribe from all keys
	for _, key := range keys {
		s.Unsubscribe(key)
	}

	// Wait for all goroutines to finish
	s.wg.Wait()

	// Clear events and close notification channel
	s.eventsMu.Lock()
	s.events = make(map[string]interface{})
	s.eventsMu.Unlock()

	close(s.eventNotify)
}

// Close closes the pub-sub hub and all subscribers
func (ps *PubSub) Close() {
	if !atomic.CompareAndSwapInt32(&ps.closed, 0, 1) {
		return // Already closed
	}

	ps.mu.Lock()
	defer ps.mu.Unlock()

	// Close all channels and cancel contexts
	for _, subscribers := range ps.subscribers {
		for _, info := range subscribers {
			info.cancel()
			close(info.ch)
		}
	}

	// Clear subscribers
	ps.subscribers = make(map[string]map[*Subscriber]*subscriberInfo)
}

// EnableLogging enables internal logging (optional feature)
func (ps *PubSub) EnableLogging() {
	ps.logger.enabled = true
}

// DisableLogging disables internal logging
func (ps *PubSub) DisableLogging() {
	ps.logger.enabled = false
}
