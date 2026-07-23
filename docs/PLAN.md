# Tolvi Canary — Go-Forward Plan

**Status:** Pre-build. Vault-informed CI test selection; the name is reclaimed. Active evidence-first probe. Canary now also owns the impact report (moved here from Provenance, which became the capture-gate).
**Last updated:** 2026-07-22

## One-line

An OSS CLI that, given a change, emits the exact set of tests worth running — the impacted subset instead of the whole E2E suite — so the inner loop stays fast and full verification is reserved for the merge/release boundary. The coverage map says which tests exercise which code; the Tolvi vault says which tests can never be skipped.

## The reframe (why the framing changed)

Canary was originally **Compliance Canary** — a HIPAA/SOC 2 config-drift auditor. It was benched on 2026-07-04 as an *ecosystem orphan*: a real internal need, but no vault dependency and no compounding with the rest of the stack. See [[2026-07-04-canary-internal-orphan-defer]] (superseded).

The path forward was to **reclaim the name for a different product**: vault-informed CI test selection, which compounds with the vault by construction. See [[2026-07-08-canary-reframe-test-selection]]. That decision assumed Canary would consume a reachability-derived impact report from Provenance. Provenance has since been repurposed to a capture-enforcement gate (see [[2026-07-22-provenance-is-the-capture-enforcement-gate]]), so Canary now **owns the impact reasoning itself** and receives declared provenance + governance from Provenance instead. Canary's identity is unchanged by this; only the boundary moved. See [[2026-07-22-canary-owns-impact-provenance-is-the-gate]].

## What it is

- **Canary owns impact and gates tests.** Given a change, Canary derives the objective changed-set from the diff and its coverage map, produces the human-readable impact report (surfaced on the PR), and turns the impacted set into the executable test selection the pipeline runs.
- **What Provenance hands it.** Not a reachability-derived impact report, but the declared provenance + vault governance for the change (the captured why / who / authority and the flags for which vault decisions govern the touched paths). Canary uses that to *rank* risk and to enforce the never-skip bindings.
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

- **(A) CI test-selection, full stop** — v1. Which tests run at which pipeline stage, plus the impact report that rides along on the PR.
- **(B) the cheap early-warning guard at every gate** — pre-merge the impacted subset, post-deploy the vault-chosen canary signals to watch before full rollout. Gives Canary its slot in the legal-system frame: Provenance keeps the record of what shipped; Canary is the sentry that keeps watching.

## Ecosystem role

Canary is the test-gate downstream of the capture-gate: the vault captures decisions, Provenance enforces at push time that a change's reasoning was captured and hands Canary the declared provenance + governance, and Canary turns the impacted set into the executable test selection the pipeline runs and gates on. Tolvi-native — the vault decides what can't be skipped. This is the compounding role the compliance framing never had.

## Boundary with the suite

Guild (plan) → Bastion (harden plan) → code → Provenance (guard the record) → **Canary (guard the tests)** → prod. Three gates, three artifacts, no overlap. Bastion still gates the plan pre-code; Provenance gates the record at push; Canary gates the tests at PR / merge / release.

## Immediate cleanup

This plan and the build kickoff prompt were reconciled on 2026-07-22: Canary now owns the impact report, receives declared provenance + governance from Provenance (not a reachability-derived impact output), and the stale "Guild is dead" premise is removed (Guild was relaunched 2026-07-21 and shipped public 2026-07-22). The marketing-site `future-projects/Canary/` material (`Compliance_Canary_BRD.docx`, `Compliance_Canary_TRD.docx`) still describes the old compliance product and needs reframing onto the test-selection thesis before the page goes public.

## Next step

Run `docs/test-selection-kickoff-prompt.md` in this repo to produce a plan for the thinnest slice, then harden it through Bastion before any code.
