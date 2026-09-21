package pubsub

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

// These tests pin the behavioural contract documented in the package
// comment. Timing assertions use only generous upper bounds so they stay
// stable under -race and on slow CI runners.

const generous = 500 * time.Millisecond

// waitInGoroutine runs sub.Wait(timeout) in a goroutine. It returns a channel
// that yields the result and a function that blocks the calling test until
// that Wait is parked on its channel (waiting == true), failing the test if
// this does not happen within the generous bound. Tests that publish "while
// Wait is blocked" must call it first so they never race Wait's entry.
func waitInGoroutine(t *testing.T, sub *Subscriber, timeout time.Duration) (<-chan map[string]any, func()) {
	t.Helper()
	out := make(chan map[string]any, 1)
	go func() { out <- sub.Wait(timeout) }()
	awaitBlocked := func() {
		t.Helper()
		deadline := time.Now().Add(generous)
		for {
			sub.pubsub.mu.RLock()
			waiting := sub.waiting
			sub.pubsub.mu.RUnlock()
			if waiting {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("Wait did not block within %v", generous)
			}
			time.Sleep(50 * time.Microsecond)
		}
	}
	return out, awaitBlocked
}

func recvWithin(t *testing.T, ch <-chan map[string]any) map[string]any {
	t.Helper()
	select {
	case r := <-ch:
		return r
	case <-time.After(generous):
		t.Fatalf("Wait did not return within %v", generous)
		return nil
	}
}

func TestWaitZeroSubscriptionsReturnsImmediately(t *testing.T) {
	ps := NewPubSub()
	defer ps.Close()

	sub := ps.NewSubscriber()
	out, _ := waitInGoroutine(t, sub, 5*time.Second)
	res := recvWithin(t, out)
	if len(res) != 0 {
		t.Fatalf("expected empty results, got %v", res)
	}
	if ps.GetTopicCount() != 0 {
		t.Fatalf("expected 0 topics, got %d", ps.GetTopicCount())
	}
}

func TestNegativeTimeoutBehavesAsZero(t *testing.T) {
	ps := NewPubSub()
	defer ps.Close()

	sub := ps.NewSubscriber()
	sub.Subscribe("a")
	sub.Subscribe("b")
	ps.Publish("a", 1)

	out, _ := waitInGoroutine(t, sub, -time.Second)
	res := recvWithin(t, out)
	if len(res) != 1 || res["a"] != 1 {
		t.Fatalf("expected {a:1}, got %v", res)
	}
	if ps.GetTopicCount() != 0 {
		t.Fatalf("expected 0 topics after finish, got %d", ps.GetTopicCount())
	}
}

func TestWaitTwiceReturnsSameSnapshot(t *testing.T) {
	ps := NewPubSub()
	defer ps.Close()

	sub := ps.NewSubscriber()
	sub.Subscribe("a")
	ps.Publish("a", "v")

	first := sub.Wait(time.Second)
	first["a"] = "mutated"
	first["injected"] = true

	out, _ := waitInGoroutine(t, sub, 5*time.Second)
	second := recvWithin(t, out)
	if len(second) != 1 || second["a"] != "v" {
		t.Fatalf("second Wait must return the original snapshot, got %v", second)
	}
}

func TestConcurrentWaitsShareSnapshot(t *testing.T) {
	ps := NewPubSub()
	defer ps.Close()

	sub := ps.NewSubscriber()
	sub.Subscribe("a")
	sub.Subscribe("b")
	ps.Publish("a", 1)

	long, awaitBlocked := waitInGoroutine(t, sub, time.Hour)
	awaitBlocked()
	short := sub.Wait(20 * time.Millisecond) // expires first and finishes sub

	// The long Wait is woken by the short one's finalization, not its own timer.
	res := recvWithin(t, long)
	if len(res) != 1 || res["a"] != 1 || len(short) != 1 || short["a"] != 1 {
		t.Fatalf("expected both Waits to return {a:1}, got long=%v short=%v", res, short)
	}
	if ps.GetTopicCount() != 0 {
		t.Fatalf("expected 0 topics after finish, got %d", ps.GetTopicCount())
	}
}

