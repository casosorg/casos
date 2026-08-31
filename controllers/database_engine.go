package controllers

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// What casos knows how to run as a database.
//
// One engine describes everything the rest of the code needs: which image to
// run, how the container is told its credentials, and the four shell commands
// that make a database more than a container — a console to type into, a dump,
// a restore, and the URI an application connects with.
//
// The credentials live in a Secret and reach the container as environment
// variables, which is also why every command below refers to them by variable
// name rather than by value: nothing here ever puts a password on a command
// line the cluster would record.

const (
	databaseManagedByLabel = "app.kubernetes.io/managed-by"
	databaseManagedByValue = "casos"
	databaseInstanceLabel  = "app.kubernetes.io/instance"
	databaseEngineLabel    = "casos.io/database-engine"
	databaseVersionLabel   = "casos.io/database-version"

	databaseDataVolume    = "data"
	databaseBackupVolume  = "backups"
	databaseBackupPath    = "/backups"
	databaseContainerName = "database"
)

type databaseEngine struct {
	Key      string
	Label    string
	Image    string
	Versions []string
	Port     int32
	DataPath string
	// The user an application signs in as. Some engines only ever have one.
	DefaultUser string
	// FixedUser is set for engines whose superuser cannot be renamed.
	FixedUser bool
	// SupportsDatabaseName is false for engines with no database-name concept.
	SupportsDatabaseName bool
	// Env wires the Secret into the container. Values are read from the Secret
	// rather than written into the pod spec.
	Env func(secretName string) []corev1.EnvVar
	// Run says what the container executes once the tuned parameters have been
	// turned into flags. Every image wants them somewhere different — after the
	// entrypoint, after the server binary, or inside the shell line that starts
	// it — so each engine composes its own. Returning nothing leaves the image
	// to run itself.
	Run func(flags []string) (command []string, args []string)
	// Flag renders one tuned parameter the way this engine's server reads it.
	Flag func(key, value string) []string
	// Params is the short list of settings worth tuning from a form. It is
	// deliberately not every setting the engine has: the rest belong in a
	// config file, and a database nobody can start is worse than one that is
	// not tuned.
	Params []databaseParam
	// BackupSuffix names the file a dump produces, so a reader can tell what
	// they are downloading.
	BackupSuffix string
	// Dump writes a backup to path; Restore reads one back.
	Dump    func(path string) string
	Restore func(path string) string
	// Console is the client shell an operator types into.
	Console func() []string
	// RestoreNeedsRestart is true where a restore only takes effect once the
	// engine has re-read its data directory.
	RestoreNeedsRestart bool
	// URI is what an application puts in its configuration.
	URI func(user, password, database, host string, port int32) string
}

// databaseParam describes one tunable setting: what it is called, what shape a
// value has to be, and what the engine does when nobody sets it.
type databaseParam struct {
	Key     string   `json:"key"`
	Label   string   `json:"label"`
	Kind    string   `json:"kind"` // int | size | enum
	Default string   `json:"default"`
	Options []string `json:"options,omitempty"`
	Hint    string   `json:"hint,omitempty"`
}

func secretEnv(name, secretName, key string) corev1.EnvVar {
	return corev1.EnvVar{
		Name: name,
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
				Key:                  key,
			},
		},
	}
}

