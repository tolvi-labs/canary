# Test-selection — build kickoff prompt

Kickoff prompt for Canary's reframed hero use case (see [[2026-07-08-canary-reframe-test-selection]]). Deliberately **plan-first, not "go write it"** — hand it to an agent working in this repo, then harden its output through Bastion before any code.

---

```
You are helping design the first build slice of Tolvi Canary, an open-source tool in the Tolvi Labs suite.

READ FIRST, then summarize what you found before designing:
- vault/decisions/2026-07-08-canary-reframe-test-selection.md   (the reframe you are building toward)
- vault/decisions/2026-07-04-canary-internal-orphan-defer.md    (SUPERSEDED — the old Compliance Canary framing; do NOT resurrect it)
- docs/PLAN.md                                                   (go-forward plan)
- The sibling Provenance repo (~/tolvi-labs/provenance): its impact/blast-radius decision and kickoff — Canary consumes Provenance's output.
- The Tolvi and Tolvi Solo repos, to match stack, CLI conventions, and vault format.

CONTEXT: Tolvi is a per-repo "decision vault" — plain-Markdown records of a team's decisions, rejected alternatives, and incidents-that-became-rules, retrievable by an agent. Provenance, given a diff, emits a vault-ranked impact report (what's impacted, ranked by risk, explained by the vault). Canary is the tool that turns that risk into the EXECUTABLE test selection a CI pipeline runs.

GOAL — the hero use case:
Given a change, emit the exact set of tests worth running — the impacted subset instead of the whole E2E suite — so the inner loop stays fast and full verification is reserved for the merge/release boundary. Primary users: engineers, QA, release managers. Job-to-be-done: cut inner-loop test time without silently skipping a test that would have caught a regression.

NON-NEGOTIABLES (this is the whole point — don't drift):
1. Provenance reasons; Canary gates. Provenance answers code → risk. Canary answers code → tests → gate: which concrete tests prove the impacted code, and what runs at which pipeline stage. Consume Provenance's impacted-code output; do NOT re-implement reachability.
2. The map is coverage (base) + vault bindings (override). Coverage-instrument a full run for an honest test↔code graph; the vault adds "never auto-skip these even if the graph says untouched" rules (e.g. "cache layer touched → always run these integration tests, per the stale-cache incident"). The vault bindings are the moat — without them this is a me-too Launchable.
3. Two manifests. globalManifest = the full registry + coverage map: one durable, always-fresh index, refreshed incrementally at commit-time; it is the ONLY thing that can drift. localManifest = the per-PR subset, derived fresh from diff + globalManifest + vault rules; it cannot drift and must never accumulate (one fixed path, overwritten). Do not persist a manifest per commit.
4. Gate policy. PR / feature branch → localManifest (impacted subset). Merge to main → globalManifest (full suite). Release/RC tag → globalManifest. The full run at merge is deliberate: per-PR passes do NOT compose into a release guarantee (PRs interact; a localManifest was computed against a base that has since moved). Enforce the best practice and make the engineer conscious of it — NEVER silently automate the skip.
5. Boundary vs Forge: Forge drafts tests; Canary maps and gates them. Canary NEVER authors tests. `canary init` audits an existing suite and flags orphan tests + impacted-but-untested paths — that audit output is Forge's to-do list.
6. Scope: build (A) CI test-selection only. (B) prod-watch "sentry" is the stated north star, NOT this slice — do not design a prod-observability surface yet.

DELIVERABLE — an implementation plan for the thinnest slice, not code yet:
- Architecture + data flow: how it ingests a diff, consumes Provenance's impacted-code output, resolves impacted symbols → tests via the coverage map, applies vault bindings, and emits the localManifest.
- The globalManifest: its on-disk shape, how `canary init` builds it, how the commit-time modifier refreshes it, and the staleness check that guards against a map older than the code.
- The audit mode: how `canary init` reports orphan tests and impacted-but-untested paths.
- CI integration: how a pipeline invokes Canary at each gate (PR / merge-to-main / release) and consumes the manifest.
- The thinnest MVP that proves the vault-ranked test-selection thesis first, dogfoodable on one real repo, then what layers on.
- Open design questions and the riskiest assumptions, called out explicitly.

Be honest about where this is hard: coverage-map accuracy and refresh cost across languages, resolving impacted symbols → tests precisely, and the false-negative risk in the "safe to skip" decision (a wrong skip on a PR is the most expensive failure mode — the merge-boundary full run is the backstop, say so). Do not hand-wave those.
```

---

**How to use:** run this in this repo, then run the returned plan back through Bastion ("harden this plan against the vault") before writing any code. Keep this prompt at design altitude; the follow-up build prompt is just "implement Phase 1 of the hardened plan."