func TestWaitNilSubscriber(t *testing.T) {
	var sub *Subscriber
	res := sub.Wait(time.Second)
	if res == nil || len(res) != 0 {
		t.Fatalf("expected non-nil empty map, got %v", res)
	}
}

func TestSubscribeNilSubscriber(_ *testing.T) {
	var sub *Subscriber
	sub.Subscribe("a") // must not panic
}

func TestSubscribeAfterCompletionTakesEffect(t *testing.T) {
	ps := NewPubSub()
	defer ps.Close()

	sub := ps.NewSubscriber()
	sub.Subscribe("a")
	if !ps.Publish("a", 1) {
		t.Fatal("publish a should succeed")
	}
	sub.Subscribe("b") // subscriber is complete but live: must take effect
	if ps.GetSubscriberCount("b") != 1 {
		t.Fatalf("expected b to be subscribed, count=%d", ps.GetSubscriberCount("b"))
	}

	start := time.Now()
	res := sub.Wait(50 * time.Millisecond)
	if time.Since(start) < 45*time.Millisecond {
		t.Fatalf("Wait must block for b, returned after %v", time.Since(start))
	}
	if len(res) != 1 || res["a"] != 1 {
		t.Fatalf("expected {a:1}, got %v", res)
	}
}

func TestSubscribeAfterCompletionThenPublishB(t *testing.T) {
	ps := NewPubSub()
	defer ps.Close()

	sub := ps.NewSubscriber()
	sub.Subscribe("a")
	ps.Publish("a", 1)
	sub.Subscribe("b")
	if !ps.Publish("b", 2) {
		t.Fatal("publish b should succeed")
	}

	out, _ := waitInGoroutine(t, sub, 5*time.Second)
	res := recvWithin(t, out)
	if len(res) != 2 || res["a"] != 1 || res["b"] != 2 {
		t.Fatalf("expected {a:1 b:2}, got %v", res)
	}
}

func TestOverwriteAcceptedBeforeWait(t *testing.T) {
	ps := NewPubSub()
	defer ps.Close()

	sub := ps.NewSubscriber()
	sub.Subscribe("a")
	if !ps.Publish("a", "v1") {
		t.Fatal("first publish should succeed")
	}
	if !ps.Publish("a", "v2") {
		t.Fatal("overwrite before Wait should succeed")
	}
	res := sub.Wait(time.Second)
	if res["a"] != "v2" {
		t.Fatalf("expected latest value v2, got %v", res["a"])
	}
}

func TestPublishRejectedAfterWaitFinished(t *testing.T) {
	ps := NewPubSub()
	defer ps.Close()

	sub := ps.NewSubscriber()
	sub.Subscribe("a")
	out, awaitBlocked := waitInGoroutine(t, sub, 5*time.Second)
	awaitBlocked()

	if !ps.Publish("a", 1) {
		t.Fatal("completing publish should succeed")
	}
	// The completing Publish finished the subscriber synchronously.
	if ps.Publish("a", 2) {
		t.Fatal("publish after the subscriber finished must return false")
	}
	res := recvWithin(t, out)
	if res["a"] != 1 {
		t.Fatalf("expected first value 1, got %v", res["a"])
	}
	if ps.GetTopicCount() != 0 {
		t.Fatalf("expected 0 topics, got %d", ps.GetTopicCount())
	}
}

func TestSubscribeDuringBlockedWaitExtends(t *testing.T) {
	ps := NewPubSub()
	defer ps.Close()

	sub := ps.NewSubscriber()
	sub.Subscribe("a")
	out, awaitBlocked := waitInGoroutine(t, sub, 5*time.Second)
	awaitBlocked()

	sub.Subscribe("b")
	ps.Publish("a", 1)
	select {
	case res := <-out:
		t.Fatalf("Wait returned before b arrived: %v", res)
	case <-time.After(20 * time.Millisecond):
	}
	ps.Publish("b", 2)
	res := recvWithin(t, out)
	if len(res) != 2 || res["a"] != 1 || res["b"] != 2 {
		t.Fatalf("expected {a:1 b:2}, got %v", res)
	}
}