var databaseEngines = map[string]databaseEngine{
	"postgresql": {
		Key:                  "postgresql",
		Label:                "PostgreSQL",
		Image:                "postgres",
		Versions:             []string{"17-alpine", "16-alpine", "15-alpine"},
		Port:                 5432,
		DataPath:             "/var/lib/postgresql/data",
		DefaultUser:          "postgres",
		SupportsDatabaseName: true,
		Env: func(secretName string) []corev1.EnvVar {
			return []corev1.EnvVar{
				secretEnv("POSTGRES_USER", secretName, "username"),
				secretEnv("POSTGRES_PASSWORD", secretName, "password"),
				secretEnv("POSTGRES_DB", secretName, "database"),
				secretEnv("PGPASSWORD", secretName, "password"),
				// initdb refuses to run in a non-empty directory and a freshly
				// bound claim arrives carrying lost+found, so the data lives one
				// level down.
				{Name: "PGDATA", Value: "/var/lib/postgresql/data/pgdata"},
			}
		},
		Flag: func(key, value string) []string { return []string{"-c", key + "=" + value} },
		Run: func(flags []string) ([]string, []string) {
			if len(flags) == 0 {
				return nil, nil
			}
			return nil, append([]string{"postgres"}, flags...)
		},
		Params: []databaseParam{
			{Key: "max_connections", Label: "Maximum connections", Kind: "int", Default: "100"},
			{Key: "shared_buffers", Label: "Shared buffers", Kind: "size", Default: "128MB", Hint: "Roughly a quarter of the memory limit."},
			{Key: "work_mem", Label: "Work memory per sort", Kind: "size", Default: "4MB"},
			{Key: "effective_cache_size", Label: "Effective cache size", Kind: "size", Default: "4GB", Hint: "What the planner assumes the OS is caching."},
			{Key: "log_min_duration_statement", Label: "Log statements slower than (ms)", Kind: "int", Default: "-1", Hint: "-1 turns slow-statement logging off."},
		},
		BackupSuffix: ".sql.gz",
		Dump: func(path string) string {
			return fmt.Sprintf("pg_dump -U \"$POSTGRES_USER\" -d \"$POSTGRES_DB\" | gzip > %q", path)
		},
		Restore: func(path string) string {
			return fmt.Sprintf("gunzip -c %q | psql -U \"$POSTGRES_USER\" -d \"$POSTGRES_DB\"", path)
		},
		Console: func() []string {
			return []string{"sh", "-c", "exec psql -U \"$POSTGRES_USER\" -d \"$POSTGRES_DB\""}
		},
		URI: func(user, password, database, host string, port int32) string {
			return fmt.Sprintf("postgresql://%s:%s@%s:%d/%s", user, password, host, port, database)
		},
	},

	"mysql": {
		Key:                  "mysql",
		Label:                "MySQL",
		Image:                "mysql",
		Versions:             []string{"8.4", "8.0"},
		Port:                 3306,
		DataPath:             "/var/lib/mysql",
		DefaultUser:          "root",
		FixedUser:            true,
		SupportsDatabaseName: true,
		Env: func(secretName string) []corev1.EnvVar {
			return []corev1.EnvVar{
				secretEnv("MYSQL_ROOT_PASSWORD", secretName, "password"),
				secretEnv("MYSQL_DATABASE", secretName, "database"),
			}
		},
		Flag: func(key, value string) []string { return []string{"--" + key + "=" + value} },
		Run: func(flags []string) ([]string, []string) {
			if len(flags) == 0 {
				return nil, nil
			}
			return nil, append([]string{"mysqld"}, flags...)
		},
		Params: []databaseParam{
			{Key: "max_connections", Label: "Maximum connections", Kind: "int", Default: "151"},
			{Key: "innodb_buffer_pool_size", Label: "InnoDB buffer pool", Kind: "size", Default: "128M", Hint: "The single setting that matters most for read speed."},
			{Key: "max_allowed_packet", Label: "Maximum packet size", Kind: "size", Default: "64M"},
			{Key: "slow_query_log", Label: "Slow query log", Kind: "enum", Default: "OFF", Options: []string{"OFF", "ON"}},
			{Key: "long_query_time", Label: "Slow query threshold (s)", Kind: "int", Default: "10"},
		},
		BackupSuffix: ".sql.gz",
		Dump: func(path string) string {
			return fmt.Sprintf("mysqldump -u root -p\"$MYSQL_ROOT_PASSWORD\" --databases \"$MYSQL_DATABASE\" | gzip > %q", path)
		},
		Restore: func(path string) string {
			return fmt.Sprintf("gunzip -c %q | mysql -u root -p\"$MYSQL_ROOT_PASSWORD\"", path)
		},
		Console: func() []string {
			return []string{"sh", "-c", "exec mysql -u root -p\"$MYSQL_ROOT_PASSWORD\" \"$MYSQL_DATABASE\""}
		},
		URI: func(user, password, database, host string, port int32) string {
			return fmt.Sprintf("mysql://%s:%s@%s:%d/%s", user, password, host, port, database)
		},
	},

	"mongodb": {
		Key:                  "mongodb",
		Label:                "MongoDB",
		Image:                "mongo",
		Versions:             []string{"7", "6"},
		Port:                 27017,
		DataPath:             "/data/db",
		DefaultUser:          "root",
		SupportsDatabaseName: true,
		Env: func(secretName string) []corev1.EnvVar {
			return []corev1.EnvVar{
				secretEnv("MONGO_INITDB_ROOT_USERNAME", secretName, "username"),
				secretEnv("MONGO_INITDB_ROOT_PASSWORD", secretName, "password"),
				secretEnv("MONGO_INITDB_DATABASE", secretName, "database"),
			}
		},
		Flag: func(key, value string) []string { return []string{"--" + key + "=" + value} },
		Run: func(flags []string) ([]string, []string) {
			if len(flags) == 0 {
				return nil, nil
			}
			return nil, append([]string{"mongod"}, flags...)
		},
		Params: []databaseParam{
			{Key: "wiredTigerCacheSizeGB", Label: "WiredTiger cache (GB)", Kind: "int", Default: "0", Hint: "0 lets MongoDB size the cache from the memory it can see."},
			{Key: "slowms", Label: "Slow operation threshold (ms)", Kind: "int", Default: "100"},
			{Key: "profile", Label: "Profiling level", Kind: "enum", Default: "0", Options: []string{"0", "1", "2"}, Hint: "0 off, 1 slow operations, 2 everything."},
		},
		BackupSuffix: ".archive.gz",
		Dump: func(path string) string {
			return fmt.Sprintf("mongodump --username \"$MONGO_INITDB_ROOT_USERNAME\" --password \"$MONGO_INITDB_ROOT_PASSWORD\" --authenticationDatabase admin --archive=%q --gzip", path)
		},
		Restore: func(path string) string {
			return fmt.Sprintf("mongorestore --username \"$MONGO_INITDB_ROOT_USERNAME\" --password \"$MONGO_INITDB_ROOT_PASSWORD\" --authenticationDatabase admin --archive=%q --gzip --drop", path)
		},
		Console: func() []string {
			return []string{"sh", "-c", "exec mongosh -u \"$MONGO_INITDB_ROOT_USERNAME\" -p \"$MONGO_INITDB_ROOT_PASSWORD\" --authenticationDatabase admin"}
		},
		URI: func(user, password, database, host string, port int32) string {
			return fmt.Sprintf("mongodb://%s:%s@%s:%d/%s?authSource=admin", user, password, host, port, database)
		},
	},

	"redis": {
		Key:         "redis",
		Label:       "Redis",
		Image:       "redis",
		Versions:    []string{"7-alpine", "6-alpine"},
		Port:        6379,
		DataPath:    "/data",
		DefaultUser: "default",
		FixedUser:   true,
		Env: func(secretName string) []corev1.EnvVar {
			return []corev1.EnvVar{secretEnv("REDIS_PASSWORD", secretName, "password")}
		},
		Flag: func(key, value string) []string { return []string{"--" + key, value} },
		// Redis takes its settings on the server's own command line, and the
		// command line is a shell line here because the password comes from the
		// environment. Values reach it only after passing their parameter's
		// shape check, so there is nothing in them a shell could act on.
		Run: func(flags []string) ([]string, []string) {
			line := "exec redis-server --requirepass \"$REDIS_PASSWORD\" --appendonly yes"
			if len(flags) > 0 {
				line += " " + strings.Join(flags, " ")
			}
			return []string{"sh", "-c", line}, nil
		},
		Params: []databaseParam{
			{Key: "maxmemory", Label: "Memory limit", Kind: "size", Default: "0", Hint: "0 means no limit of its own."},
			{
				Key: "maxmemory-policy", Label: "What to drop when full", Kind: "enum", Default: "noeviction",
				Options: []string{"noeviction", "allkeys-lru", "allkeys-lfu", "volatile-lru", "volatile-lfu", "volatile-ttl", "volatile-random", "allkeys-random"},
			},
			{Key: "timeout", Label: "Idle client timeout (s)", Kind: "int", Default: "0"},
		},
		BackupSuffix: ".rdb.gz",
		Dump: func(path string) string {
			return fmt.Sprintf("redis-cli -a \"$REDIS_PASSWORD\" --no-auth-warning SAVE && gzip -c /data/dump.rdb > %q", path)
		},
		// Redis only reads its dump file at startup, so the restore drops the
		// file in place and the pod is restarted around it.
		Restore: func(path string) string {
			return fmt.Sprintf("gunzip -c %q > /data/dump.rdb", path)
		},
		RestoreNeedsRestart: true,
		Console: func() []string {
			return []string{"sh", "-c", "exec redis-cli -a \"$REDIS_PASSWORD\" --no-auth-warning"}
		},
		URI: func(user, password, database, host string, port int32) string {
			return fmt.Sprintf("redis://:%s@%s:%d", password, host, port)
		},
	},
}

