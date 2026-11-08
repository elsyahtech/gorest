package hardening

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/elsyahtech/gorest/database"
)

const (
	reportSchemaVersion = "1.0"
	scannerVersion      = "1.0.0"
)

// Report is a JSON- and HTML-ready assessment of effective runtime config.
type Report struct {
	SchemaVersion string        `json:"schema_version"`
	Scanner       string        `json:"scanner_version"`
	GeneratedAt   time.Time     `json:"generated_at"`
	Profile       string        `json:"profile"`
	EvidenceMode  string        `json:"evidence_mode"`
	Warning       string        `json:"warning"`
	Summary       Summary       `json:"summary"`
	Inventory     []ConfigField `json:"inventory"`
	Findings      []Finding     `json:"findings"`
}

// ConfigField records one inventoried configuration value or redacted state.
type ConfigField struct {
	Path           string `json:"field_path"`
	Value          any    `json:"value"`
	EvidenceSource string `json:"evidence_source"`
	Status         string `json:"status"`
}

// Finding describes one configuration check and its evidence.
type Finding struct {
	ID             string `json:"id"`
	Status         string `json:"status"`
	Severity       string `json:"severity"`
	Confidence     string `json:"confidence"`
	FieldPath      string `json:"field_path"`
	EvidenceSource string `json:"evidence_source"`
	ObservedValue  any    `json:"observed_value,omitempty"`
	Message        string `json:"message"`
	Recommendation string `json:"recommendation"`
}

// Summary contains finding counts grouped by severity and status.
type Summary struct {
	Critical      int `json:"critical"`
	High          int `json:"high"`
	Medium        int `json:"medium"`
	Low           int `json:"low"`
	Informational int `json:"informational"`
	Pass          int `json:"pass"`
	Review        int `json:"review"`
	Fail          int `json:"fail"`
	Unknown       int `json:"unknown"`
	NotApplicable int `json:"not_applicable"`
}

// Scan inventories effective in-process configuration. It does not read source
// files, execute application code, or connect to external services.
func Scan(profile string, configs map[string]any) Report {
	if strings.TrimSpace(profile) == "" {
		profile = "unknown"
	}

	report := Report{
		SchemaVersion: reportSchemaVersion,
		Scanner:       scannerVersion,
		GeneratedAt:   time.Now().UTC(),
		Profile:       profile,
		EvidenceMode:  "effective_runtime_config",
		Warning:       "This report assesses only the effective GOREST configuration supplied to the running process. It is not a security certification and does not assess application code, infrastructure, external services, or deployment topology.",
		Inventory:     make([]ConfigField, 0),
		Findings:      make([]Finding, 0),
	}

	configNames := make([]string, 0, len(configs))
	for name := range configs {
		configNames = append(configNames, name)
	}
	sort.Strings(configNames)
	for _, name := range configNames {
		flattenConfig(&report.Inventory, name, configs[name])
	}

	assess(&report)
	summarize(&report)

	return report
}

func flattenConfig(inventory *[]ConfigField, path string, value any) {
	if value == nil {
		*inventory = append(*inventory, ConfigField{
			Path:           path,
			Value:          "not configured",
			EvidenceSource: "effective_runtime_config",
			Status:         "unknown",
		})
		return
	}

	valueRef := reflect.ValueOf(value)
	for valueRef.IsValid() && (valueRef.Kind() == reflect.Pointer || valueRef.Kind() == reflect.Interface) {
		if valueRef.IsNil() {
			*inventory = append(*inventory, ConfigField{
				Path:           path,
				Value:          "not configured",
				EvidenceSource: "effective_runtime_config",
				Status:         "unknown",
			})
			return
		}
		valueRef = valueRef.Elem()
	}

	if valueRef.Kind() == reflect.Struct && valueRef.Type() != reflect.TypeOf(time.Time{}) {
		fieldCount := 0
		for index := 0; index < valueRef.NumField(); index++ {
			fieldInfo := valueRef.Type().Field(index)
			if !fieldInfo.IsExported() {
				continue
			}
			fieldCount++
			flattenReflect(inventory, path+"."+fieldInfo.Name, valueRef.Field(index), fieldInfo.Name)
		}
		if fieldCount == 0 {
			*inventory = append(*inventory, ConfigField{
				Path:           path,
				Value:          "not assessable",
				EvidenceSource: "effective_runtime_config",
				Status:         "unknown",
			})
		}
		return
	}

	safe, status := safeReflectValue(valueRef, "")
	*inventory = append(*inventory, ConfigField{
		Path:           path,
		Value:          safe,
		EvidenceSource: "effective_runtime_config",
		Status:         status,
	})
}

