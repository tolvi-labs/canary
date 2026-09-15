# AGENTS.md

Guidance for coding agents working in this repo.

## What this is

Canary is vault-informed CI test selection: given a change, it emits the exact set of tests worth running, so the inner loop stays fast and full verification is reserved for the merge and release boundaries.

## Build and test

```bash
go build ./... && go test ./...
```

## Conventions

- **Two inputs, and the second is the point.** A coverage-instrumented map says which tests exercise which code; the vault says which tests can never be skipped. A pure coverage tool cannot have the second, which is why this is a Tolvi tool rather than a test runner.
- **Never-skip bindings live in the vault** as `x-canary-bindings`, using the `x-` extension namespace `tolvi-format-v2` reserves for exactly this.
- **`canary.yml` is authoritative once written.** `init`, `refresh` and `check` all read it and never re-detect, so the gate can never disagree with the manifest about which backend built it.
- **A language the repo shows no sign of is a hard error**, not a silent no-op: an empty manifest green-gates every later check having selected nothing.

## What not to do

- Do not let a stale coverage map pass as a fresh one. A stale map is worse than no map, because it green-gates changes it no longer understands.
