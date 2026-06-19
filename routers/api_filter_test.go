// Copyright 2022 The Casdoor Authors. All Rights Reserved.
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

package routers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/beego/beego"
)

type apiResponse struct {
	Status string `json:"status"`
	Msg    string `json:"msg"`
}

var initTestAppOnce sync.Once

func initTestApp() {
	initTestAppOnce.Do(func() {
		_, currentFile, _, _ := runtime.Caller(0)
		projectRoot := filepath.Dir(filepath.Dir(currentFile))

		beego.TestBeegoInit(projectRoot)
		beego.BeeApp = beego.NewApp()

		InitAPI()
		beego.InsertFilter("/api/*", beego.BeforeRouter, ApiFilter)
	})
}

func performRequest(method, target string, body string) apiResponse {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()

	beego.BeeApp.Handlers.ServeHTTP(recorder, req)

	var resp apiResponse
	_ = json.Unmarshal(recorder.Body.Bytes(), &resp)
	return resp
}

func TestUnauthenticatedSensitiveAPIsRequireSignin(t *testing.T) {
	initTestApp()

	testCases := []struct {
		name   string
		method string
		target string
		body   string
	}{
		{name: "get-nodes", method: http.MethodGet, target: "/api/get-nodes"},
		{name: "get-pods", method: http.MethodGet, target: "/api/get-pods?namespace=default"},
		{name: "get-dashboard", method: http.MethodGet, target: "/api/get-dashboard"},
		{name: "get-worker-kubeconfig", method: http.MethodGet, target: "/api/get-worker-kubeconfig?nodeName=test-worker"},
		{name: "delete-node", method: http.MethodPost, target: "/api/delete-node", body: `{"name":"node-1"}`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp := performRequest(tc.method, tc.target, tc.body)
			if resp.Status != "error" || resp.Msg != "please sign in first" {
				t.Fatalf("expected unauthenticated request to be rejected, got status=%q msg=%q", resp.Status, resp.Msg)
			}
		})
	}
}

func TestSignoutRemainsPublic(t *testing.T) {
	initTestApp()

	resp := performRequest(http.MethodPost, "/api/signout", "")
	if resp.Status != "ok" {
		t.Fatalf("expected signout to remain public, got status=%q msg=%q", resp.Status, resp.Msg)
	}
}
