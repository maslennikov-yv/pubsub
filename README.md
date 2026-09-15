# pubsub

[![Go Reference](https://pkg.go.dev/badge/github.com/maslennikov-yv/pubsub.svg)](https://pkg.go.dev/github.com/maslennikov-yv/pubsub)
[![CI](https://github.com/maslennikov-yv/pubsub/actions/workflows/ci.yml/badge.svg)](https://github.com/maslennikov-yv/pubsub/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/maslennikov-yv/pubsub)](https://goreportcard.com/report/github.com/maslennikov-yv/pubsub)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

**Publish-subscribe with timeout, suitable for polling a group of sensors.**

A small, thread-safe publish-subscribe hub in Go whose subscribers wait for a **set of keyed events** with a timeout. It is built for scenarios like polling a group of sensors: subscribe to one key per sensor, publish readings as they arrive, and `Wait` returns as soon as every key has a value or the timeout elapses, whichever comes first.

The implementation is deterministic: every operation runs inside a single hub-wide critical section, so the outcome of any scenario depends only on the order in which calls happen, never on goroutine scheduling. There are no background goroutines, sleeps, logs, or panics.

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

A subscriber is **single-use**. After it finishes, `Subscribe` is a no-op and every further `Wait` returns the same snapshot immediately without blocking. Concurrent `Wait` calls on one subscriber all return that snapshot as soon as the first of them finishes it, so the effective timeout is the shortest one.

### `Subscribe(key)`

Registers interest in `key`. It is a no-op on a `nil` subscriber, a closed hub, a finished subscriber, or a key that is already subscribed. It takes effect even while a `Wait` on this subscriber is blocked, extending the set of keys `Wait` requires.

### `Publish(key, payload) bool`

Delivers `payload` to every live subscriber of `key`, replacing any earlier payload for that key. Returns `true` if at least one subscriber received it, `false` if the hub is closed or nobody is subscribed.

Consequences:

- **Latest wins while live.** Overwrites are accepted until the subscriber finishes.
- **Once `Wait` is in progress, it returns at the first moment of completeness**, and from then on the subscriber accepts nothing (`Publish` returns `false` if it was the only subscriber).
- A subscriber that has collected everything but **never calls `Wait` stays registered** until `Close`; publishes to it keep returning `true`. Cleanup happens only on `Wait` or `Close`.

### `Wait(timeout) map[string]any`

Blocks until every subscribed key has a value, `timeout` elapses, or the hub closes; then finishes the subscriber and returns a **copy** of the results. Keys that received nothing are absent. `timeout <= 0` returns immediately with whatever has arrived. `Wait` on a `nil` subscriber returns an empty map.

The linearization point of `Wait` is its finalization. A payload published in the tiny window between the timer expiring and finalization is included in the result, and that `Publish` returned `true`, so results and return values are always mutually consistent.

### `Close() error`

Marks the hub closed, finishes every registered subscriber (waking blocked `Wait` calls immediately with whatever they collected so far), and drops all topics. Synchronous, idempotent, O(subscribers), always returns `nil`. Afterwards `NewSubscriber` returns `nil`, `Publish` returns `false`, `Subscribe` is a no-op, and `Wait` never blocks.

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
| `(*PubSub).GetTopicCount() int` | Keys with at least one live subscriber. |
| `(*PubSub).GetSubscriberCount(key string) int` | Live subscribers of a key. |
| `(*PubSub).Hash(key string) string` | Hex MD5 of a key, for fixed-length topic names (not a security primitive). |

## Guarantees

- **Thread safety.** All methods may be called concurrently from any goroutine. A single hub-wide `RWMutex` guards all state; there is no lock hierarchy to invert. The flip side is that every call, including `Publish` on unrelated keys, is serialized per hub; if you need parallel fan-out, shard across several `PubSub` instances.
- **No hidden concurrency.** The library starts no goroutines and never sleeps. `Close` and `Wait` finalization are synchronous.
- **No panics, no logs.** For values obtained from `NewPubSub` and `NewSubscriber` (including the `nil` subscriber returned after `Close`), invalid operations are safe no-ops with defined return values. Zero-value structs and a `nil *PubSub` are not supported.
- **Memory.** A subscriber is detached from all topics when it finishes; empty topics are deleted. A subscriber that never calls `Wait` is released by `Close`.

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
