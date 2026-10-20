#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
report_dir="${GOREST_IT_REPORT_DIR:-$repo_root/integration_test/reports}"
run_id="$(date -u +%Y%m%dT%H%M%SZ)-$$"
events_report="$report_dir/oracle-http-$run_id.jsonl"
summary_report="$report_dir/oracle-http-$run_id.summary.json"
started_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
oracle_image="${GOREST_IT_ORACLE_IMAGE:-gvenzl/oracle-free:23.26.3-slim-faststart}"

mkdir -p "$report_dir"
cd "$repo_root"

test_command='go test -json -tags=integration,integrationcontainers ./integration_test/httpapi -run ^TestOracleHTTPCRUDAndParameterBinding$ -count=1'
printf 'Oracle integration image: %s\n' "$oracle_image"
if command -v docker >/dev/null 2>&1; then
	if docker image inspect "$oracle_image" >/dev/null 2>&1; then
		printf 'Oracle image is already available locally.\n'
	else
		printf 'Oracle image is not cached. Pulling it now; the first pull can take several minutes.\n'
		if ! docker pull "$oracle_image"; then
			printf 'Oracle image pull failed. Check Docker connectivity and image availability.\n' >&2
			exit 1
		fi
	fi
else
	printf 'Docker CLI not found; Testcontainers will pull the image. The pull may not show progress here.\n'
fi
printf 'Starting Oracle HTTP integration test. Test events will stream below.\n'
printf 'Run reports: %s\n' "$report_dir"
set +e
GOREST_IT_ORACLE_IMAGE="$oracle_image" go test -json -tags=integration,integrationcontainers ./integration_test/httpapi -run '^TestOracleHTTPCRUDAndParameterBinding$' -count=1 2>&1 | tee "$events_report"
exit_code=$?
set -e

result="passed"
if [ "$exit_code" -ne 0 ]; then
	result="failed"
fi

cat >"$summary_report" <<EOF
{
  "started_at_utc": "$started_at",
  "driver": "oracle",
  "image": "$oracle_image",
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
