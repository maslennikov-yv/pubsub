# Contributing

Thanks for taking the time. This is a small library with a precise behavioural
contract, so most of the rules below are about not breaking it by accident.

## Before opening a pull request

```sh
make check   # gofmt, go vet, go mod tidy -diff, golangci-lint, go test -race, benchmark smoke run
```

Go 1.26 or newer is required (see `go.mod`). The linter runs pinned via
`go run`, so nothing needs to be installed globally.

If your local `go` is older than 1.26, the go command downloads a matching
toolchain automatically. Should that auto-switch fail with
`compile: version "go1.26.x" does not match go tool version ...`, pin the
toolchain explicitly: `GOTOOLCHAIN=go1.27.1 make check`.

## Ground rules

- **The contract in `doc.go` is normative.** If a change alters observable
  behaviour, update `doc.go`, `README.md` and `docs/README_RU.md` together and
  add a case to `contract_test.go` that pins the new behaviour.
- **Existing tests stay green.** `pubsub_test.go` and `readme_test.go` are
  legacy white-box tests and are deliberately kept as they are.
- **Design invariants** (see `AGENTS.md`): one hub-wide lock, no goroutines,
  sleeps, logging or panics inside the library, no external dependencies.
- **Exported API changes** need a `CHANGELOG.md` entry under `[Unreleased]`
  and, while the major version is 0, a minor version bump on release.
- Keep commits focused; describe *why* in the message.

## Releasing

1. Move `[Unreleased]` in `CHANGELOG.md` to a dated version section and update
   the comparison links at the bottom.
2. Tag: `git tag -a vX.Y.Z -m "vX.Y.Z" && git push origin vX.Y.Z`.
3. The `Release` workflow creates the GitHub release with the changelog
   section as its body.