// databaseEngineOrder keeps the catalogue in a stable, sensible order rather
// than in map order, which Go deliberately randomises.
var databaseEngineOrder = []string{"postgresql", "mysql", "mongodb", "redis"}

func engineByKey(key string) (databaseEngine, bool) {
	engine, ok := databaseEngines[key]
	return engine, ok
}

func engineVersionOrDefault(engine databaseEngine, version string) string {
	for _, candidate := range engine.Versions {
		if candidate == version {
			return version
		}
	}
	return engine.Versions[0]
}

// paramValuePattern is what a tuned value may look like: a number, or a number
// with a unit, or a bare word. Nothing in it can mean anything to a shell,
// which is what lets Redis take its settings on a shell command line.
var paramValuePattern = regexp.MustCompile(`^-?[0-9A-Za-z_.-]{1,32}$`)

func (p databaseParam) validate(value string) error {
	if !paramValuePattern.MatchString(value) {
		return fmt.Errorf("%s: %q is not a value this setting can take", p.Key, value)
	}
	switch p.Kind {
	case "int":
		if _, err := strconv.Atoi(value); err != nil {
			return fmt.Errorf("%s has to be a whole number", p.Key)
		}
	case "size":
		if !regexp.MustCompile(`^[0-9]+[A-Za-z]{0,2}$`).MatchString(value) {
			return fmt.Errorf("%s has to be a size like 256MB", p.Key)
		}
	case "enum":
		if !slices.Contains(p.Options, value) {
			return fmt.Errorf("%s has to be one of %s", p.Key, strings.Join(p.Options, ", "))
		}
	}
	return nil
}

// applyDatabaseParams renders the tuned parameters into the container. Values
// left at their default are not rendered at all, so a database nobody has tuned
// runs exactly the command it would have without this feature.
func applyDatabaseParams(container *corev1.Container, engine databaseEngine, values map[string]string) error {
	flags := []string{}
	for _, param := range engine.Params {
		value, ok := values[param.Key]
		if !ok || value == "" || value == param.Default {
			continue
		}
		if err := param.validate(value); err != nil {
			return err
		}
		flags = append(flags, engine.Flag(param.Key, value)...)
	}

	command, args := engine.Run(flags)
	container.Command = command
	container.Args = args
	return nil
}
