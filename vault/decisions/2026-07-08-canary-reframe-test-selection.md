---
tags: [decision, canary]
date: 2026-07-08
repo: canary
status: active
ticket: none
user_impact: none
product_area: Product scope
supersedes: 2026-07-04-canary-internal-orphan-defer
---

# Canary is reframed from Compliance Canary to vault-informed CI test selection; it now compounds with the vault and consumes Provenance

**Date:** 2026-07-08
**Repo:** canary

## Why

[[2026-07-04-canary-internal-orphan-defer]] benched Compliance Canary as an *ecosystem orphan* — real internal need, but no vault dependency and no compounding with the rest of the stack — and explicitly gated any revival on finding a way for Canary to compound. A Tolvi Labs positioning pass (personal brand vault, 2026-07-08) found that path, but not by rescuing the compliance tool: it **reclaims the name Canary for a different product — vault-informed CI test selection** — that consumes Provenance's impact output and is ranked by the vault. That tool compounds with the core bet by construction, so it clears the exact test the orphan-defer decision set. This banks the reframe and supersedes the defer.

## How

- **Identity — Provenance reasons; Canary gates.** Provenance answers *code → risk* (what a diff impacts, ranked, and why, from the vault; see [[2026-07-08-provenance-impact-blast-radius-hero]]). Canary answers *code → tests → gate* (which concrete tests prove it, and what runs at which pipeline stage). Canary *consumes* Provenance's impacted-code output; it does not redo reachability. Provenance emits an advisory "scoped regression test plan"; Canary turns that reasoning into an **executable CI artifact and a gate**, which Provenance is not. This is the boundary discipline that killed Guild — Canary must own a job no other tool does, and it does: the test↔code map and the gate.
- **vs Forge — Forge drafts tests; Canary maps and gates them.** Canary never authors tests. Its audit output (orphan tests, impacted-but-untested paths) becomes Forge's to-do list.
- **The map (the moat): coverage base + vault bindings.** (1) Coverage-instrument one full run → an honest test↔code graph. (4) Vault-declared bindings add the "never auto-skip these even if the graph says untouched" rules ("cache layer touched → always run these integration tests, per the stale-cache incident"). (4) is what makes it a Tolvi tool instead of a me-too Launchable. Rejected for v1: (3) historical/ML correlation (v2 luxury once CI history exists) and (2) static/convention-only mapping (too coarse to trust a skip on).
- **Two manifests; drift collapses to one job.** `globalManifest` = the full test registry + coverage map — one durable, always-fresh index, refreshed incrementally at commit-time. It is the *only* artifact that can drift, so all freshness engineering points at it (plus a staleness check that warns/fails when the map is older than the code). `localManifest` = the per-PR subset, *derived* from `diff + globalManifest + vault rules`; it cannot drift (recomputed every time) and is one fixed path, not a file that accumulates across merges. This kills both the "too many manifests" and "map bloat" worries — the real failure mode was always a stale global map, never file count.
- **Lifecycle.** `canary init` builds the globalManifest, or audits an existing suite (flags orphan tests + impacted-but-untested paths); "from 0" establishes structure/config/vault-bindings and does NOT generate tests. A commit-time (tolvi-commit) modifier refreshes the globalManifest for touched symbols. CI computes the localManifest on the fly and gates.
- **Gate policy — enforce best practice, don't over-automate.** PR → localManifest (impacted subset, fast, *and the engineer sees it*). Merge to main → globalManifest (full suite). Release/RC tag → globalManifest (formal gate of record). The full run at the merge boundary is deliberate: "every PR passed its localManifest → the merge is safe" is a **false guarantee** — PRs interact (two disjoint diffs each pass their subset, then together break an integration test neither selected) and a localManifest was computed against a base that has since moved. Per-PR passes do not compose into a release guarantee. Canary's job is to make that best practice explicit and the engineer conscious of it, never to silently automate the risky skip.
- **Scope: ship (A), state (B) as north star.** (A) CI test-selection, full stop — v1. (B) "the cheap early-warning guard at every gate" — pre-merge the impacted subset, post-deploy the vault-chosen canary signals to watch before full rollout. (B) retires the suite reframe's "third Canary / what-to-watch-in-prod" placeholder and gives Canary its slot in the legal-system frame: Provenance is the docket of what shipped; Canary is the sentry that keeps watching.
- **Rejected alternatives.** Keep Canary as compliance-only and leave it parked (the orphan problem stands, and the name is better spent here). Build pure coverage-based selection with no vault bindings (a me-too Launchable — commodity, off-thesis). Trust per-PR passes to compose and skip the full suite at merge (a false guarantee that ships composition bugs). Full-suite only at release, not at every main-merge (rejected: main-merge full run catches composition failures the moment they land — chosen over the faster-but-riskier "defer to release" option).

## Outcome

Canary is committed to a reframe from Compliance Canary to **vault-informed CI test selection**: it consumes Provenance's ranked impact, owns the coverage+vault test↔code map and the CI gate, ships (A) test-selection with (B) prod-watch as the stated north star, and supersedes [[2026-07-04-canary-internal-orphan-defer]] — its priority moves from parked-orphan to an active evidence-first probe. The old Compliance Canary concept stays parked under its old scope and would need a new name if ever revived. A plan-first build kickoff prompt is captured at `docs/test-selection-kickoff-prompt.md`.
