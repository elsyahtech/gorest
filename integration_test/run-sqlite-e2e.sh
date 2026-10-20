#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
report_dir="${GOREST_IT_REPORT_DIR:-$repo_root/integration_test/reports}"
run_id="$(date -u +%Y%m%dT%H%M%SZ)-$$"
events_report="$report_dir/sqlite-http-$run_id.jsonl"
summary_report="$report_dir/sqlite-http-$run_id.summary.json"
started_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

mkdir -p "$report_dir"
cd "$repo_root"

test_command='go test -json -tags=integration,integrationcontainers ./integration_test/httpapi -run ^TestSQLiteHTTPCRUDAndParameterBinding$ -count=1'
printf 'Starting SQLite HTTP integration test. No database container is required.\n'
printf 'SQLite uses an isolated temporary database file; test events will stream below.\n'
printf 'Run reports: %s\n' "$report_dir"
set +e
go test -json -tags=integration,integrationcontainers ./integration_test/httpapi -run '^TestSQLiteHTTPCRUDAndParameterBinding$' -count=1 2>&1 | tee "$events_report"
exit_code=$?
set -e

result="passed"
if [ "$exit_code" -ne 0 ]; then
	result="failed"
fi

cat >"$summary_report" <<EOF
{
  "started_at_utc": "$started_at",
  "driver": "sqlite",
  "database": "temporary file",
  "command": "$test_command",
  "result": "$result",
  "exit_code": $exit_code,
  "events_report": "$(basename "$events_report")"
}
EOF

printf 'Integration result: %s\n' "$result"
printf 'Summary: %s\n' "$summary_report"
printf 'Test events: %s\n' "$events_report"
exit "$exit_code"
