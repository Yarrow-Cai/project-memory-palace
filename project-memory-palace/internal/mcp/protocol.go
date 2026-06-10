package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

type Request struct {
	JSONRPC string         ` + json:"jsonrpc" + `
	ID      json.Number    ` + json:"id" + `
	Method  string         ` + json:"method" + `
	Params  map[string]any ` + json:"params" + `
}

type Response struct {
	JSONRPC string         ` + json:"jsonrpc" + `
	ID      json.Number    ` + json:"id" + `
	Result  any            ` + json:"result,omitempty" + `
	Error   *ResponseError ` + json:"error,omitempty" + `
}

type ResponseError struct {
	Code    int    ` + json:"code" + `
	Message string ` + json:"message" + `
}

func ParseRequest(raw []byte) (*Request, error) {
	var req Request
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, fmt.Errorf("invalid json-rpc request: %w", err)
	}
	if req.JSONRPC != "2.0" { return nil, fmt.Errorf("invalid jsonrpc version: %s", req.JSONRPC) }
	if req.Method == "" { return nil, fmt.Errorf("method is required") }
	return &req, nil
}

func NewResponse(id json.Number, result any) Response {
	return Response{JSONRPC: "2.0", ID: id, Result: result}
}

func NewErrorResponse(id json.Number, code int, message string) Response {
	return Response{JSONRPC: "2.0", ID: id, Error: &ResponseError{Code: code, Message: message}}
}

type ToolHandler func(params map[string]any) (any, error)

type ToolDef struct {
	Name, Description string
	Schema  map[string]any
	Handler ToolHandler
}

type ToolRegistry struct {
	mu    sync.RWMutex
	tools map[string]ToolDef
}

func NewToolRegistry() *ToolRegistry { return &ToolRegistry{tools: make(map[string]ToolDef)} }

func (r *ToolRegistry) Register(name, description string, schema map[string]any, handler ToolHandler) {
	r.mu.Lock(); defer r.mu.Unlock()
	r.tools[name] = ToolDef{Name: name, Description: description, Schema: schema, Handler: handler}
}

func (r *ToolRegistry) List() []map[string]any {
	r.mu.RLock(); defer r.mu.RUnlock()
	var out []map[string]any
	for _, t := range r.tools {
		out = append(out, map[string]any{"name": t.Name, "description": t.Description, "inputSchema": t.Schema})
	}
	return out
}

func (r *ToolRegistry) Dispatch(name string, params map[string]any) (any, error) {
	r.mu.RLock(); tool, ok := r.tools[name]; r.mu.RUnlock()
	if !ok { return nil, fmt.Errorf("unknown tool: %s", name) }
	return tool.Handler(params)
}

type StdioServer struct {
	Registry *ToolRegistry
	Reader   io.Reader
	Writer   io.Writer
}

func (s *StdioServer) HandleOne() error {
	scanner := bufio.NewScanner(s.Reader)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil { return fmt.Errorf("read error: %w", err) }
		return io.EOF
	}
	req, err := ParseRequest(scanner.Bytes())
	if err != nil { return s.writeResponse(NewErrorResponse("0", -32700, "Parse error")) }

	// Handle notifications (no id)
	if req.ID.String() == "" {
		switch req.Method {
		case "notifications/initialized":
			return nil // ack silently
		}
		return nil
	}

	var resp Response
	switch req.Method {
	case "initialize":
		resp = NewResponse(req.ID, map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "project-memory-palace", "version": "0.4.0"},
		})
	case "tools/list":
		resp = NewResponse(req.ID, map[string]any{"tools": s.Registry.List()})
	case "tools/call":
		name, _ := req.Params["name"].(string)
		args, _ := req.Params["arguments"].(map[string]any)
		if args == nil { args = map[string]any{} }
		result, err := s.Registry.Dispatch(name, args)
		if err != nil {
			resp = NewErrorResponse(req.ID, -32603, err.Error())
		} else {
			resp = NewResponse(req.ID, result)
		}
	default:
		resp = NewErrorResponse(req.ID, -32601, fmt.Sprintf("unknown method: %s", req.Method))
	}

	return s.writeResponse(resp)
}

func (s *StdioServer) writeResponse(resp Response) error {
	data, _ := json.Marshal(resp)
	data = append(data, '\n')
	_, err := s.Writer.Write(data)
	return err
}
