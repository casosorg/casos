package conf

import (
	"path/filepath"
	"testing"
)

func TestResolveDatabaseDataSourceName(t *testing.T) {
	dataDir := t.TempDir()

	tests := []struct {
		name           string
		driverName     string
		dataSourceName string
		want           string
	}{
		{
			name:       "default SQLite path",
			driverName: "sqlite",
			want:       filepath.Join(dataDir, "casos.db"),
		},
		{
			name:           "configured SQLite path",
			driverName:     "sqlite",
			dataSourceName: filepath.Join(dataDir, "custom.db"),
			want:           filepath.Join(dataDir, "custom.db"),
		},
		{
			name:           "MySQL DSN",
			driverName:     "mysql",
			dataSourceName: "root:secret@tcp(localhost:3306)/",
			want:           "root:secret@tcp(localhost:3306)/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveDatabaseDataSourceName(tt.driverName, tt.dataSourceName, dataDir)
			if got != tt.want {
				t.Fatalf("resolveDatabaseDataSourceName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDefaultSQLiteDataSourceName(t *testing.T) {
	want := filepath.Join(DefaultDataDir, "casos.db")
	if got := resolveDatabaseDataSourceName("sqlite", "", DefaultDataDir); got != want {
		t.Fatalf("resolveDatabaseDataSourceName() = %q, want %q", got, want)
	}
}

func TestGetDataDirReturnsAbsolutePath(t *testing.T) {
	t.Setenv("dataDir", "./data")
	got := GetDataDir()
	if !filepath.IsAbs(got) {
		t.Fatalf("GetDataDir() = %q, want an absolute path", got)
	}
	if filepath.Base(got) != "data" {
		t.Fatalf("GetDataDir() = %q, want data directory", got)
	}
}

func TestGetDatabaseDriverNameNormalizesSQLiteAlias(t *testing.T) {
	t.Setenv("driverName", "sqlite3")
	if got := GetDatabaseDriverName(); got != "sqlite" {
		t.Fatalf("GetDatabaseDriverName() = %q, want sqlite", got)
	}
}