func TestPublishOneCompleteOneIncomplete(t *testing.T) {
	ps := NewPubSub()
	defer ps.Close()

	idle := ps.NewSubscriber() // complete before Wait, stays live
	idle.Subscribe("a")
	ps.Publish("a", "old")

	waiter := ps.NewSubscriber()
	waiter.Subscribe("a")
	out, awaitBlocked := waitInGoroutine(t, waiter, 5*time.Second)
	awaitBlocked()

	if !ps.Publish("a", "new") {
		t.Fatal("publish should succeed")
	}
	res := recvWithin(t, out)
	if res["a"] != "new" {
		t.Fatalf("waiter expected new, got %v", res["a"])
	}
	if ps.GetSubscriberCount("a") != 1 {
		t.Fatalf("idle subscriber must remain registered, count=%d", ps.GetSubscriberCount("a"))
	}
	if idleRes := idle.Wait(0); idleRes["a"] != "new" {
		t.Fatalf("idle subscriber expected overwrite new, got %v", idleRes["a"])
	}
}

func TestSubscribeAfterWaitIsNoop(t *testing.T) {
	ps := NewPubSub()
	defer ps.Close()

	sub := ps.NewSubscriber()
	sub.Subscribe("a")
	sub.Wait(0)
	sub.Subscribe("b")
	if ps.GetTopicCount() != 0 {
		t.Fatalf("Subscribe after Wait must not create topics, got %d", ps.GetTopicCount())
	}
	if ps.Publish("b", 1) {
		t.Fatal("publish to ignored subscription must return false")
	}
}

func TestSubscribeAfterCloseAddsNoTopic(t *testing.T) {
	ps := NewPubSub()
	sub := ps.NewSubscriber()
	if err := ps.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	sub.Subscribe("a")
	if ps.GetTopicCount() != 0 {
		t.Fatalf("expected 0 topics after close, got %d", ps.GetTopicCount())
	}
}

func TestWaitAfterCloseNeverBlocks(t *testing.T) {
	ps := NewPubSub()
	withSubs := ps.NewSubscriber()
	withSubs.Subscribe("a")
	noSubs := ps.NewSubscriber()
	if err := ps.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	for name, sub := range map[string]*Subscriber{"withSubs": withSubs, "noSubs": noSubs} {
		out, _ := waitInGoroutine(t, sub, 5*time.Second)
		res := recvWithin(t, out)
		if len(res) != 0 {
			t.Fatalf("%s: expected empty results, got %v", name, res)
		}
	}
}

func TestCloseReturnsNilWithActiveWaiters(t *testing.T) {
	for i := range 100 {
		ps := NewPubSub()
		var wg sync.WaitGroup
		for j := range 5 {
			sub := ps.NewSubscriber()
			sub.Subscribe(fmt.Sprintf("k%d", j))
			wg.Go(func() { sub.Wait(5 * time.Second) })
		}
		if err := ps.Close(); err != nil {
			t.Fatalf("iteration %d: Close returned %v", i, err)
		}
		wg.Wait()
		assertNoFinishedInTopics(t, ps)
		if ps.GetTopicCount() != 0 {
			t.Fatalf("iteration %d: %d topics after close", i, ps.GetTopicCount())
		}
	}
}

func TestCloseWakesBlockedWait(t *testing.T) {
	ps := NewPubSub()
	sub := ps.NewSubscriber()
	sub.Subscribe("a")
	out, awaitBlocked := waitInGoroutine(t, sub, 5*time.Second)
	awaitBlocked()

	if err := ps.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	res := recvWithin(t, out)
	if len(res) != 0 {
		t.Fatalf("expected empty results, got %v", res)
	}
}

func TestClosePartialResultsPreserved(t *testing.T) {
	ps := NewPubSub()
	sub := ps.NewSubscriber()
	sub.Subscribe("a")
	sub.Subscribe("b")
	ps.Publish("a", 1)
	out, awaitBlocked := waitInGoroutine(t, sub, 5*time.Second)
	awaitBlocked()

	if err := ps.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	res := recvWithin(t, out)
	if len(res) != 1 || res["a"] != 1 {
		t.Fatalf("expected {a:1} preserved across Close, got %v", res)
	}
}

