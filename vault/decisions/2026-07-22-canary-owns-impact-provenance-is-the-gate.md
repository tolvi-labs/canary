---
tags: [decision, canary]
date: 2026-07-22
repo: canary
status: active
ticket: none
user_impact: medium
product_area: Product scope
refines: 2026-07-08-canary-reframe-test-selection
---

# Canary owns the impact reasoning; Provenance is repurposed to the capture-gate and no longer hands Canary a reachability-derived impact report

**Date:** 2026-07-22
**Repo:** canary

## Why

[[2026-07-08-canary-reframe-test-selection]] committed Canary to vault-informed CI test selection that *consumes Provenance's impacted-code output and does not redo reachability*. That boundary assumed Provenance was the impact reasoner. Provenance has since been repurposed away from impact reporting to a capture-enforcement gate at the code→push boundary (see [[2026-07-22-provenance-is-the-capture-enforcement-gate]]), so the impact reasoning it used to supply no longer exists in that form. This refines Canary's boundary without disturbing its identity: the test-selection reframe, the coverage+vault-bindings map, the two-manifest model, and the gate policy all stand. What changes is what Canary receives from Provenance and where the impact report is produced.

## How

- **Canary's identity is unchanged.** Vault-informed CI test selection: given a change, emit the exact tests worth running, ranked by the vault, and gate the pipeline at PR / merge / release. The coverage-instrumented test↔code map plus vault-declared "never auto-skip" bindings remain the moat, and the globalManifest/localManifest model is untouched.
- **The impact report moves here.** The human-readable "what does this diff touch, ranked by risk" report — retired as Provenance's standalone hero — is produced by Canary as a byproduct of the same pass that selects tests, and surfaces on the PR alongside the executable selection. One computation, two views: the eyeball-it report and the gate.
- **What Provenance now hands Canary.** Not a reachability-derived impact report, but the *declared provenance + governance*: the captured why/who/authority for the change and the flags for which vault decisions govern the paths the diff touched. Canary derives the objective changed-set from the diff and its coverage map, and uses Provenance's declared risk and vault bindings to *rank* and to enforce the never-skip overrides.
- **"Does not redo reachability" still holds, correctly read.** Canary maps changed symbols to tests through its coverage map, not through a cross-language reachability engine, and it never needed one. Nothing in this refinement adds reachability work to Canary; it adds the risk/governance input it now takes from the capture-gate instead of from an impact reporter.
- **The gate division is clean.** Provenance gates the *record* at code→push (hard-block in CI when capture is missing or inconsistent with the diff); Canary gates the *tests* at PR / merge / release. Two gates, two artifacts, no overlap. Bastion still gates the *plan* pre-code.
- **Guild-alive reconciliation.** [[2026-07-08-canary-reframe-test-selection]] invokes "the boundary discipline that killed Guild." Guild was relaunched 2026-07-21 and shipped public 2026-07-22, so it is alive as the plan/brainstorm surface. The boundary discipline itself stands (every tool owns a job no other tool does); only the "Guild is dead" framing is corrected. The suite line is Guild (plan) → Bastion (harden plan) → code → Provenance (guard the record) → Canary (guard the tests) → prod.

## Outcome

Canary keeps its vault-informed CI test-selection identity intact and additionally owns the impact/blast-radius report (surfaced on the PR), which is retired as Provenance's hero. Provenance is repurposed to the capture-gate and now hands Canary declared provenance + vault governance rather than a reachability-derived impact report; Canary derives the changed-set from the diff and its coverage map and uses the handoff to rank and to enforce never-skip bindings. This refines [[2026-07-08-canary-reframe-test-selection]] (which stays active) and corrects its dead-Guild premise. `docs/PLAN.md` and `docs/test-selection-kickoff-prompt.md` still describe consuming Provenance's impact output and a dead-Guild premise and need a reconciliation pass.