func flattenReflect(inventory *[]ConfigField, path string, value reflect.Value, fieldName string) {
	if isSensitiveField(fieldName) {
		configured := value.IsValid() && !isZeroValue(value)
		*inventory = append(*inventory, ConfigField{
			Path:           path,
			Value:          map[string]bool{"configured": configured},
			EvidenceSource: "effective_runtime_config",
			Status:         "observed",
		})
		return
	}

	for value.IsValid() && (value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface) {
		if value.IsNil() {
			*inventory = append(*inventory, ConfigField{
				Path:           path,
				Value:          nil,
				EvidenceSource: "effective_runtime_config",
				Status:         "unknown",
			})
			return
		}
		value = value.Elem()
	}

	if !value.IsValid() {
		*inventory = append(*inventory, ConfigField{
			Path:           path,
			Value:          nil,
			EvidenceSource: "effective_runtime_config",
			Status:         "unknown",
		})
		return
	}
	if value.Kind() == reflect.Struct && value.Type() != reflect.TypeOf(time.Time{}) {
		for index := 0; index < value.NumField(); index++ {
			fieldInfo := value.Type().Field(index)
			if fieldInfo.IsExported() {
				flattenReflect(inventory, path+"."+fieldInfo.Name, value.Field(index), fieldInfo.Name)
			}
		}
		return
	}

	safe, status := safeReflectValue(value, fieldName)
	*inventory = append(*inventory, ConfigField{
		Path:           path,
		Value:          safe,
		EvidenceSource: "effective_runtime_config",
		Status:         status,
	})
}

func safeReflectValue(value reflect.Value, fieldName string) (any, string) {
	for value.IsValid() && (value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface) {
		if value.IsNil() {
			return nil, "unknown"
		}
		value = value.Elem()
	}
	if !value.IsValid() {
		return nil, "unknown"
	}

	switch value.Kind() {
	case reflect.Bool:
		return value.Bool(), "observed"
	case reflect.String:
		return value.String(), "observed"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if value.Type() == reflect.TypeOf(time.Duration(0)) {
			return time.Duration(value.Int()).String(), "observed"
		}
		return value.Int(), "observed"
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return value.Uint(), "observed"
	case reflect.Float32, reflect.Float64:
		return value.Float(), "observed"
	case reflect.Struct:
		if value.Type() == reflect.TypeOf(time.Time{}) {
			return value.Interface().(time.Time).UTC().Format(time.RFC3339Nano), "observed"
		}
		return "not assessable", "unknown"
	case reflect.Func, reflect.Chan, reflect.UnsafePointer:
		if value.IsNil() {
			return "not configured", "unknown"
		}
		return "configured opaque value", "unknown"
	case reflect.Slice, reflect.Array:
		if value.Kind() == reflect.Slice && value.IsNil() {
			return []any(nil), "observed"
		}
		items := make([]any, value.Len())
		for index := 0; index < value.Len(); index++ {
			item, _ := safeReflectValue(value.Index(index), fieldName)
			items[index] = item
		}
		return items, "observed"
	case reflect.Map:
		if value.IsNil() {
			return map[string]any(nil), "observed"
		}
		result := make(map[string]any, value.Len())
		iter := value.MapRange()
		for iter.Next() {
			if iter.Key().Kind() != reflect.String {
				continue
			}
			key := iter.Key().String()
			if isSensitiveField(key) {
				result[key] = !isZeroValue(iter.Value())
				continue
			}
			item, _ := safeReflectValue(iter.Value(), key)
			result[key] = item
		}
		return result, "observed"
	default:
		return fmt.Sprintf("unsupported value (%s)", value.Kind()), "unknown"
	}
}

