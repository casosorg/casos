package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/beego/beego/logs"

	"github.com/casosorg/casos/object"
)

// MCPPath is where AI coding agents (Claude Code, Cursor, Codex, ...) reach
// casos over the Model Context Protocol's Streamable HTTP transport. The
// server is stateless: every POST carries one JSON-RPC exchange and is
// answered with plain JSON, so no session or event stream is kept.
const MCPPath = "/mcp"

const mcpMaxBodyBytes = 1 << 20

// Newest first; the first entry is offered to a client asking for a version
// this server does not know.
var mcpProtocolVersions = []string{"2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"}

const mcpInstructions = `This server deploys and operates apps on a casos Kubernetes cluster.

To deploy the user's project from a public Git repository, call deploy_git_repo: casos clones it, builds the image itself (from its Dockerfile, or for a Node.js, Python, Go or static site without one) and deploys it, so no registry is needed. Push your changes first, then call it again to redeploy.

To run a project that ships a docker-compose.yml, call deploy_compose with the file's contents: every service runs, reaching the others by name, and the ones that publish a port get an address.

Otherwise build a container image, push it to a registry the cluster can pull from (Docker Hub, GHCR, a private registry), then call deploy_app with that image. deploy_app creates the app or updates it in place, waits for the rollout, and returns the URLs the app answers on.

If a rollout fails, read get_app_logs (previous=true for a crash loop) and get_app to see why, then fix and redeploy, or call rollback_app to return to the last working revision. When the trouble may be the cluster itself, call get_cluster_health for the overall state and list_cluster_issues for the problems casos has found across it.

To run code instead of deploying it, call create_sandbox for an isolated Linux workspace, optionally with a Git repository cloned and a setup script run. Work in it with exec_in_sandbox, read_sandbox_file and write_sandbox_file, and hand long or GPU work to start_sandbox_job. A sandbox is deleted when its lease runs out; call extend_sandbox to keep working, and delete_sandbox when done. Each sandbox is also a DevBox the user can open in VS Code, so give them its url and password when they want to follow along.`

type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *mcpError       `json:"error,omitempty"`
}

const (
	mcpParseError     = -32700
	mcpInvalidRequest = -32600
	mcpMethodNotFound = -32601
	mcpInvalidParams  = -32602
)

type mcpCaller struct {
	user     string
	token    string
	readOnly bool
	// The address casos was reached by, which new app addresses are named under.
	host string
}

type mcpCallerKey struct{}

func mcpCallerOf(ctx context.Context) mcpCaller {
	caller, _ := ctx.Value(mcpCallerKey{}).(mcpCaller)
	return caller
}

