package database

import "time"

type executionType string

// PasswordEncryption type for validation.
type PasswordEncryption string

const (
	MYSQL                             = "mysql"
	MARIADB                           = "mariadb"
	POSTGRES                          = "postgres"
	SQLSERVER                         = "sqlserver"
	SQLITE                            = "sqlite"
	MONGO                             = "mongo"
	SCYLLA                            = "scylla"
	ORACLE                            = "oracle"
	MYSQL_PORT                        = "3306"
	POSTGRES_PORT                     = "5432"
	SQLSERVER_PORT                    = "1433"
	MONGO_PORT                        = "27017"
	SCYLLA_PORT                       = "9042"
	ORACLE_PORT                       = "1521"
	typeMigration       executionType = "migration"
	typeSeeder          executionType = "seeder"
	migrationTableName                = "migration_history"
	migrationColumnName               = "migration_name"
	seederTableName                   = "seeder_history"
	seederColumnName                  = "seeder_name"

	DefaultDatabaseDriver         = SQLITE
	DefaultDatabaseHost           = "127.0.0.1"
	DefaultDatabaseLibDir         = ""
	DefaultDatabaseName           = "gorest"
	DefaultDatabasePort           = ""
	TLSDisabled                   = "disable"
	TLSEnabled                    = "enable"
	TLSOptional                   = "optional"
	TLSMandatory                  = "mandatory"
	TLSTrue                       = "true"
	TLSFalse                      = "false"
	TLSVerifyFull                 = "verify-full"
	TLSStrict                     = "strict"
	DefaultSSLCertPath            = ""
	DefaultSSLKeyPath             = ""
	DefaultSSLCAPath              = ""
	DefaultCharset                = "utf8mb4"
	DefaultDatabaseDirectory      = "./database"
	DefaultMigrationDirectory     = "./migrations"
	DefaultSeederDirectory        = "./seeders"
	DefaultTimeout                = 5 * time.Second
	DefaultConnectionMaxLifetime  = 10 * time.Minute
	DefaultTrustServerCertificate = false
	DefaultQueryLogging           = false
	MigrationDisabled             = false
	MigrationEnabled              = true
	SeederDisabled                = false
	SeederEnabled                 = true
	DatabaseDisabled              = false
	DatabaseEnabled               = true
	DefaultMaxOpenConnections     = 50
	DefaultMaxIdleConnections     = 5
)

// ===============================================================================================================
// supportedDatabaseDrivers maps recognized database driver names to their supported status boolean.
// ===============================================================================================================.
var supportedDatabaseDrivers = map[string]bool{
	MYSQL:     true,
	MARIADB:   true,
	POSTGRES:  true,
	SQLSERVER: true,
	SQLITE:    true,
	MONGO:     true,
	SCYLLA:    true,
	ORACLE:    true,
}
