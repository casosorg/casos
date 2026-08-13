package object

import (
	"path/filepath"
	"testing"
)

func TestSQLiteAdapterConfiguration(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "nested", "casos.db")
	adapter := NewAdapter("sqlite", databasePath, "")
	t.Cleanup(func() {
		adapter.close()
	})

	if err := adapter.Engine.Ping(); err != nil {
		t.Fatalf("ping SQLite database: %v", err)
	}
	if got := adapter.Engine.DB().Stats().MaxOpenConnections; got != 1 {
		t.Fatalf("MaxOpenConnections = %d, want 1", got)
	}

	var journalMode string
	if _, err := adapter.Engine.SQL("PRAGMA journal_mode").Get(&journalMode); err != nil {
		t.Fatalf("read journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Fatalf("journal_mode = %q, want wal", journalMode)
	}

	var busyTimeout int
	if _, err := adapter.Engine.SQL("PRAGMA busy_timeout").Get(&busyTimeout); err != nil {
		t.Fatalf("read busy_timeout: %v", err)
	}
	if busyTimeout != 5000 {
		t.Fatalf("busy_timeout = %d, want 5000", busyTimeout)
	}

	adapter.createTable()
	exists, err := adapter.Engine.IsTableExist(new(Site))
	if err != nil {
		t.Fatalf("check Site table: %v", err)
	}
	if !exists {
		t.Fatal("Site table was not created")
	}
}

func TestSQLite3DriverAlias(t *testing.T) {
	adapter := NewAdapter("sqlite3", filepath.Join(t.TempDir(), "casos.db"), "")
	t.Cleanup(func() {
		adapter.close()
	})
	if adapter.driverName != "sqlite" {
		t.Fatalf("driverName = %q, want sqlite", adapter.driverName)
	}
	if err := adapter.Engine.Ping(); err != nil {
		t.Fatalf("ping SQLite database: %v", err)
	}
}
