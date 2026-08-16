// Copyright 2026 The casbin Authors. All Rights Reserved.
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

package util

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	startupPollInterval   = 200 * time.Millisecond
	startupRequestTimeout = 5 * time.Second
	startupResponseLimit  = 64 << 10
	casOSPageMarker       = "<title>CasOS</title>"
)

// WaitForCasOS waits until the HTTP endpoint serves the CasOS homepage.
func WaitForCasOS(url string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	client := &http.Client{Timeout: startupRequestTimeout}
	ticker := time.NewTicker(startupPollInterval)
	defer ticker.Stop()

	for {
		if casOSReady(ctx, client, url) {
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for CasOS at %s", url)
		case <-ticker.C:
		}
	}
}

func casOSReady(ctx context.Context, client *http.Client, url string) bool {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	response, err := client.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return false
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, startupResponseLimit))
	return err == nil && bytes.Contains(body, []byte(casOSPageMarker))
}