func TestConcurrentCloseAllNil(t *testing.T) {
	ps := NewPubSub()
	sub := ps.NewSubscriber()
	sub.Subscribe("a")

	var wg sync.WaitGroup
	errs := make([]error, 10)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = ps.Close()
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("Close %d returned %v", i, err)
		}
	}
	if !ps.IsClosed() {
		t.Fatal("expected closed")
	}
}

// TestPublishTrueImpliesInResults races a sequential publisher against a
// short Wait and checks the core guarantee: the value in the result is the
// payload of the last Publish that returned true, and every Publish after the
// first false also returns false.
func TestPublishTrueImpliesInResults(t *testing.T) {
	for i := range 1000 {
		ps := NewPubSub()
		sub := ps.NewSubscriber()
		sub.Subscribe("a")

		type outcome struct {
			lastTrue    int
			anyTrue     bool
			falseThenOK bool
		}
		pub := make(chan outcome, 1)
		go func() {
			var o outcome
			o.falseThenOK = true
			seenFalse := false
			for n := 1; n <= 50; n++ {
				ok := ps.Publish("a", n)
				if ok {
					if seenFalse {
						o.falseThenOK = false
					}
					o.anyTrue = true
					o.lastTrue = n
				} else {
					seenFalse = true
				}
			}
			pub <- o
		}()

		res := sub.Wait(time.Millisecond)
		o := <-pub

		if !o.falseThenOK {
			t.Fatalf("iteration %d: Publish returned true after a false", i)
		}
		v, present := res["a"]
		if o.anyTrue != present {
			t.Fatalf("iteration %d: anyTrue=%v but present=%v (res=%v)", i, o.anyTrue, present, res)
		}
		if present && v != o.lastTrue {
			t.Fatalf("iteration %d: result %v != last true publish %d", i, v, o.lastTrue)
		}
		ps.Close()
	}
}

// TestNoGoroutineLeakAfterClose checks that Close wakes every blocked Wait:
// each Wait goroutine must return well before its own 5s timeout.
func TestNoGoroutineLeakAfterClose(t *testing.T) {
	var wg sync.WaitGroup
	for i := range 20 {
		ps := NewPubSub()
		for j := range 10 {
			sub := ps.NewSubscriber()
			sub.Subscribe(fmt.Sprintf("k%d", j))
			wg.Go(func() { sub.Wait(5 * time.Second) })
		}
		if err := ps.Close(); err != nil {
			t.Fatalf("iteration %d: Close returned %v", i, err)
		}
		assertNoFinishedInTopics(t, ps)
	}
	finished := make(chan struct{})
	go func() { wg.Wait(); close(finished) }()
	select {
	case <-finished:
	case <-time.After(generous):
		t.Fatalf("Wait goroutines still blocked %v after Close", generous)
	}
}

// TestStressCloseSubscribePublishWait exercises every operation concurrently
// with a Close fired mid-run. The old implementation deadlocked here.
func TestStressCloseSubscribePublishWait(t *testing.T) {
	const iterations = 100
	for i := range iterations {
		ps := NewPubSub()
		var wg sync.WaitGroup
		for j := range 20 {
			sub := ps.NewSubscriber()
			wg.Add(1)
			go func(sub *Subscriber, seed int64) {
				defer wg.Done()
				r := rand.New(rand.NewSource(seed)) // deterministic pseudo-random schedule, not security
				for range 5 {
					sub.Subscribe(fmt.Sprintf("k%d", r.Intn(8)))
				}
				for k := range 5 {
					ps.Publish(fmt.Sprintf("k%d", r.Intn(8)), k)
				}
				// Sample the invariant here, mid-run: this is the window the
				// check exists for, when finished and live subscribers share
				// the same topics. The post-Close call below sees an empty
				// ps.topics and inspects nothing.
				assertNoFinishedInTopics(t, ps)
				sub.Wait(time.Duration(r.Intn(3)) * time.Millisecond)
			}(sub, int64(i*100+j))
		}
		closed := make(chan struct{})
		go func() {
			if err := ps.Close(); err != nil {
				t.Errorf("iteration %d: Close returned %v", i, err)
			}
			close(closed)
		}()

		finished := make(chan struct{})
		go func() { wg.Wait(); close(finished) }()
		select {
		case <-finished:
		case <-time.After(5 * time.Second):
			t.Fatalf("iteration %d: deadlock (workers did not finish)", i)
		}
		select {
		case <-closed:
		case <-time.After(5 * time.Second):
			t.Fatalf("iteration %d: deadlock (Close did not return)", i)
		}
		assertNoFinishedInTopics(t, ps)
		if ps.GetTopicCount() != 0 {
			t.Fatalf("iteration %d: %d topics left after Close", i, ps.GetTopicCount())
		}
	}
}