// ServeMCP answers one MCP request authenticated by a casos access token.
func ServeMCP(w http.ResponseWriter, r *http.Request, version string) {
	switch r.Method {
	case http.MethodPost:
	case http.MethodOptions:
		w.WriteHeader(http.StatusNoContent)
		return
	default:
		// No server-initiated stream is offered, which the transport allows
		// a server to signal this way.
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	caller, ok := authenticateAccessToken(r)
	if !ok {
		w.Header().Set("WWW-Authenticate", `Bearer realm="casos"`)
		http.Error(w, "a casos access token is required: send it as \"Authorization: Bearer <token>\"", http.StatusUnauthorized)
		return
	}
	caller.host = r.Host

	body, err := io.ReadAll(io.LimitReader(r.Body, mcpMaxBodyBytes+1))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(body) > mcpMaxBodyBytes {
		http.Error(w, "request body is too large", http.StatusRequestEntityTooLarge)
		return
	}

	body = bytes.TrimSpace(body)
	var messages []mcpRequest
	batch := len(body) > 0 && body[0] == '['
	if batch {
		err = json.Unmarshal(body, &messages)
	} else {
		var message mcpRequest
		err = json.Unmarshal(body, &message)
		messages = []mcpRequest{message}
	}
	if err != nil {
		writeMCPJSON(w, mcpResponse{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &mcpError{Code: mcpParseError, Message: err.Error()}})
		return
	}

	responses := []mcpResponse{}
	for _, message := range messages {
		// Notifications and the client's answers to server requests expect
		// no reply.
		if len(message.ID) == 0 || message.Method == "" {
			continue
		}
		responses = append(responses, handleMCPMessage(r.Context(), caller, version, message))
	}

	if len(responses) == 0 {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	if batch {
		writeMCPJSON(w, responses)
		return
	}
	writeMCPJSON(w, responses[0])
}

func authenticateAccessToken(r *http.Request) (mcpCaller, bool) {
	header := r.Header.Get("Authorization")
	secret := ""
	if len(header) > len("Bearer ") && strings.EqualFold(header[:len("Bearer ")], "Bearer ") {
		secret = strings.TrimSpace(header[len("Bearer "):])
	}
	if secret == "" {
		return mcpCaller{}, false
	}
	token, err := object.VerifyAccessToken(secret)
	if err != nil {
		logs.Warning("mcp: verify access token: %v", err)
		return mcpCaller{}, false
	}
	if token == nil {
		return mcpCaller{}, false
	}
	return mcpCaller{user: token.Owner, token: token.Name, readOnly: token.ReadOnly}, true
}

func writeMCPJSON(w http.ResponseWriter, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		logs.Warning("mcp: write response: %v", err)
	}
}

func handleMCPMessage(ctx context.Context, caller mcpCaller, version string, message mcpRequest) mcpResponse {
	response := mcpResponse{JSONRPC: "2.0", ID: message.ID}
	if message.JSONRPC != "2.0" {
		response.Error = &mcpError{Code: mcpInvalidRequest, Message: "jsonrpc must be \"2.0\""}
		return response
	}

	switch message.Method {
	case "initialize":
		var params struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(message.Params, &params)
		response.Result = map[string]interface{}{
			"protocolVersion": negotiateMCPVersion(params.ProtocolVersion),
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{"listChanged": false},
			},
			"serverInfo": map[string]interface{}{
				"name":    "casos",
				"title":   "CasOS",
				"version": version,
			},
			"instructions": mcpInstructions,
		}
		if caller.readOnly {
			response.Result.(map[string]interface{})["instructions"] = "This token is read-only. Use the listed tools to inspect apps, logs and cluster health and diagnose objects. Deployment, execution, sandbox access and the model gateway are unavailable."
		}
	case "ping":
		response.Result = map[string]interface{}{}
	case "tools/list":
		tools := make([]*mcpTool, 0, len(mcpTools))
		for _, tool := range mcpTools {
			if !caller.readOnly || mcpReadOnlyAllowed(tool) {
				tools = append(tools, tool)
			}
		}
		response.Result = map[string]interface{}{"tools": tools}
	case "tools/call":
		var params struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(message.Params, &params); err != nil {
			response.Error = &mcpError{Code: mcpInvalidParams, Message: err.Error()}
			return response
		}
		tool := findMCPTool(params.Name)
		if tool == nil {
			response.Error = &mcpError{Code: mcpInvalidParams, Message: fmt.Sprintf("unknown tool: %s", params.Name)}
			return response
		}
		response.Result = callMCPTool(ctx, caller, tool, params.Arguments)
	default:
		response.Error = &mcpError{Code: mcpMethodNotFound, Message: fmt.Sprintf("method not found: %s", message.Method)}
	}
	return response
}

func negotiateMCPVersion(requested string) string {
	for _, supported := range mcpProtocolVersions {
		if supported == requested {
			return requested
		}
	}
	return mcpProtocolVersions[0]
}

type mcpToolResult struct {
	Content []mcpContent `json:"content"`
	IsError bool         `json:"isError"`
}

type mcpContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func mcpText(text string, isError bool) mcpToolResult {
	return mcpToolResult{Content: []mcpContent{{Type: "text", Text: text}}, IsError: isError}
}

// callMCPTool reports a failed tool as a result the agent can read and react
// to, not as a protocol error, as the specification asks.
func callMCPTool(ctx context.Context, caller mcpCaller, tool *mcpTool, arguments json.RawMessage) mcpToolResult {
	if caller.readOnly && !mcpReadOnlyAllowed(tool) {
		return mcpText(fmt.Sprintf("permission denied: read-only access tokens cannot call %s", tool.Name), true)
	}
	logs.Info("mcp: %s (token %s) called %s", caller.user, caller.token, tool.Name)

	if len(arguments) == 0 || string(arguments) == "null" {
		arguments = json.RawMessage("{}")
	}
	cfg := getAdminRestConfig()
	if cfg == nil {
		return mcpText("the casos apiserver is not ready yet; try again in a moment", true)
	}

	ctx = context.WithValue(ctx, mcpCallerKey{}, caller)
	result, err := tool.handler(ctx, cfg, arguments)
	if err != nil {
		return mcpText(err.Error(), true)
	}
	if text, ok := result.(string); ok {
		return mcpText(text, false)
	}
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return mcpText(err.Error(), true)
	}
	return mcpText(string(encoded), false)
}
