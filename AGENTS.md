# AGENTS.md

Guidance for coding agents (and humans) working in this repository.

## What this is

`github.com/maslennikov-yv/pubsub` is a single-package Go library: a
deterministic keyed publish/subscribe hub where a subscriber waits for a set of
keys with a timeout. No dependencies outside the standard library. Go 1.26+.

## Commands

```sh
make check        # the CI gates: fmt-check, vet, tidy-check, lint, test, bench (govulncheck: make vuln)
go test -race ./...
make lint         # golangci-lint v2, pinned, via go run
make example      # go run ./examples/sensors
```

## Files

| Path | Role |
|---|---|
| `doc.go` | Package documentation. **The behavioural contract here is normative.** |
| `pubsub.go` | The whole implementation (~250 lines). |
| `contract_test.go` | Tests that pin the contract, including race-detector stress tests. |
| `pubsub_test.go`, `readme_test.go` | Legacy white-box tests. Keep passing; do not rewrite. |
| `example_test.go` | Godoc examples, verified by `// Output:`. |
| `examples/sensors/` | Runnable demo program. |
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
  created once and never replaced.
- **`results ⊆ subscriptions`**, so completeness is `len(results) == len(subscriptions)`.
- **`Publish` runs under the write lock** and its boolean is exact: `true` iff
  at least one live subscriber stored the payload.
- **Wait's linearization point is its finalization** (documented; do not
  "fix" the late-publish inclusion).

## Rules

- Behaviour changes require: `doc.go` + both READMEs + a `contract_test.go`
  case + a `CHANGELOG.md` entry.
- Exported API changes additionally require a minor version bump (v0.x).
- Legacy tests (`pubsub_test.go`, `readme_test.go`) access unexported fields
  `topics`, `pubsub`, `subscriptions`, `results`, `waitCh`, and
  `contract_test.go` additionally reads `mu` and `waiting`; keep those names.
- Do not add dependencies. Do not add logging or configuration knobs.
- Run `make check` before finishing; it must be clean.