func isSensitiveField(name string) bool {
	name = strings.ToLower(name)
	for _, marker := range []string{
		"password", "secret", "signaturekey", "signingkey", "privatekey",
		"apikey", "accesskey", "sslkeypath", "keyfile", "authorization",
	} {
		if strings.Contains(name, marker) {
			return true
		}
	}
	if strings.Contains(name, "token") &&
		!strings.Contains(name, "duration") &&
		!strings.Contains(name, "disable") &&
		!strings.Contains(name, "cookie") {
		return true
	}
	return false
}

func isZeroValue(value reflect.Value) bool {
	if !value.IsValid() {
		return true
	}
	for value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface {
		if value.IsNil() {
			return true
		}
		value = value.Elem()
	}
	switch value.Kind() {
	case reflect.Map, reflect.Slice, reflect.Func, reflect.Chan:
		return value.IsNil()
	default:
		return value.IsZero()
	}
}

func assess(report *Report) {
	values := make(map[string]any, len(report.Inventory))
	for _, field := range report.Inventory {
		values[field.Path] = field.Value
	}
	production := isProductionProfile(report.Profile)

	if debug, ok := values["app.Debug"].(bool); ok && production {
		if debug {
			addFinding(report, "APP-002", "fail", "high", "high", "app.Debug", debug,
				"Debug mode is enabled for a production-like environment.",
				"Disable debug mode in production and review any debug output for sensitive details.")
		} else {
			addFinding(report, "APP-002", "pass", "high", "high", "app.Debug", debug,
				"Debug mode is disabled for this production-like configuration.",
				"Keep debug mode disabled in production.")
		}
	}
	checkInsecureException(report, values, "app.AllowInsecureProduction", "APP-003", production)

	if production {
		if host, ok := values["server.Config.Host"].(string); ok {
			if host == "0.0.0.0" || host == "::" || host == "" {
				addFinding(report, "HTTP-001", "review", "medium", "medium", "server.Config.Host", host,
					"The listener is configured on a wildcard or unspecified host.",
					"Confirm that public exposure is intended and protected by the deployment network boundary.")
			} else {
				addFinding(report, "HTTP-001", "pass", "medium", "medium", "server.Config.Host", host,
					"The configured listener host is not a wildcard address.",
					"Verify the effective bind address and network exposure in the deployment environment.")
			}
		}

		certFileConfigured := configuredValue(values["server.Config.Listener.TLSCertFile"])
		certKeyFileConfigured := configuredValue(values["server.Config.Listener.TLSCertKeyFile"])
		if certFileConfigured != certKeyFileConfigured {
			addFinding(report, "HTTP-002", "fail", "high", "high", "server.Config.Listener", map[string]bool{
				"tls_certificate_configured": certFileConfigured,
				"tls_key_configured":         certKeyFileConfigured,
			}, "The listener has only one of its TLS certificate and key configured.",
				"Configure a matching certificate and key, or document TLS termination at a trusted proxy.")
		} else if !certFileConfigured {
			addFinding(report, "HTTP-002", "unknown", "high", "low", "server.Config.Listener", "TLS not configured in GOREST",
				"GOREST listener TLS is not configured; TLS termination outside the process cannot be verified.",
				"Confirm that a trusted reverse proxy or load balancer terminates TLS before traffic reaches this application.")
		} else {
			addFinding(report, "HTTP-002", "pass", "high", "medium", "server.Config.Listener", "TLS certificate and key configured",
				"TLS certificate and key paths are configured on the listener.",
				"Verify that the certificate files are readable, current, and correctly mounted in the deployment.")
			if minVersion, ok := numericValue(values["server.Config.Listener.TLSMinVersion"]); ok && minVersion < 0x0303 {
				addFinding(report, "HTTP-003", "fail", "high", "high", "server.Config.Listener.TLSMinVersion", minVersion,
					"The configured minimum TLS version is older than TLS 1.2.",
					"Use TLS 1.2 or newer.")
			} else {
				addFinding(report, "HTTP-003", "pass", "high", "medium", "server.Config.Listener.TLSMinVersion", values["server.Config.Listener.TLSMinVersion"],
					"The configured minimum TLS version is TLS 1.2 or newer.",
					"Keep the minimum TLS version aligned with current organizational policy.")
			}
		}

		if enabled, ok := values["server.Config.Listener.EnablePrintRoutes"].(bool); ok && enabled {
			addFinding(report, "HTTP-008", "review", "low", "high", "server.Config.Listener.EnablePrintRoutes", enabled,
				"Route printing is enabled for a production-like environment.",
				"Disable route printing in production unless startup logs are appropriately restricted.")
		}
	}

	for _, path := range []string{
		"server.Config.Core.ReadTimeout",
		"server.Config.Core.WriteTimeout",
	} {
		if duration, ok := values[path].(string); ok {
			parsed, err := time.ParseDuration(duration)
			if err == nil && parsed <= 0 {
				addFinding(report, "HTTP-004", "review", "medium", "high", path, duration,
					"The HTTP timeout is not bounded by a positive duration.",
					"Set a finite timeout appropriate to the application's request and response workload.")
			}
		}
	}
	if bodyLimit, ok := numericValue(values["server.Config.Core.BodyLimit"]); ok && bodyLimit == 0 {
		addFinding(report, "HTTP-004", "review", "medium", "high", "server.Config.Core.BodyLimit", bodyLimit,
			"The HTTP request body limit is zero or not bounded by the configured value.",
			"Set a finite request body limit based on the largest legitimate application payload.")
	}

	if production {
		if enabled, ok := values["database.Config.Disable"].(bool); ok && !enabled {
			checkDatabaseTLS(report, values)
			for _, field := range []string{"Migration", "Seeder", "QueryLogging"} {
				path := "database.Config." + field
				if value, ok := values[path].(bool); ok && value {
					addFinding(report, "DB-007", "review", "medium", "high", path, value,
						"Database migration, seeding, or query logging is enabled in a production-like configuration.",
						"Review whether this setting should be enabled for every production application instance.")
				}
			}
		}

		if enabled, ok := values["redis.Config.Disable"].(bool); ok && !enabled {
			if tlsEnabled, ok := values["redis.Config.TLS"].(bool); ok && !tlsEnabled {
				status := "fail"
				message := "Redis TLS is disabled for a production-like configuration."
				if insecureProductionApproved(values, "redis.Config.AllowInsecureProduction") {
					status = "review"
					message = "Redis TLS is disabled and the configuration explicitly approves this production exception."
				}
				addFinding(report, "REDIS-001", status, "high", "high", "redis.Config.TLS", tlsEnabled,
					message, "Enable Redis TLS or document and protect the approved network exception.")
			} else {
				addFinding(report, "REDIS-001", "pass", "high", "medium", "redis.Config.TLS", values["redis.Config.TLS"],
					"Redis TLS is enabled for this production-like configuration.",
					"Verify certificate validation and the Redis server's TLS configuration.")
			}
		}

		if enabled, ok := values["filter.Config.CookieConfig.DisableStoreTokenCookie"].(bool); ok && !enabled {
			for _, setting := range []struct {
				path string
				id   string
				msg  string
			}{
				{"filter.Config.CookieConfig.CookieSecure", "JWT-004", "JWT cookies do not require HTTPS transport."},
				{"filter.Config.CookieConfig.CookieHTTPOnly", "JWT-004", "JWT cookies are accessible to client-side scripts."},
			} {
				if value, exists := values[setting.path].(bool); exists && !value {
					addFinding(report, setting.id, "review", "high", "high", setting.path, value,
						setting.msg, "Enable the cookie attribute when using browser cookies, and review the application's transport and CSRF protections.")
				}
			}
		}

		if enabled, ok := values["monitoring.Config.DisableMonitoringEndpoints"].(bool); ok && !enabled {
			allowed := stringSliceValue(values["monitoring.Config.MonitoringAllowedIPs"])
			if len(allowed) == 0 {
				addFinding(report, "MON-001", "fail", "high", "high", "monitoring.Config.MonitoringAllowedIPs", allowed,
					"Built-in monitoring endpoints are enabled without a peer-IP allowlist.",
					"Disable monitoring endpoints or configure a narrow allowlist and restrict access at the network boundary.")
			} else {
				addFinding(report, "MON-001", "review", "high", "medium", "monitoring.Config.MonitoringAllowedIPs", allowed,
					"Monitoring endpoints are enabled and have an IP allowlist, but this report cannot verify the deployment's proxy topology.",
					"Confirm the list matches the immediate trusted peer addresses and assess legacy diagnostic endpoints separately.")
			}
		}
	}
}

