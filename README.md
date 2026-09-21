# pubsub

[![Go Reference](https://pkg.go.dev/badge/github.com/maslennikov-yv/pubsub.svg)](https://pkg.go.dev/github.com/maslennikov-yv/pubsub)
[![CI](https://github.com/maslennikov-yv/pubsub/actions/workflows/ci.yml/badge.svg)](https://github.com/maslennikov-yv/pubsub/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/maslennikov-yv/pubsub)](https://goreportcard.com/report/github.com/maslennikov-yv/pubsub)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

**Publish-subscribe with timeout, suitable for polling a group of sensors.**

A small, thread-safe publish-subscribe hub in Go whose subscribers wait for a **set of keyed events** with a timeout. It is built for scenarios like polling a group of sensors: subscribe to one key per sensor, publish readings as they arrive, and `Wait` returns as soon as every key has a value or the timeout elapses, whichever comes first.

The hub is linearizable: every operation applies its effect inside a single hub-wide critical section, so the outcome of a scenario depends on the order in which calls acquire that section, not on how their goroutines interleave inside it. Two qualifications: `Wait` releases the lock while it is parked, and a blocked `Wait` is woken by a wall-clock timer whose expiry is not ordered against concurrent calls (the linearization rule for `Wait` keeps the results consistent either way). There are no background goroutines, sleeps, logs, or panics.

[Русская версия](docs/README_RU.md) (English is canonical.)

## Install

```sh
go get github.com/maslennikov-yv/pubsub@latest
```

Requires Go 1.26 or newer. No dependencies outside the standard library.

## Key Features

- **Simple API.** Create a hub and subscribers, subscribe by string key, wait with a timeout, publish with a boolean result.
- **Early return or partial result.** `Wait` returns immediately once every subscribed key has a value; on timeout it returns whatever arrived in time.
- **Latest value wins.** If a key is published several times while the subscriber is live, only the most recent payload is kept.
- **Exact publisher feedback.** `Publish` returns `true` if and only if at least one live subscriber stored the payload. A `true` result guarantees the payload will appear in that subscriber's `Wait` result (unless a later `true` publish overwrote it); a `false` result guarantees nobody saw it.
- **Synchronous, idempotent `Close`.** Wakes every blocked `Wait` immediately, drops all topics, always returns `nil`.

## Usage Example

```go
hub := pubsub.NewPubSub()
defer hub.Close()

subscriber := hub.NewSubscriber()
subscriber.Subscribe("foo")
subscriber.Subscribe("buz")

// Publishers usually run in other goroutines while Wait blocks.
go func() {
    hub.Publish("foo", map[string]int{"foo": 90})  // true
    hub.Publish("foo", map[string]int{"foo": 100}) // true, overwrites 90
    hub.Publish("bar", map[string]int{"bar": 50})  // false: nobody listens to "bar"
}()

// Wait up to 1 second. "buz" never arrives, so Wait returns on timeout.
results := subscriber.Wait(time.Second)

// results == map[string]any{"foo": map[string]int{"foo": 100}}
// "buz" is absent: nothing was published for it.
// "foo" holds the latest value (100, not 90).
```

A runnable version lives in [`examples/sensors/main.go`](examples/sensors/main.go); a deterministic variant is the godoc `Example` in [`example_test.go`](example_test.go).

## Semantics

### Subscriber lifecycle

A subscriber is **live** from `NewSubscriber` until it **finishes**. Finishing is a single, atomic, terminal transition: the subscriber stops accepting messages and is removed from every topic (topics left empty are deleted). It happens at exactly one of these moments:

1. **Entry to `Wait`**, if the hub is closed, the timeout is `<= 0`, or the subscriber is already *complete* (every subscribed key has a value; a subscriber with no subscriptions is trivially complete and returns at once).
2. **A `Publish` that makes the subscriber complete while a `Wait` is blocked.**
3. **Expiry of a blocked `Wait`'s timer.**
4. **`Close`.**

Becoming complete *before* `Wait` is called is **not** a trigger: the subscriber stays live, keeps accepting overwrites, and accepts further `Subscribe` calls. So `Subscribe(a); Publish(a); Subscribe(b)` works as expected: `Wait` will require both `a` and `b`.

A subscriber is **single-use**. After it finishes, `Subscribe` is a no-op and every further `Wait` returns the same snapshot immediately without blocking. Concurrent `Wait` calls on one subscriber all return that snapshot as soon as the first of them finishes it, so what governs is the **earliest deadline** among the `Wait` calls started while the subscriber was live -- a deadline being the moment a `Wait` was entered plus its timeout, or the moment of the call itself when the timeout is `<= 0`. That is *not* the shortest timeout: a `Wait(50ms)` started after a `Wait(100ms)` has the later deadline and is cut off by it. A `Wait` started after the subscriber finished changes nothing.

### `Subscribe(key)`

Registers interest in `key`. It is a no-op on a `nil` subscriber, a closed hub, a finished subscriber, or a key that is already subscribed. It takes effect even while a `Wait` on this subscriber is blocked, extending the set of keys `Wait` requires.

### `Publish(key, payload) bool`

Delivers `payload` to every live subscriber of `key`, replacing any earlier payload for that key. Returns `true` if at least one subscriber received it, `false` if the hub is closed or nobody is subscribed. Cost is `O(subscribers of the key)`, plus `O(total subscriptions)` for any subscriber the call completes, all under the write lock.

Consequences:

- **Latest wins while live.** Overwrites are accepted until the subscriber finishes.
- **Once `Wait` is in progress, it returns at the first moment of completeness**, and from then on the subscriber accepts nothing (`Publish` returns `false` if it was the only subscriber).
- A subscriber that has collected everything but **never calls `Wait` stays registered** until `Close`; publishes to it keep returning `true`. Cleanup happens only on `Wait` or `Close`.

### `Wait(timeout) map[string]any`

Blocks until every subscribed key has a value, `timeout` elapses, or the hub closes; then finishes the subscriber and returns a copy of the results. The copy is **shallow**: payloads come back exactly as they were published, so every subscriber of a key -- and the publisher -- hold the same value. Keys that received nothing are absent, but a key published with a `nil` payload is *present* with a `nil` value, so only the comma-ok form (`v, ok := res[k]`) distinguishes the two. `timeout <= 0` returns immediately with whatever has arrived -- and finishes the subscriber. `Wait` on a `nil` subscriber returns an empty map.

The linearization point of `Wait` is its finalization. A payload published in the tiny window between the timer expiring and finalization is included in the result, and that `Publish` returned `true`, so results and return values are always mutually consistent.

### `Close() error`

Marks the hub closed, finishes every **registered** subscriber -- one holding at least one subscription -- waking blocked `Wait` calls immediately with whatever they collected so far, and drops all topics. A subscriber that never called `Subscribe` is in no topic, so `Close` does not reach it: it stays live and finishes on its first `Wait`, which cannot block. Synchronous, idempotent, `O(total subscriptions)`, always returns `nil`. Afterwards `NewSubscriber` returns `nil`, `Publish` returns `false`, `Subscribe` is a no-op, and `Wait` never blocks.

## API

| Method | Purpose |
|---|---|
| `NewPubSub() *PubSub` | Create an open hub. |
| `(*PubSub).NewSubscriber() *Subscriber` | Create a live subscriber; `nil` after `Close`. |
| `(*Subscriber).Subscribe(key string)` | Register interest in a key. |
| `(*Subscriber).Wait(timeout time.Duration) map[string]any` | Collect results. |
| `(*PubSub).Publish(key string, payload any) bool` | Deliver a payload. |
| `(*PubSub).Close() error` | Shut down; always `nil`. |
| `(*PubSub).IsClosed() bool` | Whether `Close` was called. |
| `(*PubSub).GetTopicCount() int` | Keys with at least one live subscriber (advisory: stale on return). |
| `(*PubSub).GetSubscriberCount(key string) int` | Live subscribers of a key (advisory: stale on return). |
| `(*PubSub).Hash(key string) string` | Hex MD5 of a key, for fixed-length topic names; ignores the receiver, not a security primitive. |

## Guarantees

- **Thread safety.** All methods may be called concurrently from any goroutine. A single hub-wide `RWMutex` guards all state; there is no lock hierarchy to invert. The flip side is that every call, including `Publish` on unrelated keys, is serialized per hub; if you need parallel fan-out, shard across several `PubSub` instances.
- **No hidden concurrency.** The library starts no goroutines and never sleeps. `Close` and `Wait` finalization are synchronous.
- **No panics, no logs.** For values obtained from `NewPubSub` and `NewSubscriber` (including the `nil` subscriber returned after `Close`), invalid operations are safe no-ops with defined return values. Zero-value structs and a `nil *PubSub` are not supported -- `Hash` happens not to dereference its receiver, but that is an implementation detail, not a guarantee.
- **Memory.** A subscriber is detached from all topics when it finishes; empty topics are deleted. A registered subscriber that never calls `Wait` is held by the hub until `Close`. On a long-lived hub that is a leak, so a caller that may return before its `Wait` should `defer sub.Wait(0)` -- note that this discards the returned map and finishes the subscriber, so do not use it when another goroutine is still collecting into it.

## Use Cases

1. **Sensor data collection.** Poll several sensors and collect their responses within a time window.
2. **Fan-in with timeout.** Wait for replies from multiple services, accepting a partial set.
3. **Event aggregation.** Gather the latest state from several sources before processing.

## Versioning

Semantic Versioning. While the major version is 0, a **minor** release may
contain breaking changes (listed in [CHANGELOG.md](CHANGELOG.md)); a **patch**
release never does. v0.2.0 removed `PubSubError`, the cleanup
constants and the exported `Topic` type, and changed the behaviour of a
subscriber that completes before `Wait`; see the changelog for details.

## Development

```sh
make check     # gofmt, go vet, go mod tidy -diff, golangci-lint, go test -race, benchmark smoke run
make test      # tests with the race detector and coverage
make bench     # benchmark smoke run
make example   # run examples/sensors
```

See [CONTRIBUTING.md](CONTRIBUTING.md) and [AGENTS.md](AGENTS.md).

## License

[MIT](LICENSE)
