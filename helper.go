package gorest

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/elsyahtech/gorest/database"
	"github.com/elsyahtech/gorest/filter"
)

const (
	startupRetryAttempts    = 3
	insecureApprovalTimeout = 30 * time.Second
)

var (
	insecureProductionPromptInput  io.Reader = os.Stdin
	insecureProductionPromptOutput io.Writer = os.Stdout
	isInteractiveTerminal                    = defaultIsInteractiveTerminal
)

func defaultIsInteractiveTerminal() bool {
	file, ok := insecureProductionPromptInput.(*os.File)
	if !ok {
		return false
	}

	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func validateProductionSQLiteHost(host string) error {
	host = strings.TrimSpace(strings.ToLower(host))
	switch host {
	case "", "localhost", "127.0.0.1", "::1":
		return nil
	default:
		return fmt.Errorf("SQLite database must use local storage in production; remote host %q is not supported", host)
	}
}

func (app *App) insecureProductionApproved(componentApproved bool) bool {
	return app != nil && (app.config.AllowInsecureProduction || componentApproved)
}

func (app *App) requestInsecureProductionApproval(component string, componentApproved bool) (bool, error) {
	if app.insecureProductionApproved(componentApproved) {
		return true, nil
	}
	if !isInteractiveTerminal() {
		return false, fmt.Errorf("%s is configured without TLS in production; add AllowInsecureProduction=true or enable TLS before startup", component)
	}

	fmt.Fprintf(insecureProductionPromptOutput, "\nProduction environment uses %s without TLS. Continue without encrypted transport? [y/N]: ", component)
	answerChannel := make(chan string, 1)
	readErrorChannel := make(chan error, 1)
	go func() {
		answer, err := bufio.NewReader(insecureProductionPromptInput).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			readErrorChannel <- err
			return
		}
		answerChannel <- answer
	}()

	var answer string
	select {
	case answer = <-answerChannel:
	case err := <-readErrorChannel:
		return false, fmt.Errorf("read %s insecure-production approval: %w", component, err)
	case <-time.After(insecureApprovalTimeout):
		return false, fmt.Errorf("%s startup cancelled: insecure-production approval timed out after %s; default was no", component, insecureApprovalTimeout)
	}

	approved := strings.EqualFold(strings.TrimSpace(answer), "y") || strings.EqualFold(strings.TrimSpace(answer), "yes")
	if !approved {
		return false, fmt.Errorf("%s startup cancelled: production operator did not approve non-TLS transport", component)
	}

	fmt.Fprintf(insecureProductionPromptOutput, "Warning: %s will continue with TLS disabled.\n", component)
	return true, nil
}

func retryStartup[T any](ctx context.Context, operation func(context.Context) (T, string, error)) (T, string, error) {
	var zero T
	for attempt := 1; attempt <= startupRetryAttempts; attempt++ {
		value, message, err := operation(ctx)
		if err == nil || !isTransientStartupError(err) || attempt == startupRetryAttempts {
			return value, message, err
		}

		timer := time.NewTimer(time.Duration(attempt) * 500 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return zero, message, ctx.Err()
		case <-timer.C:
		}
	}
	return zero, "", errors.New("startup retry exhausted")
}

func isTransientStartupError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	message := strings.ToLower(err.Error())
	for _, marker := range []string{"connection refused", "connection reset", "no such host", "i/o timeout", "timeout", "temporarily unavailable", "deadline exceeded", "server selection timeout"} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func (app *App) logProductionWarnings() {
	if app.config.database != nil && database.NormalizeDatabaseDriver(app.config.database.Driver) == database.SQLITE && !isProductionEnvironment(app.config.Environment) {
		app.Log(Map{
			"environment": app.config.Environment,
			"driver":      database.SQLITE,
			"message":     "SQLite is recommended only for local development, testing, and single-node workloads.",
		}, true).Warn("SQLite is not recommended for distributed production workloads")
	}
	if !isProductionEnvironment(app.config.Environment) {
		return
	}

	if warning, shouldWarn := productionDatabaseTLSWarning(app.config.database); shouldWarn {
		approved := app.insecureProductionApproved(app.config.database.AllowInsecureProduction)
		level := "database transport encryption is disabled in production without explicit approval"
		if approved {
			level = "database transport encryption is disabled by explicit production approval"
		}
		app.Log(Map{
			"environment": app.config.Environment,
			"driver":      database.NormalizeDatabaseDriver(app.config.database.Driver),
			"tls_mode":    app.config.database.TLS,
			"approved":    approved,
			"message":     warning,
		}, true).Warn(level)
	}

	if app.config.database != nil && database.NormalizeDatabaseDriver(app.config.database.Driver) == database.SQLITE {
		app.Log(Map{
			"environment": app.config.Environment,
			"driver":      database.SQLITE,
			"message":     "SQLite is embedded and local; use a server database for horizontally scaled or remote production workloads.",
		}, true).Warn("SQLite is being used in a production environment")
	}

	if app.config.redis != nil && !app.config.redis.Disable && !app.config.redis.TLS {
		level := "Redis transport encryption is disabled by explicit production approval"
		if !app.insecureProductionApproved(app.config.redis.AllowInsecureProduction) {
			level = "Redis transport encryption is disabled in production without explicit approval"
		}
		app.Log(Map{
			"environment": app.config.Environment,
			"tls_enabled": app.config.redis.TLS,
			"approved":    app.insecureProductionApproved(app.config.redis.AllowInsecureProduction),
			"message":     "Use TLS for untrusted networks; trusted local/loopback deployments may explicitly opt out.",
		}, true).Warn(level)
	}

	if warning, shouldWarn := productionFilterCookieWarning(app.config.filter); shouldWarn {
		app.Log(Map{
			"environment": app.config.Environment,
			"message":     warning,
		}, true).Warn("JWT cookie transport security is not guaranteed in a production environment")
	}
}

