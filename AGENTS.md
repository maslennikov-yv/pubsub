# AGENTS.md

Guidance for coding agents (and humans) working in this repository.

## What this is

`github.com/maslennikov-yv/pubsub` is a single-package Go library: a
linearizable keyed publish/subscribe hub where a subscriber waits for a set of
keys with a timeout. Every operation applies its effect inside one hub-wide
critical section; `Wait` is the exception that releases the lock while parked,
and its wall-clock timer is not ordered against concurrent calls. No
dependencies outside the standard library. Go 1.26+.

## Commands

```sh
make check        # the CI gates: fmt-check, vet, tidy-check, lint, test, bench (govulncheck: make vuln)
go test -race ./...
make lint         # golangci-lint v2, pinned, via go run
make example      # go run ./examples/sensors
make example-check # the example's output must match examples/sensors/testdata/golden.txt
```

## Files

| Path | Role |
|---|---|
| `doc.go` | Package documentation. **The behavioural contract here is normative.** |
| `pubsub.go` | The whole implementation (~250 lines). |
| `contract_test.go` | Tests that pin the contract, including race-detector stress tests. |
| `pubsub_test.go`, `readme_test.go` | Older white-box tests, since de-flaked onto `testing/synctest`. Keep passing; change them only to remove timing fragility, never to weaken an assertion. |
| `example_test.go` | Godoc examples, verified by `// Output:`. |
| `examples/sensors/` | Runnable demo program; its stdout is pinned by `testdata/golden.txt` (`make example-check`). |
| `README.md`, `docs/README_RU.md` | User docs. English is canonical; keep the Russian copy in sync. |
| `CHANGELOG.md` | Keep a Changelog format; every user-visible change goes under `[Unreleased]`. |

## Invariants — do not break

- **One lock.** `PubSub.mu` (an `RWMutex`) guards all hub and subscriber
  state. There is no second mutex and no lock ordering to reason about. Never
  call a public method (which takes the lock) from inside a `*Locked` helper.
- **No hidden concurrency.** The library starts no goroutines, never sleeps,
  never logs, never panics for values obtained from `NewPubSub`/`NewSubscriber`.
- **Single terminal transition.** `finishLocked` is the only place that sets
  `done`, closes `waitCh`, and detaches a subscriber from topics. `waitCh` is
  created once and never replaced. `finishLocked` is idempotent, so callers do
  not check `done` first - but that guard *masks* a broken invariant instead of
  panicking on it, which is why `assertNoFinishedInTopics` in
  `contract_test.go` must stay called **while live and finished subscribers
  still share topics** (mid-run in `TestStressCloseSubscribePublishWait`, and in
  `TestPublishLeavesMultiTopicWaitersAttached`). After `Close` returns,
  `ps.topics` is empty and the check inspects nothing.
- **Offers only rise while live.** `readyAll`/`readyAny` are set by a parked
  `Wait` (at park, or at the end of its `AtLeast` hold) and cleared only by
  `finishLocked`: any parked `Wait` leaving its loop finishes the subscriber,
  so no offer outlives its call. `Publish` finishes on `readyLocked()`.
- **`results ⊆ subscriptions`**, so completeness is `len(results) == len(subscriptions)`.
- **`Publish` runs under the write lock** and its boolean is exact: `true` iff
  at least one live subscriber stored the payload.
- **Wait's linearization point is its finalization** (documented; do not
  "fix" the late-publish inclusion).

## Rules

- Behaviour changes require: `doc.go` + both READMEs + a `contract_test.go`
  case + a `CHANGELOG.md` entry.
- Exported API changes additionally require a minor version bump (v0.x).
- The white-box tests (`pubsub_test.go`, `readme_test.go`) access unexported
  fields `topics`, `pubsub`, `subscriptions`, `results`, `waitCh`, and
  `contract_test.go` additionally reads `mu`, `waiting`, `readyAll`, `readyAny`, `done`, `closed` and
  `topic.subscribers`; keep those names.
- Timing-sensitive tests use `testing/synctest`. Inside a bubble, waiting on a
  mutex is **not** durably blocking: a goroutine queued on `ps.mu` freezes the
  fake clock without synctest reporting a deadlock. Use `bubbledWait`, not
  `waitInGoroutine`, in bubbled tests.
- Do not add dependencies. Do not add logging or configuration knobs.
- Run `make check` before finishing; it must be clean.
