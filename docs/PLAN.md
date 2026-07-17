# Tolvi Canary — Go-Forward Plan

**Status:** Pre-build. Reframed from Compliance Canary to vault-informed CI test selection; the name is reclaimed. Active evidence-first probe.
**Last updated:** 2026-07-08

## One-line

An OSS CLI that, given a change, emits the exact set of tests worth running — the impacted subset instead of the whole E2E suite — so the inner loop stays fast and full verification is reserved for the merge/release boundary. The coverage map says which tests exercise which code; the Tolvi vault says which tests can never be skipped.

## The reframe (why the original framing changed)

Canary was originally **Compliance Canary** — a HIPAA/SOC 2 config-drift auditor. It was benched on 2026-07-04 as an *ecosystem orphan*: a real internal need, but no vault dependency and no compounding with the rest of the stack, with any revival gated on finding a way to compound. See [[2026-07-04-canary-internal-orphan-defer]] (superseded).

The path forward isn't to rescue the compliance tool — it's to **reclaim the name for a different product**: vault-informed CI test selection. That product consumes Provenance's impact output and is ranked by the vault, so it compounds with the core bet by construction and clears the exact test the orphan-defer decision set. "Canary = the cheap early-warning signal you run before you pay for the expensive thing" finally fits the tool wearing the name. See [[2026-07-08-canary-reframe-test-selection]].

## What it is

- **Provenance reasons; Canary gates.** Provenance answers *code → risk* (what a diff impacts, ranked and explained by the vault). Canary answers *code → tests → gate* (which concrete tests prove it, and what runs at which pipeline stage). Canary consumes Provenance's impacted-code output; it does not redo reachability.
- **The map (the moat):** coverage-instrumented test↔code graph (base) + vault-declared bindings (the "never auto-skip these" overrides). The vault bindings are what make it a Tolvi tool and not a me-too Launchable.
- **Two manifests:** `globalManifest` (the full registry + coverage map — one durable, always-fresh index, refreshed at commit-time; the only thing that can drift) and `localManifest` (the per-PR subset, derived fresh from `diff + globalManifest + vault rules`; cannot drift, never accumulates).
- **Boundary vs Forge:** Forge drafts tests; Canary maps and gates them. Canary's audit output is Forge's to-do list. Canary never authors tests.

## Lifecycle & gate policy

- `canary init` — builds the globalManifest, or audits an existing suite (flags orphan tests + impacted-but-untested paths). "From 0" establishes structure/config/vault-bindings; it does not generate tests.
- Commit-time (tolvi-commit) modifier — incrementally refreshes the globalManifest for touched symbols; plus a staleness check.
- CI — computes the localManifest on the fly and gates:
  - **PR / feature branch → localManifest** (impacted subset; fast, and the engineer sees exactly what runs and why).
  - **Merge to main → globalManifest** (full suite; catches composition failures the moment they land).
  - **Release / RC tag → globalManifest** (formal gate of record).

The full run at merge is deliberate: "every PR passed → the merge is safe" is a false guarantee (PRs interact; a localManifest was computed against a base that has since moved). Canary enforces the existing best practice and makes the engineer conscious of it — it never silently automates the risky skip.

## Scope: ship (A), state (B) as north star

- **(A) CI test-selection, full stop** — v1. Which tests run at which pipeline stage.
- **(B) the cheap early-warning guard at every gate** — pre-merge the impacted subset, post-deploy the vault-chosen canary signals to watch before full rollout. Retires the suite reframe's "third Canary / what-to-watch-in-prod" placeholder; gives Canary its slot in the legal-system frame (Provenance = the docket; Canary = the sentry that keeps watching).

## Ecosystem role

Canary is the **gate layer** downstream of Provenance: Tolvi captures decisions, Provenance ranks a diff's risk against them, Canary turns that risk into the executable test selection the pipeline runs. Tolvi-native — the vault decides what can't be skipped. This is the compounding role the compliance framing never had.

## Immediate cleanup (not a build)

The marketing-site `future-projects/Canary/` material (`Compliance_Canary_BRD.docx`, `Compliance_Canary_TRD.docx`) still describes the old compliance product. It should be reframed onto the test-selection thesis before the page goes public.

## Next step

Run `docs/test-selection-kickoff-prompt.md` in this repo to produce a plan for the thinnest slice, then harden it through Bastion before any code.
