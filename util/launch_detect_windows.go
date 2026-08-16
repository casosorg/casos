// Copyright 2026 The OpenAgent Authors. All Rights Reserved.
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

//go:build windows

package util

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// IsDoubleClicked reports whether the process was launched by double-clicking
// its executable in Explorer, rather than from a terminal or another program.
func IsDoubleClicked() bool {
	exe, err := os.Executable()
	if err == nil && isGoBuildTemp(exe) {
		return false
	}

	parentName, err := getParentProcessName()
	if err != nil {
		return false
	}

	return strings.EqualFold(parentName, "explorer.exe")
}

// ShowStartupNotice opens a separate console containing only the successful
// startup status and URL, so Kubernetes logs cannot immediately bury it.
func ShowStartupNotice(url string) error {
	systemDirectory, err := windows.GetSystemDirectory()
	if err != nil {
		return err
	}
	cmdPath := filepath.Join(systemDirectory, "cmd.exe")
	commandLine, err := windows.UTF16PtrFromString(windows.ComposeCommandLine([]string{
		cmdPath, "/d", "/q", "/c", startupNoticeCommand(url),
	}))
	if err != nil {
		return err
	}

	startupInfo := &windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfo{}))}
	var processInfo windows.ProcessInformation
	if err := windows.CreateProcess(
		nil,
		commandLine,
		nil,
		nil,
		false,
		windows.CREATE_NEW_CONSOLE,
		nil,
		nil,
		startupInfo,
		&processInfo,
	); err != nil {
		return err
	}
	_ = windows.CloseHandle(processInfo.Thread)
	_ = windows.CloseHandle(processInfo.Process)
	return nil
}

func startupNoticeCommand(url string) string {
	return strings.Join([]string{
		"title CasOS Started",
		"cls",
		"echo.",
		"echo CasOS started successfully.",
		"echo.",
		"echo Open " + url + " in your browser.",
		"echo.",
		"echo Keep the original CasOS window open while using CasOS.",
		"echo Press any key to close this message window.",
		"pause >nul",
	}, " & ")
}

func isGoBuildTemp(exe string) bool {
	for _, part := range strings.Split(strings.ToLower(strings.ReplaceAll(exe, `\`, "/")), "/") {
		const prefix = "go-build"
		if strings.HasPrefix(part, prefix) {
			if _, err := strconv.ParseUint(strings.TrimPrefix(part, prefix), 10, 64); err == nil {
				return true
			}
		}
	}

	return false
}

func getParentProcessName() (string, error) {
	currentPID := windows.GetCurrentProcessId()

	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(snapshot)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))

	if err := windows.Process32First(snapshot, &entry); err != nil {
		return "", err
	}

	// Collect all processes in one pass so we can look up parent by PID.
	processes := make(map[uint32]windows.ProcessEntry32)
	for {
		processes[entry.ProcessID] = entry
		if err := windows.Process32Next(snapshot, &entry); err != nil {
			if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
				break
			}
			return "", err
		}
	}

	current, ok := processes[currentPID]
	if !ok {
		return "", fmt.Errorf("current process %d not found in snapshot", currentPID)
	}

	parent, ok := processes[current.ParentProcessID]
	if !ok {
		return "", fmt.Errorf("parent process %d not found in snapshot", current.ParentProcessID)
	}

	return windows.UTF16ToString(parent.ExeFile[:]), nil
}