// --- helpers for the bubbled (fake-clock) tests ----------------------------

// bubbledWait is the synctest counterpart of waitInGoroutine: it starts
// sub.Wait(timeout) in a bubbled goroutine and returns a probe that quiesces
// the bubble and reports whether that Wait has returned. waitInGoroutine
// cannot be used inside a bubble because its poll loop sleeps, and every sleep
// would advance the fake clock into the pending timer.
func bubbledWait(sub *Subscriber, timeout time.Duration) func() (map[string]any, bool) {
	out := make(chan map[string]any, 1) // buffered: the waiter never blocks on send
	go func() { out <- sub.Wait(timeout) }()
	return func() (map[string]any, bool) {
		synctest.Wait()
		select {
		case res := <-out:
			return res, true
		default:
			return nil, false
		}
	}
}

// assertNoFinishedInTopics pins the invariant that finishLocked's idempotence
// guard would otherwise hide: a finished subscriber must never stay attached to
// a topic. Without the guard that situation panics on the second close(waitCh);
// with it, Close silently fails to detach and ps.topics never empties.
//
// It only detects anything while live and finished subscribers coexist, so it
// has to be called mid-run: once Close has returned, ps.topics is empty and the
// loop body never executes. The call sites that carry it are the worker in
// TestStressCloseSubscribePublishWait and TestPublishLeavesMultiTopicWaitersAttached;
// the post-Close calls are cheap belt-and-braces, not the detector.
func assertNoFinishedInTopics(t *testing.T, ps *PubSub) {
	t.Helper()
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	for key, topic := range ps.topics {
		for s := range topic.subscribers {
			if s.done {
				t.Errorf("finished subscriber still attached to topic %q", key)
			}
		}
	}
}

// assertFinished pins the rest of the terminal transition: a finished
// subscriber has a closed waitCh and is no longer marked as waiting. The
// waiting half is not observable through the public API, and finishLocked's
// idempotence guard makes a stale waiting flag silent rather than fatal, so it
// needs pinning here.
func assertFinished(t *testing.T, sub *Subscriber) {
	t.Helper()
	ps := sub.pubsub
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	if !sub.done {
		t.Fatal("subscriber is not finished")
	}
	if sub.waiting {
		t.Error("finished subscriber is still marked as waiting")
	}
	select {
	case <-sub.waitCh:
	default:
		t.Error("finished subscriber has an open waitCh")
	}
}

// --- the effective timeout is the earliest deadline, not the shortest ------

// TestStaggeredWaitsUseEarliestDeadline is the counterexample to "the effective
// timeout is the shortest one": a Wait(50ms) started at t=80ms has a later
// deadline (t=130ms) than a Wait(100ms) started at t=0, so the nominally
// longer timeout is the one that governs.
func TestStaggeredWaitsUseEarliestDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ps := NewPubSub()
		sub := ps.NewSubscriber()
		sub.Subscribe("a") // never published

		start := time.Now()
		long := bubbledWait(sub, 100*time.Millisecond) // deadline t=100ms
		if _, done := long(); done {
			t.Fatal("the 100ms Wait returned immediately")
		}

		time.Sleep(80 * time.Millisecond)
		short := bubbledWait(sub, 50*time.Millisecond) // deadline t=130ms
		if _, done := short(); done {
			t.Fatal("the 50ms Wait returned immediately")
		}
		if _, done := long(); done {
			t.Fatal("the 100ms Wait returned before its deadline")
		}

		time.Sleep(20 * time.Millisecond) // t=100ms: the 100ms Wait's timer
		resLong, doneLong := long()
		resShort, doneShort := short()
		if !doneLong || !doneShort {
			t.Fatalf("at t=%v: 100ms done=%v, 50ms done=%v; both must have returned",
				time.Since(start), doneLong, doneShort)
		}
		if elapsed := time.Since(start); elapsed != 100*time.Millisecond {
			t.Fatalf("both Waits returned at %v, want exactly 100ms", elapsed)
		}
		if len(resLong) != 0 || len(resShort) != 0 {
			t.Fatalf("expected empty snapshots, got %v and %v", resLong, resShort)
		}
		assertFinished(t, sub) // finished by its timer
	})
}

