# Compliance Canary — Go-Forward Plan

**Status:** Pre-build. Real internal need, but an ecosystem orphan. Lowest priority as a standalone product.
**Last updated:** 2026-07-04

## One-line

An OSS, read-only CLI that audits a Firebase/GCP project against a curated catalog of HIPAA Security Rule and SOC 2 checks and emits a structured findings report — continuous controls monitoring for config drift between formal audits. Manual mode (one command, terminal report) and scheduled mode (weekly Cloud Run job, GCS-persisted, diffed, Slack summary).

## Why it is attractive

- **Real, validated internal need:** Torres Atlantic's own regulated projects — Corvin Health and Claimblast — need config-drift monitoring and there is no clean off-the-shelf answer for small teams (enterprise CSPM is priced for security budgets; OSS tools skew enterprise/rules-engine-heavy).
- **Correctness/context play, not capability:** the value is the *curated, cited catalog* — ~45–55 checks each tied to a specific HIPAA §164 clause and SOC 2 CC ID, plus a hardened least-privilege, read-only deployment topology. A frontier model can write a check; it can't be the vetted, versioned catalog an auditor trusts.
- **Build shape suits agentic execution:** each check is an independent YAML + function + fixture, so the bulk (Phase 5, ~45 checks) is highly parallelizable.

## Why it is deprioritized

- **Ecosystem orphan.** Canary has no Tolvi vault dependency and does not compound with the vault, Guild, or Forge. Every other initiative strengthens the ecosystem's core bet; Canary sits beside it. Under the "every build dollar should compound with the vault" test, it is the weakest fit.
- **Maintenance tax with no compounding return.** The moat is a catalog that decays the moment quarterly curation stops — a standing time commitment that does not feed the rest of the stack. (Monetization is not a factor either way; all Tolvi products are OSS and unmonetized.)
- **The near-term need is capturable more cheaply.** The immediate payoff — audit TA's own Corvin/Claimblast projects — is ~80% reachable with a thin internal skill or script, without the 10-week OSS-grade build, community stewardship, and supply-chain hardening.

## Plan

1. **Satisfy the internal need first, cheaply:** a thin internal audit tool/skill covering the handful of highest-value HIPAA/SOC 2 config checks for Corvin and Claimblast, built when the compliance timeline forces it.
2. **Defer the full OSS product** until either the internal tool proves the catalog is worth generalizing, or there is a way to make Canary compound with the ecosystem.
3. **Open question to revisit:** could Canary be re-scoped to write its findings into a Tolvi vault (decisions/patterns about the security posture), so it stops being an orphan and starts feeding the substrate? If so, its priority changes.

## Ecosystem role

Currently none — that is the core problem. See [[2026-07-04-canary-internal-orphan-defer]] and, for the composition thesis the other initiatives share, [[2026-07-04-guild-correctness-recall-reframe]].
