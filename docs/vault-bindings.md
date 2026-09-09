# `x-canary-bindings` — vault-declared never-skip tests

This is the moat. Coverage-based test selection alone is a commodity: any tool can trace which tests exercise which lines. What makes Canary a Tolvi tool is that the vault can declare, in plain YAML, "if this path is touched, always run these tests" — independent of whatever the coverage map says — and Canary enforces it. A typical use is pinning down a test that guards against a specific incident: coverage might technically link the touched file to a dozen tests, but the vault knows which one actually caught the regression last time and refuses to let it be skipped.

## Where it lives

`x-canary-bindings` is a new field on the YAML frontmatter of a `vault/decisions/*.md` file, alongside the existing fields such as `status`, `tags`, and `date`:

```yaml
---
tags: [decision, cache]
date: 2026-08-01
repo: my-service
status: active
x-canary-bindings:
  - paths: ["internal/cache/**"]
    tests: ["TestCacheInvalidation_StaleWrite", "TestCacheInvalidation_ConcurrentRead"]
---
```

Each entry in the list is one binding: `paths` is a set of [doublestar](https://github.com/bmatcuk/doublestar) glob patterns (so `**` matches across directory boundaries), and `tests` is the set of test names to force-include whenever a changed file matches any of those globs. A decision can declare more than one binding, and each binding can name any number of paths and tests.

## Only `status: active` decisions are evaluated

Canary only evaluates `x-canary-bindings` on decisions whose frontmatter `status` is `active`. A `superseded` or otherwise inactive decision's bindings are ignored entirely, even if the file still contains them — supersede the decision instead of trying to remove the binding by hand.

## A binding on an unknown test fails open

If a binding names a test that doesn't exist in the manifest's test registry (typos happen, and tests get renamed or deleted), Canary does not hard-fail the check. It reports the problem as a warning in `canary check`'s human-readable output, under "Problems (fail-open, not blocking)", and continues gating normally. A bad binding should never be able to block a PR; it should just get noticed and fixed.

## What a matched binding does

When a changed file matches a binding's `paths`, every test in that binding's `tests` is added to the PR gate's selection with `reason: never-skip-binding` (and the JSON/human report also records which decision forced it in). This happens regardless of what the coverage-derived selection already contains — a never-skip binding is a floor, not a suggestion, and it stacks with (rather than replaces) whatever coverage-based selection would have chosen anyway.

This only applies to the `pr` gate. The `merge` and `release` gates already run the full suite unconditionally, so a never-skip binding has nothing to add there.
