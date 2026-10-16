package database

import "testing"

func TestRunDoesNotRepeatDirectoryDefaults(t *testing.T) {
	config := Config{
		Driver:            MYSQL,
		DatabaseDirectory: "testdata/db",
	}

	first := Run(config)
	second := Run(first)

	if first.MaxOpenConnections != ConfigDefault.MaxOpenConnections {
		t.Errorf("MaxOpenConnections = %d, want ConfigDefault value %d", first.MaxOpenConnections, ConfigDefault.MaxOpenConnections)
	}
	if first.MaxIdleConnections != ConfigDefault.MaxIdleConnections {
		t.Errorf("MaxIdleConnections = %d, want ConfigDefault value %d", first.MaxIdleConnections, ConfigDefault.MaxIdleConnections)
	}
	if first.ConnectionMaxLifetime != ConfigDefault.ConnectionMaxLifetime {
		t.Errorf("ConnectionMaxLifetime = %s, want ConfigDefault value %s", first.ConnectionMaxLifetime, ConfigDefault.ConnectionMaxLifetime)
	}
	if second.MigrationDirectory != first.MigrationDirectory {
		t.Errorf("MigrationDirectory changed from %q to %q", first.MigrationDirectory, second.MigrationDirectory)
	}
	if second.SeederDirectory != first.SeederDirectory {
		t.Errorf("SeederDirectory changed from %q to %q", first.SeederDirectory, second.SeederDirectory)
	}
}