func checkDatabaseTLS(report *Report, values map[string]any) {
	driver := database.NormalizeDatabaseDriver(stringValue(values["database.Config.Driver"]))
	if driver == database.SQLITE {
		addFinding(report, "DB-001", "not_applicable", "high", "high", "database.Config.Driver", driver,
			"Network database TLS checks do not apply to SQLite.",
			"Review the database driver and deployment topology.")
		return
	}

	tlsMode := strings.ToLower(strings.TrimSpace(stringValue(values["database.Config.TLS"])))
	status, confidence, message, recommendation := databaseTLSAssessment(driver, tlsMode, values)
	if status == "fail" {
		if insecureProductionApproved(values, "database.Config.AllowInsecureProduction") {
			status = "review"
			message += " The configuration explicitly approves this production exception."
		}
	}

	addFinding(report, "DB-001", status, "high", confidence, "database.Config.TLS", tlsMode,
		message, recommendation)
	if trusted, _ := values["database.Config.TrustServerCertificate"].(bool); trusted &&
		driver == database.SQLSERVER {
		addFinding(report, "DB-002", "fail", "high", "high", "database.Config.TrustServerCertificate", true,
			"SQL Server is configured to trust the server certificate without normal verification.",
			"Disable TrustServerCertificate and configure a certificate chain trusted by the application.")
	}
}

