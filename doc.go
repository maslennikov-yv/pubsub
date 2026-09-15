// Package pubsub implements a small, deterministic publish/subscribe hub
// whose subscribers wait for a set of keyed events with a timeout.
//
// A typical use is polling a group of sensors: subscribe to one key per
// sensor, publish each reading as it arrives, and Wait returns as soon as
// every key has a value or the timeout elapses, whichever comes first.
//
// # Contract
//
// Every operation runs inside a single hub-wide critical section, so the
// outcome of any scenario is a pure function of the order in which calls
// acquire it, never of goroutine scheduling. The price is that all calls,
// including Publish on unrelated keys, are serialized per hub; callers that
// need parallel fan-out should shard across several PubSub instances.
//
// A Subscriber is live from NewSubscriber until it finishes. Finishing is a
// single terminal transition: the subscriber stops accepting messages and is
// removed from every topic (topics left empty are deleted). It happens at
// exactly one of these moments:
//
//  1. Entry to Wait, if the hub is closed, the timeout is <= 0, or the
//     subscriber is already complete (every subscribed key has a value; a
//     subscriber with no subscriptions is trivially complete).
//  2. A Publish that makes the subscriber complete while a Wait is blocked.
//  3. Expiry of a blocked Wait's timer.
//  4. Close.
//
// Becoming complete before Wait is called is not a trigger: the subscriber
// stays live, keeps accepting overwrites, and accepts further Subscribe calls.
//
// Publish returns true if and only if the hub is open and at least one live
// subscriber holds the key. Every such subscriber stores the payload under
// the key, replacing any earlier value ("latest wins"). A true result
// guarantees the payload appears in that subscriber's Wait result unless a
// later successful Publish overwrote it; a false result guarantees nobody
// observed it.
//
// Wait returns a fresh copy of the results collected so far. Its
// linearization point is its finalization: a payload published between the
// timer expiring and finalization is included, and that Publish returned
// true, so results and return values are always mutually consistent.
//
// Close is synchronous, idempotent, and always returns nil. It finishes every
// registered subscriber (one holding at least one subscription; a subscriber
// with none is in no topic and finishes on its first Wait), waking blocked
// Wait calls immediately with whatever was collected before Close. Afterwards
// NewSubscriber returns nil, Publish returns false, Subscribe is a no-op, and
// Wait never blocks.
//
// A subscriber is single-use: after it finishes, Subscribe is a no-op and
// repeated Wait calls return the same snapshot immediately. Concurrent Wait
// calls on one subscriber all return that snapshot as soon as the first of
// them finishes it, so the effective timeout is the shortest one. Cleanup happens
// only on Wait or Close; a subscriber that never calls Wait stays registered
// until Close.
//
// The package never panics or logs for values obtained from NewPubSub and
// NewSubscriber (including the nil Subscriber returned after Close); invalid
// operations are no-ops with defined return values.
package pubsub
