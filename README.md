# Canary

**Vault-informed CI test selection.** Given a change, Canary emits the exact set of tests worth running — the impacted subset instead of the whole E2E suite — so the inner loop stays fast and full verification is reserved for the merge and release boundaries. The coverage map says which tests exercise which code; the Tolvi vault says which tests can never be skipped.

> **Status: v1 shipped.** Canary is a working CLI: `canary init [--audit]`, `canary audit`, `canary check --base <ref> --head <ref> [--gate pr|merge|release]`, `canary refresh`, and `canary hook install|uninstall`. The design is in [`vault/decisions/`](vault/decisions/) and [`docs/PLAN.md`](docs/PLAN.md).

It is the test-gate of the Tolvi stack:

```
Guild (plan) → Bastion (harden plan) → code → Provenance (guard the record) → Canary (guard the tests) → prod
```

## What it does

Canary maps changed code to the tests that prove it (via a coverage-instrumented test↔code map), applies the vault's "never auto-skip these" bindings, and gates the pipeline: the impacted subset on a PR, the full suite at merge to main and at release. It consumes [Provenance](https://github.com/tolvi-labs/provenance)'s handoff — the declared provenance and vault governance for a change — to rank risk and enforce the never-skip overrides, and it produces the human-readable impact report that surfaces on the PR.

`canary init` builds the global coverage manifest from scratch (optionally running `--audit` right after); `canary hook install` wires up a `.git/hooks/post-commit` shim that calls `canary refresh` to keep that manifest incrementally up to date after every commit; `canary check` is the CI entry point that computes the gated test selection for a PR, merge, or release. See [`docs/vault-bindings.md`](docs/vault-bindings.md) for the `x-canary-bindings` schema that backs the never-skip overrides, and [`docs/ci-integration.md`](docs/ci-integration.md) for wiring it into CI.

The full run at the merge boundary is deliberate: "every PR passed → the merge is safe" is a false guarantee, because PRs interact and a per-PR subset was computed against a base that has since moved. Canary enforces that best practice and makes the engineer conscious of it; it never silently automates the risky skip.

## Design principles

- **The moat is the vault, not the coverage map.** Coverage-based selection alone is a commodity. The vault bindings — "cache layer touched → always run these integration tests, per the stale-cache incident" — are what make it a Tolvi tool.
- **One thing can drift, so all freshness points at it.** The `globalManifest` (the full registry + coverage map) is the only durable, drift-prone artifact; the per-PR `localManifest` is derived fresh every time and never accumulates.
- **Canary maps and gates tests; it never authors them.** Its audit output — orphan tests and impacted-but-untested paths — is [Forge](https://github.com/tolvi-labs/forge)'s to-do list.

The full rationale is in [`docs/PLAN.md`](docs/PLAN.md) and the decisions behind it live in [`vault/decisions/`](vault/decisions/).

## License

Apache 2.0, in line with the rest of the Tolvi suite.
