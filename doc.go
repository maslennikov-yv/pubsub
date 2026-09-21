// Package pubsub implements a small, linearizable publish/subscribe hub whose
// subscribers wait for a set of keyed events with a timeout.
//
// A typical use is polling a group of sensors: subscribe to one key per
// sensor, publish each reading as it arrives, and Wait returns as soon as
// every key has a value or the timeout elapses, whichever comes first.
//
// # Contract
//
// Every operation applies its effect inside a single hub-wide critical
// section, so each is atomic with respect to every other and the hub is
// linearizable: the outcome of a scenario is a function of the order in which
// calls acquire that section, not of how their goroutines are interleaved
// inside it. Two qualifications. Wait releases the lock while it is parked, so
// Subscribe, Publish and Close all run between its two critical sections. And
// a blocked Wait is woken by a wall-clock timer whose expiry the program does
// not order against concurrent calls - the linearization rule below is what
// keeps results consistent either way. The price of the single section is that
// all calls, including Publish on unrelated keys, are serialized per hub;
// callers that need parallel fan-out should shard across several PubSub
// instances.
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
// Wait returns a fresh copy of the results map collected so far. The copy is
// shallow: payloads are handed back exactly as they were published, so every
// subscriber of a key - and the publisher - hold the same value. A key
// published with a nil payload is present with a nil value, which only the
// comma-ok form distinguishes from a key that received nothing. Wait's
// linearization point is its finalization: a payload published between the
// timer expiring and finalization is included, and that Publish returned true,
// so results and return values are always mutually consistent.
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
// them finishes it, so what governs is the earliest deadline among the Wait
// calls started while the subscriber was live - a deadline being the moment a
// Wait was entered plus its timeout, or the moment of the call itself when the
// timeout is <= 0. That is not the shortest timeout: a Wait(50ms) started
// later than a Wait(100ms) has the later deadline and is cut off by it. A Wait
// started after the subscriber finished changes nothing. A woken Wait must
// re-acquire the lock before returning, so no Wait outlives that earliest
// deadline - though any Wait can of course return sooner, when a Publish
// completes the subscriber or Close intervenes. Cleanup happens only on Wait
// or Close; a registered subscriber that never calls Wait stays registered
// until Close.
//
// IsClosed, GetTopicCount and GetSubscriberCount are observers and Hash is a
// pure helper. The two counters report live subscribers and the keys they
// hold, and they take the lock for the read alone, so what they return already
// describes a moment that has passed: treat them as advisory - useful in tests
// and metrics - never as the basis for a decision another goroutine can
// invalidate. Hash ignores its receiver; it is the hex-encoded MD5 of its
// argument, identical for every hub, and is not a security primitive.
//
// The package never panics or logs for values obtained from NewPubSub and
// NewSubscriber (including the nil Subscriber returned after Close); invalid
// operations are no-ops with defined return values.
package pubsub
