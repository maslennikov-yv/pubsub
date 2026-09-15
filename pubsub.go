package pubsub

import (
	"crypto/md5" //nolint:gosec // non-cryptographic key fingerprint, part of the public API
	"encoding/hex"
	"maps"
	"sync"
	"time"
)

// PubSub is a thread-safe publish/subscribe hub. Create one with NewPubSub.
type PubSub struct {
	mu     sync.RWMutex
	topics map[string]*topic
	closed bool
}

// topic is the set of subscribers registered for one key. It is guarded by
// the owning PubSub's lock.
type topic struct {
	subscribers map[*Subscriber]struct{}
}

// Subscriber collects the latest payload for each subscribed key until Wait
// returns or the hub closes. Create one with (*PubSub).NewSubscriber. All
// fields are guarded by the owning PubSub's lock.
type Subscriber struct {
	pubsub        *PubSub
	subscriptions map[string]bool
	results       map[string]any
	waitCh        chan struct{} // closed exactly once, when the subscriber finishes
	waiting       bool          // a Wait call is blocked on waitCh
	done          bool          // finished; terminal
}

// NewPubSub creates an open hub with no topics.
func NewPubSub() *PubSub {
	return &PubSub{topics: make(map[string]*topic)}
}

// NewSubscriber creates a live subscriber with no subscriptions. It returns
// nil if the hub is closed; the nil Subscriber is safe to use (Subscribe is a
// no-op, Wait returns an empty map).
func (ps *PubSub) NewSubscriber() *Subscriber {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	if ps.closed {
		return nil
	}
	return &Subscriber{
		pubsub:        ps,
		subscriptions: make(map[string]bool),
		results:       make(map[string]any),
		waitCh:        make(chan struct{}),
	}
}

// Subscribe registers interest in key. It is a no-op on a nil subscriber, a
// closed hub, a finished subscriber, or a key already subscribed. It takes
// effect even while a Wait on this subscriber is blocked, extending the set
// of keys Wait requires for completion.
func (s *Subscriber) Subscribe(key string) {
	if s == nil {
		return
	}
	ps := s.pubsub
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if ps.closed || s.done || s.subscriptions[key] {
		return
	}
	s.subscriptions[key] = true
	ps.topicLocked(key).subscribers[s] = struct{}{}
}

// Publish delivers payload to every live subscriber of key, replacing any
// earlier payload for that key. It returns true if at least one subscriber
// received it and false if the hub is closed or nobody is subscribed.
func (ps *PubSub) Publish(key string, payload any) bool {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if ps.closed {
		return false
	}
	topic, ok := ps.topics[key]
	if !ok {
		return false
	}

	// Ranging directly is safe: finishLocked only removes the current s from
	// topic.subscribers (deleting the current entry during range is defined),
	// and the local topic pointer stays valid if the key is dropped from
	// ps.topics.
	delivered := false
	for s := range topic.subscribers {
		s.results[key] = payload
		delivered = true
		if s.waiting && s.completeLocked() {
			ps.finishLocked(s)
		}
	}
	return delivered
}

// Wait blocks until every subscribed key has a value, timeout elapses, or the
// hub closes, then finishes the subscriber and returns a copy of the results
// (absent keys received nothing). A timeout <= 0 returns immediately. Once
// finished, further calls return the same snapshot without blocking.
// Concurrent Wait calls on one subscriber all return the same snapshot as
// soon as the first of them finishes it. Wait on a nil subscriber returns an
// empty map.
func (s *Subscriber) Wait(timeout time.Duration) map[string]any {
	if s == nil {
		return make(map[string]any)
	}
	ps := s.pubsub

	ps.mu.Lock()
	if !s.done && !ps.closed && timeout > 0 && !s.completeLocked() {
		s.waiting = true
		ps.mu.Unlock()

		timer := time.NewTimer(timeout)
		defer timer.Stop()
		select {
		case <-s.waitCh:
		case <-timer.C:
		}

		ps.mu.Lock()
	}
	defer ps.mu.Unlock()
	// Finalization is the linearization point: whether we got here directly,
	// via waitCh, or via the timer, finish s if nobody else has.
	if !s.done {
		ps.finishLocked(s)
	}
	// s.results is never nil, so the copy is never nil either.
	return maps.Clone(s.results)
}

// Close shuts the hub down: it finishes every registered subscriber, waking
// blocked Wait calls immediately, and drops all topics. It is synchronous,
// idempotent, and always returns nil.
func (ps *PubSub) Close() error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if ps.closed {
		return nil
	}
	ps.closed = true
	// finishLocked detaches s from every topic it holds and drops topics left
	// empty, so a multi-topic subscriber is visited once and ps.topics is empty
	// when the loop ends. Deleting map entries during range is defined.
	for _, topic := range ps.topics {
		for s := range topic.subscribers {
			ps.finishLocked(s)
		}
	}
	return nil
}

// IsClosed reports whether Close has been called.
func (ps *PubSub) IsClosed() bool {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return ps.closed
}

// Hash returns the hex-encoded MD5 digest of key. It is a convenience for
// deriving fixed-length topic keys and is not a security primitive.
func (ps *PubSub) Hash(key string) string {
	sum := md5.Sum([]byte(key)) //nolint:gosec // non-cryptographic key fingerprint
	return hex.EncodeToString(sum[:])
}

// GetTopicCount returns the number of keys that currently have at least one
// live subscriber.
func (ps *PubSub) GetTopicCount() int {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return len(ps.topics)
}

// GetSubscriberCount returns the number of live subscribers of key.
func (ps *PubSub) GetSubscriberCount(key string) int {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	if topic, ok := ps.topics[key]; ok {
		return len(topic.subscribers)
	}
	return 0
}

// topicLocked returns the topic for key, creating it if needed.
// Caller holds ps.mu for writing.
func (ps *PubSub) topicLocked(key string) *topic {
	t, ok := ps.topics[key]
	if !ok {
		t = &topic{subscribers: make(map[*Subscriber]struct{})}
		ps.topics[key] = t
	}
	return t
}

// finishLocked performs the terminal transition for s: it stops accepting
// messages, wakes a blocked Wait, and detaches s from every topic. It is the
// only place that sets done or closes waitCh.
// Caller holds ps.mu for writing and has checked !s.done.
//
// Invariant relied upon: while s is live, every key in s.subscriptions has a
// topic that contains s. Subscribe adds both under the lock and only
// finishLocked removes s, so ps.topics[key] is always present here.
//
// Constraint on this function: Publish and Close call it while ranging over
// topic.subscribers and ps.topics. It must only delete s itself (and topics
// left empty by that), never insert entries or remove other subscribers, or
// those loops would silently skip live subscribers.
func (ps *PubSub) finishLocked(s *Subscriber) {
	s.done = true
	s.waiting = false
	close(s.waitCh)
	for key := range s.subscriptions {
		topic := ps.topics[key]
		delete(topic.subscribers, s)
		if len(topic.subscribers) == 0 {
			delete(ps.topics, key)
		}
	}
}

// completeLocked reports whether every subscribed key has a value.
// results is always a subset of subscriptions, so comparing sizes suffices.
// Caller holds ps.mu.
func (s *Subscriber) completeLocked() bool {
	return len(s.results) == len(s.subscriptions)
}