func databaseTLSAssessment(driver, tlsMode string, values map[string]any) (status, confidence, message, recommendation string) {
	const verifyRecommendation = "Configure driver-appropriate TLS with server certificate verification, or document and protect the approved network exception."

	switch driver {
	case database.MYSQL:
		switch tlsMode {
		case "", database.TLSDisabled, database.TLSFalse:
			return "fail", "high", "MySQL TLS is disabled.", verifyRecommendation
		case "skip-verify", "preferred":
			return "review", "high", "MySQL TLS mode may disable certificate verification or fall back to plaintext.", verifyRecommendation
		case database.TLSTrue:
			return "pass", "medium", "MySQL TLS is configured to use system roots and verify the server hostname.", "Verify the negotiated connection and any custom CA or named TLS profile."
		default:
			return "unknown", "low", "The MySQL TLS mode is not one of the directly assessable built-in modes.", "Verify the named or custom MySQL TLS profile and confirm certificate verification."
		}
	case database.POSTGRES:
		switch tlsMode {
		case "", database.TLSDisabled:
			return "fail", "high", "PostgreSQL TLS is disabled.", verifyRecommendation
		case "allow", "prefer":
			return "fail", "high", "PostgreSQL TLS may fall back to plaintext.", verifyRecommendation
		case "require", "verify-ca":
			return "review", "high", "PostgreSQL encrypts traffic but does not verify the server hostname in this mode.", "Prefer verify-full and configure the trusted CA as appropriate."
		case database.TLSVerifyFull:
			return "pass", "medium", "PostgreSQL is configured to verify the server certificate and hostname.", "Verify the trusted CA and connection behavior in the deployment."
		default:
			return "unknown", "low", "The PostgreSQL TLS mode is not recognized by this assessment.", verifyRecommendation
		}
	case database.SQLSERVER:
		if tlsMode == "" || tlsMode == database.TLSDisabled || tlsMode == database.TLSFalse || tlsMode == "optional" {
			return "fail", "high", "SQL Server encryption is disabled or optional.", "Use mandatory or strict TLS and keep TrustServerCertificate false."
		}
		if trusted, _ := values["database.Config.TrustServerCertificate"].(bool); trusted {
			return "fail", "high", "SQL Server certificate verification is disabled by TrustServerCertificate.", "Set TrustServerCertificate to false and use a certificate trusted by the application."
		}
		if tlsMode == "mandatory" || tlsMode == "strict" {
			return "pass", "medium", "SQL Server TLS is mandatory and TrustServerCertificate is false.", "Verify the server certificate chain and hostname in the deployment."
		}
		return "unknown", "low", "The SQL Server TLS mode is not recognized by this assessment.", "Use mandatory or strict TLS and keep TrustServerCertificate false."
	case database.ORACLE:
		if tlsMode != "enable" || stringValue(values["database.Config.SSLCAPath"]) == "" {
			return "fail", "high", "Oracle TCPS is not configured with the required wallet path.", "Set TLS to enable and SSLCAPath to a valid Oracle wallet directory."
		}
		return "pass", "medium", "Oracle TCPS and a wallet path are configured.", "Verify that the wallet is valid and mounted securely."
	case database.MONGO, database.SCYLLA:
		if (driver == database.MONGO && tlsMode != database.TLSTrue) ||
			(driver == database.SCYLLA && tlsMode != "enable") {
			return "fail", "high", driver + " TLS is disabled.", verifyRecommendation
		}
		return "pass", "medium", driver + " TLS is configured.", "Verify certificate and hostname validation in the deployment."
	default:
		return "unknown", "low", "The selected database driver is not covered by the TLS assessment.", verifyRecommendation
	}
}

