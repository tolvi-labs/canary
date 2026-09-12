---
tags: [decision, canary]
date: 2026-09-11
repo: canary
status: active
supersedes: 2026-09-09-canary-ships-as-go-cli
ticket: none
user_impact: high
product_area: Product scope
---

# Canary adds Python and Node coverage backends via a pluggable Backend interface, introducing canary.yml

**Date:** 2026-09-11
**Repo:** canary

## Why

Dogfooding against the real target repos (`birdie-os-sls`, Python; `birdie-os-extension`, Node) found `canary init` fails immediately outside a Go module — the Go-only v1 scope was deliberate but left the tool unusable on two of the three repos it was built to serve.

## How

- Extracted a `coverage.Backend` interface (`ModulePath`/`ListUnits`/`UnitTests`/`TouchedUnits`/`SourceExtensions`) from the original three free functions; moved the Go engine into `internal/coverage/golang` unchanged.
- Added a Python backend (pytest + coverage.py dynamic contexts, one pytest run per file rather than per-test, to stay fast against thousands of tests) and a Node backend (`node:test` + per-test invocation, since `node:test` has no per-test-context feature analogous to coverage.py's).
- New `canary.yml` config file (auto-detected default on first `canary init`, authoritative after that, with a `--lang` override) — this directly reverses the prior decision's "no config file" stance, which was correct for a Go-only tool and stopped being correct once a repo's language had to be declared.
- Built via full brainstorm → spec → plan → subagent-driven-development, 7 tasks, each with an isolated implementer and a fresh reviewer. Two tasks each caught and fixed a real bug in their first review round: the Python backend originally scoped coverage measurement to the whole repo (`--cov=repoDir`) instead of the unit under test, leaking unrelated files' data into every test's results; the Node backend had an unanchored `--test-name-pattern` (a substring match, not exact, despite its own doc comment claiming otherwise) causing coverage misattribution whenever one test name is a substring of another, plus a swallowed exec error that reported a broken Node toolchain as "0 tests found."
- **A final whole-branch review found the deeper problem:** Canary's core safety guarantee — never silently skip a test that should run — was never actually extended past Go, because `internal/gate` and `internal/manifest` carried Go-specific assumptions (a hardcoded `.go` suffix check, directory-prefix semantics for what a "unit" is) that no single task was ever scoped to touch. Two Critical findings: the unmapped-code fallback in `gate.go` silently selected zero tests for a brand-new untested Python/Node file, with no warning; Python's `ListUnits` silently dropped a file that failed pytest collection, and pytest aborts collection entirely on the first error, truncating the whole unit list with no degradation signal. Eight further Important findings (stale coverage never invalidated on `canary refresh` for file-shaped units, the degraded-package fallback being dead code for the new backends, a Node syntax error fabricating a phantom test name, the spec's mandated project-marker validation being entirely unimplemented, a bad `--lang`/malformed `canary.yml` being unrecoverable without manual file deletion, `describe`/`it` suite lines parsed as individual tests, two missing spec-mandated tests, and the feature shipping fully undocumented). Root cause for both Criticals: the spec itself incorrectly claimed `gate.Check` "was never Go-specific" — a spec defect the implementation faithfully inherited, not a task-implementer error.
- One comprehensive fix wave addressed all 10 findings, each with a real regression test verified to fail against reverted pre-fix code with the exact symptom the review reported.
- **That fix wave's own scoped re-review — the only one the process allows — found it had introduced two new regressions.** Per the subagent-driven-development process, no third fix wave was dispatched; both were surfaced to the owner and merged anyway on explicit direction rather than looped further:
  - `gate.go`'s widened degraded-scope matching (needed to fix the dead-code bug above) reopens a narrow, untested, signal-free version of the original Critical under-selection bug — but now for **Go itself**, the one backend this branch was supposed to leave untouched. Trigger: a degraded Go package directory with coverage history containing a sub-package that receives a brand-new untested file.
  - The new `TestsByUnit` manifest field (added to fix stale-coverage invalidation for file-shaped Python/Node units) can, if only partially populated, silently retire a live test that a not-yet-rebuilt unit still owns. Reachable through the realistic upgrade path: an existing pre-branch manifest, `canary.yml` newly committed, then `canary refresh` firing via the installed post-commit hook and touching only some units.
- Full session narrative, every finding, and every ruling made along the way: `marketing-site/vault/sessions/2026-09-11-canary.md`.

## Outcome

Canary now runs `init`/`refresh`/`check` across Go, Python, and Node repos via `canary.yml`, merged to `main` (`d6432a3..b5810c8`) with all tests green — but with two known, documented, unresolved Important regressions left open by explicit owner choice rather than fixed: a narrow Go-specific under-selection edge case (`gate.go`'s degraded-scope widening), and a test-registry data-loss edge case reachable during the `canary refresh` upgrade window (`TestsByUnit` partial population). Fix directions for both are recorded in the session note and are follow-up work, not forgotten debt.

**Supersedes:** [[2026-09-09-canary-ships-as-go-cli]] — only its "v1 is Go-only, no config file" claim; the coverage engine internals, per-test isolated coverage design, two-manifest model, three-tier gate policy, and audit-mode subprocess-boundary limitation it documents are untouched and remain active.

## 2026-09-12 update — both regressions fixed

Both N1 and N2 (above) are resolved, merged to `main` (`f83173f..eed337e`), independently reviewed and confirmed correct.

- **N1 fixed:** added `manifest.FileOwnScope` (a file's own immediate scope only, no ancestor matching) and used it for `gate.go`'s unmapped-code guard specifically — `degradedPackageTests`'s separate, intentionally-broader `FileInScope` matching was left untouched, since widening there only ever adds safety. Regression test: a degraded package with coverage history plus a brand-new untested file in an unrelated sub-package nested in its directory now correctly falls back to the full suite.
- **N2 fixed:** added `testsByUnitComplete`, gating all retirement on every known unit having a `TestsByUnit` entry — an incomplete index is now treated exactly like an absent one (no retirement that round, self-healing as units get rebuilt, restored immediately by a full `canary init`). Regression test: two Node test files sharing a test name, one deliberately missing from the index, refreshing only the other — the shared name now survives.
- **Review found one follow-up gap in N2's own fix**, fixed in the same pass: a unit that has *never once* compiled can never earn a `TestsByUnit` entry, so without an exemption the completeness check would stay false forever for that unit — even across repeated `canary init` rebuilds — silently disabling retirement for the *whole* manifest, not just the broken unit. Fixed by exempting any unit already in `DegradedPackages`, since it's known to own zero test names rather than merely being unaccounted-for. Regression test: a permanently-broken unit alongside two healthy ones confirms retirement still works for the healthy ones regardless of how long the broken one stays broken.
- Every fix (including the follow-up) was verified red-then-green against reverted code before being restored, plus an independent code review. Session narrative: `marketing-site/vault/sessions/2026-09-12-canary.md`.

Canary now has no known open regressions from the multi-language coverage work.
