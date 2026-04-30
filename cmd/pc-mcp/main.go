// Command pc-mcp is the Promptcellar MCP retrieval server.
//
// It speaks JSON-RPC 2.0 over stdio (the MCP transport pattern) and exposes
// four read-only tools: search, log, touched, session — all reading the
// `.prompts/` directory of the cwd it was launched in (which is the user's
// project root, since CC starts MCP servers there).
//
// The server is registered via plugin/.mcp.json. Off by default in the plugin
// manifest until the user opts in (TODO: `enabled` flag once we have config
// for it; for now MCP availability follows whatever Claude Code does with
// plugin-shipped MCP servers).
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"promptcellar/internal/capture"
	"promptcellar/internal/plf"
	"promptcellar/internal/plfread"
)

const (
	serverName    = "promptcellar"
	serverVersion = "0.3.0"
)

type rpcReq struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResp struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcErr         `json:"error,omitempty"`
}

type rpcErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func main() {
	cwd, _ := os.Getwd()
	enc := json.NewEncoder(os.Stdout)
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 1<<20), 32<<20)

	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var req rpcReq
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}
		// Notifications (no ID): no response.
		if len(req.ID) == 0 {
			continue
		}
		resp := handle(&req, cwd)
		_ = enc.Encode(resp)
	}
}

func handle(req *rpcReq, cwd string) *rpcResp {
	resp := &rpcResp{JSONRPC: "2.0", ID: req.ID}
	switch req.Method {
	case "initialize":
		resp.Result = map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": serverName, "version": serverVersion},
		}
	case "tools/list":
		resp.Result = map[string]any{"tools": toolDefs()}
	case "tools/call":
		out, err := handleToolCall(req.Params, cwd)
		if err != nil {
			resp.Error = err
		} else {
			resp.Result = out
		}
	case "ping":
		resp.Result = map[string]any{}
	default:
		resp.Error = &rpcErr{Code: -32601, Message: "method not found: " + req.Method}
	}
	return resp
}

func toolDefs() []map[string]any {
	return []map[string]any{
		{
			"name":        "promptcellar.search",
			"description": "Search prompts in this repo's .prompts/ store by case-insensitive substring on prompt text.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string", "description": "Substring to match against prompt text"},
					"limit": map[string]any{"type": "integer", "minimum": 1, "default": 20},
				},
				"required": []string{"query"},
			},
		},
		{
			"name":        "promptcellar.log",
			"description": "Return the most recent N prompts (newest first).",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"limit": map[string]any{"type": "integer", "minimum": 1, "default": 20},
				},
			},
		},
		{
			"name":        "promptcellar.touched",
			"description": "Return prompts whose outcome.files_touched contains the given path substring.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":  map[string]any{"type": "string"},
					"limit": map[string]any{"type": "integer", "minimum": 1, "default": 50},
				},
				"required": []string{"path"},
			},
		},
		{
			"name":        "promptcellar.session",
			"description": "Return all records belonging to the given session_id, in chronological order.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_id": map[string]any{"type": "string"},
				},
				"required": []string{"session_id"},
			},
		},
	}
}

func handleToolCall(params json.RawMessage, cwd string) (any, *rpcErr) {
	var p struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &rpcErr{Code: -32602, Message: "invalid params: " + err.Error()}
	}
	records, _ := plfread.ReadAll(capture.PromptsRoot(cwd))

	var matches []plf.Record
	switch p.Name {
	case "promptcellar.search":
		q, _ := p.Arguments["query"].(string)
		matches = plfread.Search(records, q)
		matches = limitN(matches, intArg(p.Arguments, "limit", 20))
	case "promptcellar.log":
		matches = limitN(records, intArg(p.Arguments, "limit", 20))
	case "promptcellar.touched":
		path, _ := p.Arguments["path"].(string)
		matches = plfread.Touched(records, path)
		matches = limitN(matches, intArg(p.Arguments, "limit", 50))
	case "promptcellar.session":
		sid, _ := p.Arguments["session_id"].(string)
		matches = plfread.Session(records, sid)
	default:
		return nil, &rpcErr{Code: -32601, Message: "unknown tool: " + p.Name}
	}

	return map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": formatMatches(matches)},
		},
		"isError": false,
	}, nil
}

func intArg(args map[string]any, key string, dflt int) int {
	v, ok := args[key]
	if !ok {
		return dflt
	}
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	}
	return dflt
}

func limitN(records []plf.Record, n int) []plf.Record {
	if n <= 0 || n >= len(records) {
		return records
	}
	return records[:n]
}

func formatMatches(records []plf.Record) string {
	if len(records) == 0 {
		return "(no matching records)"
	}
	var b strings.Builder
	for _, r := range records {
		b.WriteString(formatLine(&r))
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatLine(r *plf.Record) string {
	if r.Excluded != nil {
		return fmt.Sprintf("%s  [excluded:%s] %s", r.Timestamp, r.Excluded.PatternID, r.Excluded.Reason)
	}
	status := "?"
	files, commits := 0, 0
	cost := 0.0
	if r.Outcome != nil {
		if r.Outcome.Status != "" {
			status = r.Outcome.Status
		}
		files = len(r.Outcome.FilesTouched)
		commits = len(r.Outcome.Commits)
	}
	if r.Enrichments != nil {
		cost = r.Enrichments.CostUSD
	}
	prompt := strings.ReplaceAll(r.Prompt, "\n", " ")
	if len(prompt) > 100 {
		prompt = prompt[:97] + "..."
	}
	extras := []string{}
	if files > 0 {
		extras = append(extras, fmt.Sprintf("files:%d", files))
	}
	if commits > 0 {
		extras = append(extras, fmt.Sprintf("commits:%d", commits))
	}
	if cost > 0 {
		extras = append(extras, fmt.Sprintf("$%.4f", cost))
	}
	tail := ""
	if len(extras) > 0 {
		tail = "  (" + strings.Join(extras, ", ") + ")"
	}
	return fmt.Sprintf("%s  [%s]  %s%s\n  id=%s session=%s", r.Timestamp, status, prompt, tail, r.ID, r.SessionID)
}
