# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses
[Semantic Versioning](https://semver.org/). While the major version is 0, a
minor release may contain breaking changes; a patch release never does.

## [Unreleased]

## [0.2.0] - 2026-09-15

### Changed
- **Deterministic core rewrite.** Every operation now runs inside a single
  hub-wide critical section; outcomes depend only on the order of calls, never
  on goroutine scheduling. The full contract lives in the package
  documentation (`doc.go`).
- `Close` is synchronous, idempotent and always returns `nil`. It wakes every
  blocked `Wait` immediately; no goroutines, sleeps or timeouts are involved.
- A subscriber that has collected every subscribed key **before** calling
  `Wait` stays live: further `Publish` calls overwrite (and return `true`) and
  further `Subscribe` calls take effect. Previously it froze and silently
  dropped both.
- `Wait` on a subscriber with no subscriptions returns immediately.
- `Publish` return value is exact: `true` guarantees the payload lands in that
  subscriber's `Wait` result unless overwritten later; `false` guarantees
  nobody observed it.
- Minimum Go version is now 1.26 (`go.mod`).
- `interface{}` spelled as `any` throughout (identical type, no API change).

### Removed
- Exported `PubSubError` and the constants `CleanupTimeout`,
  `CleanupGracePeriod`, `ImmediateCleanupDelay` (they described removed
  cleanup machinery).
- Exported `Topic` type (it had no exported fields or methods and was never
  returned by any function).
- All logging via the standard `log` package.
- `Dockerfile` and the cross-compilation targets of the Makefile (a library
  ships no binaries).

### Fixed
- Deadlock between `Close` and a concurrent `Subscribe` (lock-order inversion).
- `Close` occasionally returned a spurious "subscriber cleanup timeout" error
  depending on scheduler timing.
- `Subscribe` could register a topic after `Close` had cleared them.

### Added
- `contract_test.go` pinning the deterministic contract, including race-detector
  stress tests; godoc examples for `Wait`, `Close` and `Hash`.
- MIT `LICENSE` (restored; it had been dropped in an earlier commit).
- CI: lint (golangci-lint v2), vulnerability check, tidy check, release
  workflow, Dependabot; `AGENTS.md`/`CLAUDE.md` for coding agents;
  `CONTRIBUTING.md`, `SECURITY.md`, `.editorconfig`.

## [0.1.1] - 2025-10-05

- Documentation updates.

## [0.1.0] - 2025-05-24

- Initial release.

[Unreleased]: https://github.com/maslennikov-yv/pubsub/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/maslennikov-yv/pubsub/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/maslennikov-yv/pubsub/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/maslennikov-yv/pubsub/releases/tag/v0.1.0