func (app *App) startLifecycle() {
	quit := make(chan os.Signal, 1)

	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(quit)

	startupCtx, cancelStartup := context.WithCancel(context.Background())
	defer cancelStartup()

	app.context = startupCtx

	shutdownSignal := make(chan os.Signal, 1)
	go forwardShutdownSignal(startupCtx, quit, cancelStartup, shutdownSignal)

	app.runStartup(startupCtx, shutdownSignal)
}

func forwardShutdownSignal(
	startupCtx context.Context,
	quit <-chan os.Signal,
	cancelStartup context.CancelFunc,
	shutdownSignal chan<- os.Signal,
) {
	select {
	case sig := <-quit:
		shutdownSignal <- sig

		cancelStartup()
	case <-startupCtx.Done():
	}
}

func (app *App) runStartup(startupCtx context.Context, shutdownSignal <-chan os.Signal) {
	if err := app.runMigration(); err != nil {
		if startupCtx.Err() != nil {
			app.shutdownAfterSignal(<-shutdownSignal)

			return
		}

		app.abortStartup(err)

		return
	}

	if startupCtx.Err() != nil {
		app.shutdownAfterSignal(<-shutdownSignal)

		return
	}

	if app.httpServerDisabled() {
		app.Log(nil, true).Info("gorest is ready; HTTP server is disabled")
		app.waitForShutdown(startupCtx, shutdownSignal)

		return
	}

	app.startHTTPServer(startupCtx, shutdownSignal)
}

func (app *App) httpServerDisabled() bool {
	return app.config.Debug || app.server == nil || app.config.server == nil
}

func (app *App) waitForShutdown(startupCtx context.Context, shutdownSignal <-chan os.Signal) {
	select {
	case sig := <-shutdownSignal:
		app.shutdownAfterSignal(sig)
	case <-startupCtx.Done():
		app.shutdownAfterSignal(<-shutdownSignal)
	}
}

func (app *App) startHTTPServer(startupCtx context.Context, shutdownSignal <-chan os.Signal) {
	app.server.ExpectListen()

	serverDone := make(chan error, 1)
	go func() {
		serverDone <- app.startServer()
	}()

	listenerReady := make(chan error, 1)
	go func() {
		listenerReady <- app.server.WaitForListening(startupCtx)
	}()

	if !app.waitForListenerReady(startupCtx, shutdownSignal, serverDone, listenerReady) {
		return
	}

	app.Log(nil, true).Info("server started successfully")
	app.Log(nil, true).Info("gorest is running")
	app.waitForHTTPServerShutdown(shutdownSignal, serverDone)
}

func (app *App) waitForListenerReady(
	startupCtx context.Context,
	shutdownSignal <-chan os.Signal,
	serverDone <-chan error,
	listenerReady <-chan error,
) bool {
	ctx := startupCtx
	if ctx == nil {
		ctx = context.Background()
	}

	select {
	case <-ctx.Done():
		select {
		case sig := <-shutdownSignal:
			app.shutdownAfterSignal(sig)
		default:
		}

		return false
	case sig := <-shutdownSignal:
		app.shutdownAfterSignal(sig)

		return false
	case err := <-serverDone:
		if err != nil {
			app.abortStartup(err)
			return false
		}

		app.abortStartup(errors.New("HTTP server stopped before becoming ready"))

		return false
	case err := <-listenerReady:
		if err != nil {
			app.handleListenerReadyError(startupCtx, shutdownSignal, err)

			return false
		}
	}

	return true
}

func (app *App) handleListenerReadyError(startupCtx context.Context, _ <-chan os.Signal, err error) {
	if startupCtx.Err() != nil {
		return
	}

	app.abortStartup(err)
}

