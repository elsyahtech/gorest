#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
report_dir="${GOREST_IT_REPORT_DIR:-$repo_root/integration_test/reports}"
run_id="$(date -u +%Y%m%dT%H%M%SZ)-$$"
events_report="$report_dir/app-filter-log-$run_id.jsonl"
summary_report="$report_dir/app-filter-log-$run_id.summary.json"
started_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
test_command='go test -json -tags=integration ./integration_test/app ./integration_test/filter ./integration_test/log -count=1'

mkdir -p "$report_dir"
cd "$repo_root"

printf 'Running app, filter, and logger integration tests.\n'
printf 'Test results will be saved under: %s\n' "$report_dir"
set +e
go test -json -tags=integration ./integration_test/app ./integration_test/filter ./integration_test/log -count=1 2>&1 |
	tee "$events_report" |
	awk -F'"' '
	{
		action = package = test = ""
		for (i = 1; i < NF; i++) {
			if ($i == "Action") action = $(i + 2)
			if ($i == "Package") package = $(i + 2)
			if ($i == "Test") test = $(i + 2)
		}
		if (test != "" && action == "run") printf "RUN  %s\n", test
		if (test != "" && action == "pass") printf "PASS %s\n", test
		if (test != "" && action == "fail") printf "FAIL %s\n", test
		if (test == "" && action == "fail") printf "FAIL %s\n", package
	}'
exit_code=${PIPESTATUS[0]}
set -e

result="passed"
if [ "$exit_code" -ne 0 ]; then
	result="failed"
fi
finished_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

cat >"$summary_report" <<EOF
{
  "started_at_utc": "$started_at",
  "finished_at_utc": "$finished_at",
  "suite": "app-filter-log",
  "packages": [
    "./integration_test/app",
    "./integration_test/filter",
    "./integration_test/log"
  ],
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
