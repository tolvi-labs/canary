# CI integration

`canary` is a platform-agnostic binary — this is the documented pattern for wiring it into a CI pipeline, not a published GitHub Action.

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
            canary init
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

## Other CI providers

The same steps apply to any provider: check out with full history, build or cache the `canary` binary, run `canary init` once (or restore a cached `.canary/global-manifest.json`, refreshed via `canary refresh` on each subsequent run), run `canary check --gate <pr|merge|release> --json-out report.json` at the right trigger, post `report.json` however that provider's PR/MR-comment mechanism works, and feed `selected_tests` into the actual test-runner invocation. Canary's own exit code is not the enforcement point — it always exits 0 on a successful selection; the CI step that runs the selected tests is what can fail the build.
