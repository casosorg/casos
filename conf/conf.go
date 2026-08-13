// Copyright 2023 The casbin Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package conf

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/beego/beego"
	"github.com/beego/beego/logs"
)

const DefaultDataDir = "./data"

func init() {
	// this array contains the beego configuration items that may be modified via env
	presetConfigItems := []string{"httpport", "appname"}
	for _, key := range presetConfigItems {
		if value, ok := os.LookupEnv(key); ok {
			err := beego.AppConfig.Set(key, value)
			if err != nil {
				panic(err)
			}
		}
	}
}

func GetConfigString(key string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}

	// the only place in the codebase that reads beego's app.conf directly
	res := beego.AppConfig.String(key)
	if res == "" {
		if key == "staticBaseUrl" {
			res = "https://cdn.casbin.org"
		} else if key == "logConfig" {
			res = fmt.Sprintf("{\"filename\": \"logs/%s.log\", \"maxdays\":99999, \"perm\":\"0770\"}", GetConfigString("appname"))
		}
	}

	return res
}

// GetConfigStringDefault returns the value for key, falling back to defaultValue
// when the key is unset or blank.
func GetConfigStringDefault(key string, defaultValue string) string {
	value := strings.TrimSpace(GetConfigString(key))
	if value == "" {
		return defaultValue
	}
	return value
}

func GetConfigBool(key string) bool {
	return GetConfigBoolDefault(key, false)
}

// GetConfigBoolDefault parses key as a boolean, falling back to defaultValue when
// the key is unset or cannot be parsed. Accepts the strconv forms plus
// yes/y/on and no/n/off, case-insensitively.
func GetConfigBoolDefault(key string, defaultValue bool) bool {
	value := strings.TrimSpace(GetConfigString(key))
	if value == "" {
		return defaultValue
	}

	switch strings.ToLower(value) {
	case "yes", "y", "on":
		return true
	case "no", "n", "off":
		return false
	}

	res, err := strconv.ParseBool(value)
	if err != nil {
		logs.Warning("invalid boolean config %s=%q, using default %t", key, value, defaultValue)
		return defaultValue
	}
	return res
}

func GetConfigInt(key string) int {
	value := GetConfigString(key)
	num, err := strconv.Atoi(value)
	if err != nil {
		panic(err)
	}
	return num
}

// GetConfigIntDefault parses key as an int, falling back to defaultValue when the
// key is unset or cannot be parsed.
func GetConfigIntDefault(key string, defaultValue int) int {
	value := strings.TrimSpace(GetConfigString(key))
	if value == "" {
		return defaultValue
	}

	res, err := strconv.Atoi(value)
	if err != nil {
		logs.Warning("invalid int config %s=%q, using default %d", key, value, defaultValue)
		return defaultValue
	}
	return res
}

func GetConfigInt64(key string) (int64, error) {
	value := GetConfigString(key)
	num, err := strconv.ParseInt(value, 10, 64)
	return num, err
}

func GetConfigDataSourceName() string {
	dataSourceName := GetConfigString("dataSourceName")

	runningInDocker := os.Getenv("RUNNING_IN_DOCKER")
	if runningInDocker == "true" {
		// https://stackoverflow.com/questions/48546124/what-is-linux-equivalent-of-host-docker-internal
		if runtime.GOOS == "linux" {
			dataSourceName = strings.ReplaceAll(dataSourceName, "localhost", "172.17.0.1")
		} else {
			dataSourceName = strings.ReplaceAll(dataSourceName, "localhost", "host.docker.internal")
		}
	}

	return dataSourceName
}

func GetDatabaseDriverName() string {
	driverName := strings.ToLower(GetConfigStringDefault("driverName", "sqlite"))
	if driverName == "sqlite3" {
		return "sqlite"
	}
	return driverName
}

func GetDatabaseDataSourceName() string {
	return resolveDatabaseDataSourceName(
		GetDatabaseDriverName(),
		GetConfigDataSourceName(),
		GetDataDir(),
	)
}

func GetDataDir() string {
	dataDir := GetConfigStringDefault("dataDir", DefaultDataDir)
	absolutePath, err := filepath.Abs(dataDir)
	if err != nil {
		return dataDir
	}
	return absolutePath
}

func resolveDatabaseDataSourceName(driverName, dataSourceName, dataDir string) string {
	if driverName == "sqlite" && strings.TrimSpace(dataSourceName) == "" {
		return filepath.Join(dataDir, "casos.db")
	}
	return dataSourceName
}

func GetLanguage(language string) string {
	if language == "" || language == "*" {
		return "en"
	}

	if len(language) != 2 || language == "nu" {
		return "en"
	} else {
		return language
	}
}

func IsDemoMode() bool {
	return GetConfigBoolDefault("isDemoMode", false)
}

func GetConfigBatchSize() int {
	res, err := strconv.Atoi(GetConfigString("batchSize"))
	if err != nil {
		res = 100
	}
	return res
}

func GetConfigRealDataSourceName(driverName string) string {
	dataSourceName := GetDatabaseDataSourceName()
	if driverName == "mysql" {
		dataSourceName += GetConfigStringDefault("dbName", "casos")
	}
	return dataSourceName
}