// --- Close and the subscriber that never subscribed ------------------------

// TestCloseLeavesZeroSubscriptionSubscriberLive pins the qualifier in the
// contract: Close reaches subscribers through ps.topics, so one that never
// subscribed is not registered anywhere and stays live until its first Wait.
func TestCloseLeavesZeroSubscriptionSubscriberLive(t *testing.T) {
	ps := NewPubSub()
	sub := ps.NewSubscriber()

	if err := ps.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	ps.mu.RLock()
	done := sub.done
	ps.mu.RUnlock()
	if done {
		t.Fatal("Close finished a subscriber that holds no subscription")
	}

	// Run it in a goroutine with a bounded receive: if this ever regresses to
	// blocking, the test fails within `generous` instead of hanging for the
	// whole package timeout.
	out, _ := waitInGoroutine(t, sub, time.Hour) // must not block, and must finish it now
	res := recvWithin(t, out)
	if len(res) != 0 {
		t.Fatalf("expected empty results, got %v", res)
	}
	ps.mu.RLock()
	done = sub.done
	ps.mu.RUnlock()
	if !done {
		t.Fatal("Wait after Close did not finish the subscriber")
	}
	assertFinished(t, sub) // finished on entry to Wait
}

// --- Wait returns a shallow copy -------------------------------------------

// TestWaitCopyIsShallow pins that payloads are shared by reference: only the
// results map itself is copied.
func TestWaitCopyIsShallow(t *testing.T) {
	ps := NewPubSub()
	defer ps.Close()

	payload := map[string]int{"n": 1}
	subA, subB := ps.NewSubscriber(), ps.NewSubscriber()
	subA.Subscribe("a")
	subB.Subscribe("a")
	if !ps.Publish("a", payload) {
		t.Fatal("Publish must return true")
	}

	resA := subA.Wait(0)
	got, ok := resA["a"].(map[string]int)
	if !ok {
		t.Fatalf("unexpected payload type %T", resA["a"])
	}
	got["n"] = 2

	resB := subB.Wait(0)
	shared, ok := resB["a"].(map[string]int)
	if !ok {
		t.Fatalf("unexpected payload type %T", resB["a"])
	}
	if shared["n"] != 2 {
		t.Fatalf("payload is not shared: second subscriber sees %d, want 2", shared["n"])
	}
	if payload["n"] != 2 {
		t.Fatalf("payload is not shared with the publisher: %d, want 2", payload["n"])
	}
}

// --- a nil payload is a value, not an absence ------------------------------

// TestNilPayloadIsPresent pins that only the comma-ok form distinguishes
// "received nil" from "received nothing".
func TestNilPayloadIsPresent(t *testing.T) {
	ps := NewPubSub()
	defer ps.Close()

	sub := ps.NewSubscriber()
	sub.Subscribe("a")
	sub.Subscribe("b") // never published
	if !ps.Publish("a", nil) {
		t.Fatal("Publish of a nil payload must return true")
	}

	res := sub.Wait(0)
	if _, ok := res["a"]; !ok {
		t.Fatalf("key a must be present with a nil value, got %v", res)
	}
	if _, ok := res["b"]; ok {
		t.Fatalf("key b never received anything and must be absent, got %v", res)
	}
	if len(res) != 1 {
		t.Fatalf("expected exactly one key, got %v", res)
	}
}

// --- the nil subscriber actually returned by NewSubscriber -----------------

