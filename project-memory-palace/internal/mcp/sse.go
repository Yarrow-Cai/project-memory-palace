package mcp

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type SSESession struct {
	id     string
	events chan string
	done   chan struct{}
	closed bool
}

type SSEServer struct {
	Registry *ToolRegistry
	mu       sync.Mutex
	sessions map[string]*SSESession
}

func NewSSEServer(registry *ToolRegistry) *SSEServer {
	return &SSEServer{
		Registry: registry,
		sessions: make(map[string]*SSESession),
	}
}

func newSessionID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *SSEServer) addSession() *SSESession {
	s.mu.Lock()
	defer s.mu.Unlock()
	ses := &SSESession{
		id:     newSessionID(),
		events: make(chan string, 64),
		done:   make(chan struct{}),
	}
	s.sessions[ses.id] = ses
	return ses
}

func (s *SSEServer) removeSession(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ses, ok := s.sessions[id]
	if !ok || ses.closed {
		return
	}
	ses.closed = true
	close(ses.done)
	delete(s.sessions, id)
}

func (s *SSEServer) getSession(id string) *SSESession {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessions[id]
}

func (s *SSEServer) HandleSSE(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rc := recover(); rc != nil {
			log.Printf("mcp: panic in HandleSSE: %v", rc)
		}
	}()

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	session := s.addSession()
	defer s.removeSession(session.id)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	flusher.Flush()

	// Send absolute URL so clients don't need to resolve relative paths
	msgURL := fmt.Sprintf("http://127.0.0.1:8147/message?sessionId=%s", session.id)
	if _, err := fmt.Fprintf(w, "event: endpoint\ndata: %s\n\n", msgURL); err != nil {
		log.Printf("mcp: write endpoint event: %v", err)
		return
	}
	flusher.Flush()
	log.Printf("mcp: session %s started", session.id)

	for {
		select {
		case evt := <-session.events:
			if _, err := fmt.Fprint(w, evt); err != nil {
				log.Printf("mcp: session %s write event: %v", session.id, err)
				return
			}
			flusher.Flush()
		case <-r.Context().Done():
			log.Printf("mcp: session %s client disconnected", session.id)
			return
		case <-session.done:
			log.Printf("mcp: session %s closed", session.id)
			return
		}
	}
}

func (s *SSEServer) HandleMessage(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rc := recover(); rc != nil {
			log.Printf("mcp: panic in HandleMessage: %v", rc)
		}
	}()

	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != "POST" {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	sessionID := r.URL.Query().Get("sessionId")
	if sessionID == "" {
		http.Error(w, "sessionId required", http.StatusBadRequest)
		return
	}

	session := s.getSession(sessionID)
	if session == nil {
		log.Printf("mcp: message for unknown session %s", sessionID)
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("mcp: session %s read body: %v", session.id, err)
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	req, err := ParseRequest(body)
	if err != nil {
		s.sendEvent(session, NewErrorResponse("0", -32700, "Parse error"))
		w.WriteHeader(http.StatusAccepted)
		return
	}

	var resp Response
	switch req.Method {
	case "tools/list":
		resp = NewResponse(req.ID, map[string]any{"tools": s.Registry.List()})
	case "initialize":
		resp = NewResponse(req.ID, map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "project-memory-palace", "version": "0.3.0"},
		})
	case "tools/call":
		name, _ := req.Params["name"].(string)
		args, _ := req.Params["arguments"].(map[string]any)
		if args == nil {
			args = map[string]any{}
		}
		result, err := s.Registry.Dispatch(name, args)
		if err != nil {
			resp = NewErrorResponse(req.ID, -32603, err.Error())
		} else {
			resp = NewResponse(req.ID, result)
		}
	case "notifications/initialized":
		w.WriteHeader(http.StatusAccepted)
		return
	default:
		resp = NewErrorResponse(req.ID, -32601, fmt.Sprintf("unknown method: %s", req.Method))
	}

	s.sendEvent(session, resp)
	w.WriteHeader(http.StatusAccepted)
}

func (s *SSEServer) sendEvent(session *SSESession, resp Response) {
	data, err := json.Marshal(resp)
	if err != nil {
		log.Printf("mcp: marshal response: %v", err)
		return
	}
	msg := fmt.Sprintf("event: message\ndata: %s\n\n", string(data))
	timer := time.NewTimer(5 * time.Second)
	select {
	case session.events <- msg:
		timer.Stop()
	case <-timer.C:
		log.Printf("mcp: session %s event channel full, dropping response", session.id)
	}
}

func BuildMCPConfig(exePath string, projectRoot string) string {
	cfg := map[string]any{
		"mcpServers": map[string]any{
			"project-memory-palace": map[string]any{
				"transport": "sse",
				"url":       "http://127.0.0.1:8147/sse",
			},
		},
	}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	return strings.TrimSpace(string(data))
}
