package mcp

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestParseRequest(t *testing.T) {
	req, err := ParseRequest([]byte(`{"jsonrpc":"2.0","id":1,"method":"ping","params":{}}`))
	if err != nil { t.Fatalf("unexpected error: %v", err) }
	if req.Method != "ping" { t.Fatal("expected method ping") }
}

func TestNewResponse(t *testing.T) {
	resp := NewResponse("1", "ok")
	if resp.Result != "ok" { t.Fatal("result mismatch") }
}

func TestNewErrorResponse(t *testing.T) {
	resp := NewErrorResponse("1", -32601, "not found")
	if resp.Error == nil || resp.Error.Code != -32601 { t.Fatal("error response mismatch") }
}

func TestToolRegistry(t *testing.T) {
	reg := NewToolRegistry()
	reg.Register("test", "desc", map[string]any{},
		func(p map[string]any) (any, error) { return "ok", nil })
	if len(reg.List()) != 1 { t.Fatal("should have 1 tool") }
	result, _ := reg.Dispatch("test", nil)
	if result != "ok" { t.Fatal("dispatch failed") }
}

func TestToolDispatchUnknown(t *testing.T) {
	reg := NewToolRegistry()
	_, err := reg.Dispatch("unknown", nil)
	if err == nil { t.Fatal("expected error") }
}

func TestStdioServer(t *testing.T) {
	reg := NewToolRegistry()
	reg.Register("ping", "", nil, func(p map[string]any) (any, error) { return "pong", nil })
	in := bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ping","arguments":{}}}` + "\n")
	out := &bytes.Buffer{}
	srv := &StdioServer{Registry: reg, Reader: in, Writer: out}
	srv.Serve()
	var resp Response
	json.Unmarshal(out.Bytes(), &resp)
	if resp.Error != nil { t.Fatalf("unexpected error: %+v", resp.Error) }
}

func TestParseInvalid(t *testing.T) {
	_, err := ParseRequest([]byte(`{bad}`))
	if err == nil { t.Fatal("expected error") }
}
