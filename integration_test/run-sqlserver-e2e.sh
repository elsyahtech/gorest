#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
report_dir="${GOREST_IT_REPORT_DIR:-$repo_root/integration_test/reports}"
run_id="$(date -u +%Y%m%dT%H%M%SZ)-$$"
events_report="$report_dir/sqlserver-http-$run_id.jsonl"
summary_report="$report_dir/sqlserver-http-$run_id.summary.json"
started_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
sqlserver_image="${GOREST_IT_SQLSERVER_IMAGE:-mcr.microsoft.com/mssql/server:2022-latest}"

if [[ "${GOREST_IT_SQLSERVER_ACCEPT_EULA:-}" != "Y" ]]; then
	cat >&2 <<'EOF'
SQL Server's container image requires acceptance of Microsoft's license terms.
Review the terms first, then rerun with GOREST_IT_SQLSERVER_ACCEPT_EULA=Y if you accept them.
EOF
	exit 2
fi

mkdir -p "$report_dir"
cd "$repo_root"

test_command='go test -json -tags=integration,integrationcontainers ./integration_test/httpapi -run ^TestSQLServerHTTPCRUDAndParameterBinding$ -count=1'
printf 'SQL Server integration image: %s\n' "$sqlserver_image"
if command -v docker >/dev/null 2>&1; then
	if docker image inspect "$sqlserver_image" >/dev/null 2>&1; then
		printf 'SQL Server image is already available locally.\n'
	else
		printf 'SQL Server image is not cached. Pulling it now; the first pull can take several minutes.\n'
		if ! docker pull "$sqlserver_image"; then
			printf 'SQL Server image pull failed. Check Docker connectivity and image availability.\n' >&2
			exit 1
		fi
	fi
else
	printf 'Docker CLI not found; Testcontainers will pull the image. The pull may not show progress here.\n'
fi
printf 'Starting SQL Server HTTP integration test. Test events will stream below.\n'
printf 'Run reports: %s\n' "$report_dir"
set +e
GOREST_IT_SQLSERVER_ACCEPT_EULA=Y GOREST_IT_SQLSERVER_IMAGE="$sqlserver_image" go test -json -tags=integration,integrationcontainers ./integration_test/httpapi -run '^TestSQLServerHTTPCRUDAndParameterBinding$' -count=1 2>&1 | tee "$events_report"
exit_code=$?
set -e

result="passed"
if [ "$exit_code" -ne 0 ]; then
	result="failed"
fi

cat >"$summary_report" <<EOF
{
  "started_at_utc": "$started_at",
  "driver": "sqlserver",
  "image": "$sqlserver_image",
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