func (app *App) waitForHTTPServerShutdown(shutdownSignal <-chan os.Signal, serverDone <-chan error) {
	select {
	case sig := <-shutdownSignal:
		app.shutdownAfterSignal(sig)
	case err := <-serverDone:
		if err != nil {
			app.abortStartup(fmt.Errorf("HTTP server stopped: %w", err))
			return
		}

		app.Log(nil, true).Warn("HTTP server stopped unexpectedly")
		app.closeAndReport()
	}
}

func productionDatabaseTLSWarning(cfg *database.Config) (string, bool) {
	if cfg == nil || cfg.Disable {
		return "", false
	}

	driver := database.NormalizeDatabaseDriver(cfg.Driver)
	tlsMode := strings.ToLower(strings.TrimSpace(cfg.TLS))

	switch driver {
	case database.SQLITE:
		return "", false
	case database.MYSQL:
		return mysqlCaseOnProductionDatabaseTLSWarning(cfg, tlsMode)
	case database.POSTGRES:
		return postgresCaseOnProductionDatabaseTLSWarning(tlsMode)
	case database.SQLSERVER:
		return sqlServerCaseOnProductionDatabaseTLSWarning(cfg, tlsMode)
	case database.ORACLE:
		return oracleCaseOnProductionDatabaseTLSWarning(cfg, tlsMode)
	case database.MONGO:
		return mongoCaseOnProductionDatabaseTLSWarning(tlsMode)
	case database.SCYLLA:
		if tlsMode != database.TLSEnabled {
			return "ScyllaDB TLS is disabled. Set TLS to \"enable\" and configure certificate verification for production.", true
		}

		return "", false
	default:
	}

	return "", false
}

func productionFilterCookieWarning(cfg *filter.Config) (string, bool) {
	if cfg == nil || cfg.CookieSecure {
		return "", false
	}

	return "JWT cookies are configured without the Secure attribute. " +
		"They may be sent over unencrypted HTTP; set CookieSecure to true when clients use HTTPS. " +
		"This warning does not prevent startup and may be acceptable for trusted intranet deployments.", true
}

func isProductionEnvironment(environment string) bool {
	switch strings.ToLower(strings.TrimSpace(environment)) {
	case "", Development, Testing:
		return false
	default:
		return true
	}
}

func mysqlCaseOnProductionDatabaseTLSWarning(cfg *database.Config, tlsMode string) (string, bool) {
	if cfg.SSLCAPath == "" || tlsMode != database.TLSTrue {
		return "MySQL TLS is disabled or cannot be verified from this configuration. " +
			"Set TLS to \"true\" to use the driver's system certificate roots, or register and verify a go-sql-driver/mysql TLS profile; " +
			"GOREST only passes the TLS option when SSLCAPath is set.", true
	}

	return "", false
}

func postgresCaseOnProductionDatabaseTLSWarning(tlsMode string) (string, bool) {
	switch tlsMode {
	case "", database.TLSDisabled:
		return "PostgreSQL TLS is disabled. Set TLS to \"verify-full\" and configure SSLCAPath with the trusted CA certificate for production.", true
	case "require":
		return "PostgreSQL traffic is encrypted, but TLS mode \"require\" does not ensure server identity verification. " +
			"Prefer TLS \"verify-full\" with the trusted CA configured in SSLCAPath.", true
	case "verify-ca":
		return "PostgreSQL verifies the certificate chain but not the server hostname. " +
			"Prefer TLS \"verify-full\" with the trusted CA configured in SSLCAPath.", true
	case "allow", "prefer":
		return "PostgreSQL TLS mode \"" + tlsMode + "\" may fall back to " +
			"an unencrypted connection. Set TLS to \"verify-full\" and configure SSLCAPath with the trusted CA certificate for production.", true
	}

	return "", false
}

func sqlServerCaseOnProductionDatabaseTLSWarning(cfg *database.Config, tlsMode string) (string, bool) {
	if tlsMode == "" || tlsMode == database.TLSDisabled || tlsMode == database.TLSFalse ||
		tlsMode == database.TLSOptional || cfg.TrustServerCertificate {
		return "SQL Server encryption is disabled or optional. Set TLS " +
			"to \"mandatory\" or \"strict\" and keep TrustServerCertificate false so the server certificate is validated.", true
	}

	return "", false
}

func oracleCaseOnProductionDatabaseTLSWarning(cfg *database.Config, tlsMode string) (string, bool) {
	if tlsMode != database.TLSEnabled || cfg.SSLCAPath == "" {
		return "Oracle TLS is not configured. Set TLS to \"enable\" and " +
			"SSLCAPath to the Oracle wallet directory for a TCPS connection.", true
	}

	return "", false
}

func mongoCaseOnProductionDatabaseTLSWarning(tlsMode string) (string, bool) {
	if tlsMode != database.TLSTrue {
		return "MongoDB TLS is disabled. " +
			"Set TLS to \"true\" and configure certificate verification for production.", true
	}

	return "", false
} //nolint:revive
