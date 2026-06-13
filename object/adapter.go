package object

import (
	"fmt"
	"strings"
	"sync"

	"github.com/beego/beego"
	_ "github.com/go-sql-driver/mysql"
	"github.com/sirupsen/logrus"
	"xorm.io/xorm"
)

var (
	once   sync.Once
	engine *xorm.Engine
)

// InitDB reads database config from app.conf and initializes the xorm engine.
// Follows the same pattern as Casdoor's object/adapter.go.
func InitDB() error {
	var initErr error
	once.Do(func() {
		driver := beego.AppConfig.String("driverName")
		if driver == "" {
			driver = "mysql"
		}
		dsn := beego.AppConfig.String("dataSourceName")
		if dsn == "" {
			initErr = fmt.Errorf("dataSourceName not set in app.conf")
			return
		}
		dbName := beego.AppConfig.String("dbName")
		if dbName == "" {
			dbName = "casos"
		}
		dsn = injectDBName(dsn, dbName)

		var err error
		engine, err = xorm.NewEngine(driver, dsn)
		if err != nil {
			initErr = fmt.Errorf("xorm.NewEngine: %w", err)
			return
		}
		engine.SetMaxOpenConns(20)
		engine.SetMaxIdleConns(5)

		if err := engine.Ping(); err != nil {
			initErr = fmt.Errorf("db ping: %w", err)
			return
		}
		logrus.Infof("database connected: driver=%s", driver)
	})
	return initErr
}

// GetEngine returns the global xorm engine. InitDB must be called first.
func GetEngine() *xorm.Engine {
	return engine
}

// injectDBName inserts dbName into a MySQL DSN that has no database component.
// Mirrors the same helper in server/server.go — both honour the dbName field.
func injectDBName(dsn, dbName string) string {
	idx := strings.LastIndex(dsn, "/")
	if idx < 0 {
		return dsn + dbName
	}
	base := dsn[:idx+1]
	rest := dsn[idx+1:]
	if q := strings.Index(rest, "?"); q >= 0 {
		return base + dbName + rest[q:]
	}
	return base + dbName
}
