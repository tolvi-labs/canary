---
tags: [decision, canary]
date: 2026-07-04
repo: canary
status: active
ticket: none
user_impact: none
product_area: Prioritization
---

# Canary is an ecosystem orphan — satisfy the internal need cheaply, defer the OSS product

**Date:** 2026-07-04
**Repo:** canary

## Why

Canary solves a real, validated internal problem — HIPAA/SOC 2 config-drift monitoring for Corvin Health and Claimblast, which have no clean off-the-shelf answer. But the guiding test for what to build next is "does it compound with the Tolvi vault and improve the engineering lifecycle we already dogfood," and Canary is the one candidate that does neither: it stands beside the ecosystem rather than strengthening it.

## How

- **It is a correctness/context play** — the durable value is a curated, cited catalog (~45–55 checks mapped to specific HIPAA §164 clauses and SOC 2 CC IDs) plus a hardened read-only, least-privilege deployment topology. That is defensible while curated, and the per-check YAML+function+fixture shape is highly parallelizable for agentic build.
- **But it is an ecosystem orphan:** no Tolvi vault dependency, no compounding with vault / Guild / Forge. Its moat is a catalog that decays without ongoing quarterly curation — a standing maintenance tax that does not feed the rest of the stack. Monetization is irrelevant to the decision (all Tolvi products are OSS and unmonetized); the disqualifier is non-compounding, not revenue.
- **The near-term need is capturable more cheaply:** the immediate payoff (audit TA's own regulated projects) is ~80% reachable with a thin internal skill/script, without the ~10-week OSS-grade build, community stewardship, and supply-chain hardening.
- **Plan:** build the thin internal audit tool when Corvin/Claimblast's compliance timeline forces it; defer the full OSS product; revisit priority only if Canary can be re-scoped to write findings into a Tolvi vault so it stops being an orphan and starts feeding the substrate.
- **Rejected: build the 10-week standalone OSS product next.** It is well-specified and internally useful, but it is the weakest fit against the compounds-with-the-vault test, and cheaper means capture most of the near-term value.

## Outcome

Canary is committed to a cheap internal-first path with the OSS product deferred, and its priority is explicitly gated on finding a way for it to compound with the vault ecosystem.