func checkInsecureException(report *Report, values map[string]any, path, id string, production bool) {
	if !production {
		return
	}
	if enabled, ok := values[path].(bool); ok && enabled {
		addFinding(report, id, "review", "high", "high", path, enabled,
			"The application explicitly allows insecure transport in a production-like environment.",
			"Review the affected connections and prefer verified TLS.")
	}
}

func insecureProductionApproved(values map[string]any, componentPath string) bool {
	componentApproved, _ := values[componentPath].(bool)
	globalApproved, _ := values["app.AllowInsecureProduction"].(bool)
	return componentApproved || globalApproved
}

func addFinding(report *Report, id, status, severity, confidence, path string, observed any, message, recommendation string) {
	report.Findings = append(report.Findings, Finding{
		ID:             id,
		Status:         status,
		Severity:       severity,
		Confidence:     confidence,
		FieldPath:      path,
		EvidenceSource: "effective_runtime_config",
		ObservedValue:  observed,
		Message:        message,
		Recommendation: recommendation,
	})
}

func summarize(report *Report) {
	for _, finding := range report.Findings {
		if finding.Status == "fail" || finding.Status == "review" {
			switch finding.Severity {
			case "critical":
				report.Summary.Critical++
			case "high":
				report.Summary.High++
			case "medium":
				report.Summary.Medium++
			case "low":
				report.Summary.Low++
			case "informational":
				report.Summary.Informational++
			}
		}
		switch finding.Status {
		case "pass":
			report.Summary.Pass++
		case "review":
			report.Summary.Review++
		case "fail":
			report.Summary.Fail++
		case "unknown":
			report.Summary.Unknown++
		case "not_applicable":
			report.Summary.NotApplicable++
		}
	}
}

func isProductionProfile(profile string) bool {
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case "production", "prod", "staging", "stage", "uat":
		return true
	default:
		return false
	}
}

func stringValue(value any) string {
	result, _ := value.(string)
	return result
}

func configuredValue(value any) bool {
	if state, ok := value.(map[string]bool); ok {
		return state["configured"]
	}
	text, ok := value.(string)
	return ok && text != ""
}

func numericValue(value any) (uint64, bool) {
	switch number := value.(type) {
	case uint64:
		return number, true
	case uint:
		return uint64(number), true
	case int64:
		return uint64(number), number >= 0
	case int:
		return uint64(number), number >= 0
	case uint32:
		return uint64(number), true
	case int32:
		return uint64(number), number >= 0
	default:
		return 0, false
	}
}

func stringSliceValue(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if ok {
			result = append(result, text)
		}
	}
	return result
}
