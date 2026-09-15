package pubsub

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"
)

// These tests pin the deterministic contract documented in the package
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
		if ps.GetTopicCount() != 0 {
			t.Fatalf("iteration %d: %d topics left after Close", i, ps.GetTopicCount())
		}
	}
}
