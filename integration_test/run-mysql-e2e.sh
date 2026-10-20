#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
report_dir="${GOREST_IT_REPORT_DIR:-$repo_root/integration_test/reports}"
run_id="$(date -u +%Y%m%dT%H%M%SZ)-$$"
events_report="$report_dir/mysql-http-$run_id.jsonl"
summary_report="$report_dir/mysql-http-$run_id.summary.json"
started_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
mysql_image="${GOREST_IT_MYSQL_IMAGE:-mysql:8.4}"

mkdir -p "$report_dir"
cd "$repo_root"

test_command='go test -json -tags=integration,integrationcontainers ./integration_test/httpapi -run ^TestMySQLHTTPCRUDAndParameterBinding$ -count=1'
printf 'MySQL integration image: %s\n' "$mysql_image"
if command -v docker >/dev/null 2>&1; then
	if docker image inspect "$mysql_image" >/dev/null 2>&1; then
		printf 'MySQL image is already available locally.\n'
	else
		printf 'MySQL image is not cached. Pulling it now; the first pull can take several minutes.\n'
		if ! docker pull "$mysql_image"; then
			printf 'MySQL image pull failed. Check Docker connectivity and image availability.\n' >&2
			exit 1
		fi
	fi
else
	printf 'Docker CLI not found; Testcontainers will pull the image. The pull may not show progress here.\n'
fi
printf 'Starting MySQL HTTP integration test. Test events will stream below.\n'
printf 'Run reports: %s\n' "$report_dir"
set +e
GOREST_IT_MYSQL_IMAGE="$mysql_image" go test -json -tags=integration,integrationcontainers ./integration_test/httpapi -run '^TestMySQLHTTPCRUDAndParameterBinding$' -count=1 2>&1 | tee "$events_report"
exit_code=$?
set -e

result="passed"
if [ "$exit_code" -ne 0 ]; then
	result="failed"
fi

cat >"$summary_report" <<EOF
{
  "started_at_utc": "$started_at",
  "driver": "mysql",
  "image": "$mysql_image",
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
