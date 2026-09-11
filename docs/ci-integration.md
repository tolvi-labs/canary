# CI integration

`canary` is a platform-agnostic binary — this is the documented pattern for wiring it into a CI pipeline, not a published GitHub Action.

The workflow below is written for a Go repository. Everything except the final "run the selected tests" step is language-agnostic: the repo's `canary.yml` tells `canary` which coverage backend to use, and the CI job needs whatever toolchain that backend drives (a Go toolchain, `python3` with `pytest`/`pytest-cov`, or Node 22+ for `node:test`'s coverage support). See [Running the selected tests](#running-the-selected-tests) for the Python and Node forms of that last step.

## GitHub Actions

```yaml
name: canary
on:
  pull_request:
    branches: [main]
  push:
    branches: [main]

jobs:
  canary:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0   # full history is required to diff against the base ref

      - name: Build canary
        run: |
          git clone https://github.com/tolvi-labs/canary /tmp/canary
          cd /tmp/canary && go build -o /usr/local/bin/canary ./cmd/canary

      - name: Restore or build the global manifest
        run: |
          if [ ! -f .canary/global-manifest.json ]; then
            canary init   # add --lang go|python|node if auto-detection guesses wrong
          fi

      - name: Run the PR gate
        if: github.event_name == 'pull_request'
        run: |
          canary check \
            --base "origin/${{ github.base_ref }}" \
            --head "${{ github.sha }}" \
            --gate pr \
            --json-out report.json

      - name: Run the merge gate
        if: github.event_name == 'push'
        run: |
          canary check \
            --base "${{ github.event.before }}" \
            --head "${{ github.sha }}" \
            --gate merge \
            --json-out report.json

      - name: Post the report to the PR
        if: github.event_name == 'pull_request'
        run: |
          gh pr comment "${{ github.event.pull_request.number }}" --body-file report.json
        env:
          GH_TOKEN: ${{ github.token }}

      - name: Run the selected tests
        run: |
          tests=$(jq -r '.selected_tests[].test' report.json | paste -sd '|' -)
          go test -run "^($tests)$" ./...
```

A release/RC tag workflow follows the same shape with `--gate release` and a `base` computed from the previous release tag, always running the full suite regardless of the diff.

## Running the selected tests

`report.json`'s `selected_tests[].test` values are plain test names, in whatever form the repo's backend reports them: a Go test function (`TestAdd`), a pytest function (`test_add`), or a `node:test` name, which is free text and may contain spaces and regex metacharacters. Translating them into a runner invocation is the one language-specific step, and Canary's own exit code is not the enforcement point — it exits 0 on any successful selection, so it is *this* step that fails the build.

Every example below handles the empty selection: Canary legitimately selects zero tests for a change nothing covers (a docs-only PR), and both `pytest` and a bare `go test -run "^()$"` treat "nothing matched" as an error.

### Go — `go test -run`

```bash
tests=$(jq -r '.selected_tests[].test' report.json | paste -sd '|' -)
if [ -z "$tests" ]; then echo "no tests selected"; exit 0; fi
go test -run "^($tests)$" ./...
```

### Python — `pytest -k`

```bash
tests=$(jq -r '.selected_tests[].test' report.json | paste -sd ' or ' -)
if [ -z "$tests" ]; then echo "no tests selected"; exit 0; fi
pytest -k "$tests"
```

Two things to know about `-k`:

- It matches test names as **substrings**, not exact names, so a selected `test_add` also runs `test_add_negative`. That over-includes, which is the safe direction, but it means the run is a superset of Canary's selection rather than exactly it.
- `pytest` **exits 5** when `-k` deselects everything, and most CI configurations read a non-zero exit as a failed build. The guard above covers an empty selection; if you would rather tolerate exit 5 in general, run `pytest -k "$tests" || [ $? -eq 5 ]`.

### Node — `node --test --test-name-pattern`

`--test-name-pattern` takes a **regex** and matches it **unanchored**, so passing a bare name runs every test whose name contains it. Anchor each name with `^...$` and escape its regex metacharacters — the same treatment Canary's own Node backend applies internally when it instruments one test at a time (see `regexEscapeExact` in `internal/coverage/node/node.go`). The flag may be repeated, and the patterns OR together:

```bash
args=()
while IFS= read -r name; do
  # Escape regex metacharacters, then anchor for an exact match.
  escaped=$(printf '%s' "$name" | sed -e 's/[.[\*^$()+?{}|\\]/\\&/g')
  args+=("--test-name-pattern=^${escaped}$")
done < <(jq -r '.selected_tests[].test' report.json)

if [ ${#args[@]} -eq 0 ]; then echo "no tests selected"; exit 0; fi
node --test "${args[@]}"
```

Reading the names with `jq -r` line by line (rather than joining them) is deliberate: `node:test` names routinely contain spaces, so any delimiter-joining approach corrupts them.

A name nested in a `describe` block is matched on its own name, not a `suite > test` path — Canary records the individual `it`/`test` cases and never the suite, because a suite is not something a runner can be asked to run as a single test.

## Other CI providers

The same steps apply to any provider: check out with full history, build or cache the `canary` binary, run `canary init` once (or restore a cached `.canary/global-manifest.json`, refreshed via `canary refresh` on each subsequent run), run `canary check --gate <pr|merge|release> --json-out report.json` at the right trigger, post `report.json` however that provider's PR/MR-comment mechanism works, and feed `selected_tests` into the actual test-runner invocation. Canary's own exit code is not the enforcement point — it always exits 0 on a successful selection; the CI step that runs the selected tests is what can fail the build.
