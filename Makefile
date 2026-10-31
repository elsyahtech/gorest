.PHONY: help run lint upgrade fieldalignment clean unit-coverage integration-test-coverage integration-test-app-filter-log integration-test-postgres-e2e integration-test-oracle-e2e integration-test-sqlserver-e2e integration-test-mysql-e2e integration-test-sqlite-e2e integration-test-mongo-e2e integration-test-scylla-e2e integration-test-redis-e2e

help:
	@printf '%s\n' \
		'Available Make targets:' \
		'  help                           Show this help message' \
		'  run                            Run the application' \
		'  lint                           Run golangci-lint across the repository' \
		'  unit-coverage                  Run unit tests and write production coverage report' \
		'  upgrade                        Upgrade Go dependencies and tidy go.mod' \
		'  fieldalignment                 Apply fieldalignment fixes' \
		'  clean                          Clear Go build, test, module, and fuzz caches' \
		'  integration-test-app-filter-log Run app, filter, and logger integration tests with reports' \
		'  integration-test-postgres-e2e  Run managed PostgreSQL HTTP integration tests' \
		'  integration-test-oracle-e2e    Run managed Oracle HTTP integration tests' \
		'  integration-test-sqlserver-e2e Run managed SQL Server HTTP integration tests' \
		'  integration-test-mysql-e2e     Run managed MySQL HTTP integration tests' \
		'  integration-test-sqlite-e2e    Run managed SQLite HTTP integration tests' \
		'  integration-test-mongo-e2e     Run managed MongoDB HTTP integration tests' \
		'  integration-test-scylla-e2e    Run managed ScyllaDB HTTP integration tests' \
		'  integration-test-redis-e2e     Run managed Redis integration tests' \
		'  integration-test-coverage      Run managed integration tests and generate coverage reports' \
		'' \
		'Go commands (not Make targets):' \
		'  go test ./...                                      Run unit tests' \
		'  go test -cover ./...                              Run unit tests with coverage' \
		'  go build ./...                                    Build all packages' \
		'  go test -tags=integration ./integration_test/...   Run configured service integration suites' \
		'  make integration-test-redis-e2e                   Run Redis tests with a disposable container' \
		'' \
		'Examples:' \
		'  make help' \
		'  make integration-test-sqlite-e2e' \
		'  go test ./...'

run:
	@go run ./cmd

integration-test-app-filter-log:
	@./integration_test/run-app-filter-log.sh

integration-test-postgres-e2e:
	@./integration_test/run-postgres-e2e.sh

integration-test-oracle-e2e:
	@./integration_test/run-oracle-e2e.sh

integration-test-sqlserver-e2e:
	@./integration_test/run-sqlserver-e2e.sh

integration-test-mysql-e2e:
	@./integration_test/run-mysql-e2e.sh

integration-test-sqlite-e2e:
	@./integration_test/run-sqlite-e2e.sh

integration-test-mongo-e2e:
	@./integration_test/run-mongo-e2e.sh

integration-test-scylla-e2e:
	@./integration_test/run-scylla-e2e.sh

integration-test-redis-e2e:
	@go test -tags=integration,integrationcontainers -count=1 -timeout=10m ./integration_test/redis

lint:
	@echo "Running only golangci-lint..."
	golangci-lint run ./...

unit-coverage:
	@mkdir -p integration_test/reports
	@coverage_file="integration_test/reports/unit-coverage.out"; \
	production_packages=$$(go list ./... | grep -v -E '/integration_test/|/cmd$$|/coba$$' | paste -sd, -); \
	go test -coverpkg="$$production_packages" -coverprofile="$$coverage_file" ./... && \
	go tool cover -func="$$coverage_file" | tail -n 1

integration-test-coverage:
	@if [ "$$GOREST_IT_SQLSERVER_ACCEPT_EULA" != "Y" ]; then \
		printf '%s\n' "Review and accept Microsoft's SQL Server license, then set GOREST_IT_SQLSERVER_ACCEPT_EULA=Y."; \
		exit 2; \
	fi
	@mkdir -p integration_test/reports
	@coverage_file="integration_test/reports/integration-coverage-$$(date -u +%Y%m%dT%H%M%SZ).out"; \
	production_packages=$$(go list ./... | grep -v -E '/integration_test/|/cmd$$|/coba$$' | paste -sd, -); \
	printf 'Running complete managed integration matrix; coverage report: %s\n' "$$coverage_file"; \
	go test -p 1 -timeout=30m -tags=integration,integrationcontainers \
		-coverpkg="$$production_packages" \
		-coverprofile="$$coverage_file" \
		./... -count=1 && \
	go tool cover -func="$$coverage_file" | tail -n 1 && \
	html_file="$${coverage_file%.out}.html" && \
	go tool cover -html="$$coverage_file" -o="$$html_file" && \
	printf 'HTML coverage report: %s\n' "$$html_file"

upgrade:
	@echo "Running uprade dependency..."
	go get -u ./...
	go mod tidy

fieldalignment:
	fieldalignment -fix ./...

clean:
	go clean -cache
	go clean -testcache
	go clean -modcache
	go clean -fuzzcache