func TestNilSubscriberFromClosedHub(t *testing.T) {
	ps := NewPubSub()
	if err := ps.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	sub := ps.NewSubscriber()
	if sub != nil {
		t.Fatalf("NewSubscriber on a closed hub returned %v, want nil", sub)
	}

	sub.Subscribe("a") // no-op on the typed nil
	for _, d := range []time.Duration{-1, 0, time.Hour} {
		res := sub.Wait(d)
		if res == nil || len(res) != 0 {
			t.Fatalf("Wait(%v) = %v, want a non-nil empty map", d, res)
		}
		res["x"] = 1 // every call must hand back a fresh map
	}
	if ps.GetTopicCount() != 0 || ps.GetSubscriberCount("a") != 0 {
		t.Fatalf("state leaked: %d topics, %d subscribers",
			ps.GetTopicCount(), ps.GetSubscriberCount("a"))
	}
}

// --- Publish deleting the topic while ranging over its subscribers ---------

// TestPublishCompletesEveryWaiterOnOneKey drives the case the comment in
// Publish defends: finishLocked deletes the current entry of the map being
// ranged and, on the last subscriber, drops the key from ps.topics.
func TestPublishCompletesEveryWaiterOnOneKey(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const n = 16
		ps := NewPubSub()
		subs := make([]*Subscriber, n)
		probes := make([]func() (map[string]any, bool), n)
		for i := range n {
			sub := ps.NewSubscriber()
			sub.Subscribe("k")
			subs[i] = sub
			probes[i] = bubbledWait(sub, time.Hour)
		}
		probes[0]()
		if got := ps.GetSubscriberCount("k"); got != n {
			t.Fatalf("GetSubscriberCount(k) = %d, want %d", got, n)
		}

		if !ps.Publish("k", 1) {
			t.Fatal("Publish must return true")
		}
		for i := range n {
			res, done := probes[i]()
			if !done || len(res) != 1 || res["k"] != 1 {
				t.Fatalf("subscriber %d: done=%v res=%v", i, done, res)
			}
			assertFinished(t, subs[i]) // finished by the Publish that completed it
		}
		assertNoFinishedInTopics(t, ps)
		if ps.GetTopicCount() != 0 {
			t.Fatalf("expected 0 topics, got %d", ps.GetTopicCount())
		}
		if ps.Publish("k", 2) {
			t.Fatal("Publish after every subscriber finished must return false")
		}
	})
}

// TestPublishLeavesMultiTopicWaitersAttached is the mixed state the
// idempotence guard could hide: half the waiters finish on this Publish while
// the other half stay live in the same topic.
func TestPublishLeavesMultiTopicWaitersAttached(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const n = 8
		ps := NewPubSub()
		subs := make([]*Subscriber, n)
		probes := make([]func() (map[string]any, bool), n)
		for i := range n {
			sub := ps.NewSubscriber()
			sub.Subscribe("k")
			if i%2 == 1 {
				sub.Subscribe("extra") // Publish("k") cannot complete these
			}
			subs[i] = sub
			probes[i] = bubbledWait(sub, time.Hour)
		}
		probes[0]()

		if !ps.Publish("k", 1) {
			t.Fatal("Publish must return true")
		}
		assertNoFinishedInTopics(t, ps)
		for i := range n {
			_, done := probes[i]()
			if want := i%2 == 0; done != want {
				t.Fatalf("subscriber %d: returned=%v, want %v", i, done, want)
			}
		}
		if got := ps.GetSubscriberCount("k"); got != n/2 {
			t.Fatalf("GetSubscriberCount(k) = %d, want %d", got, n/2)
		}

		if err := ps.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
		for i := 1; i < n; i += 2 {
			res, done := probes[i]()
			if !done || len(res) != 1 || res["k"] != 1 {
				t.Fatalf("subscriber %d: done=%v res=%v", i, done, res)
			}
			assertFinished(t, subs[i]) // finished by Close
		}
		assertNoFinishedInTopics(t, ps)
		if ps.GetTopicCount() != 0 {
			t.Fatalf("expected 0 topics after Close, got %d", ps.GetTopicCount())
		}
	})
}
