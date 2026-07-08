# pmem Batch 1 Fixes: B1 + B2 + B3

You are fixing 3 bugs in the pmem Go codebase at:
C:\Users\Atop\Desktop\pmem\project-memory-palace\project-memory-palace\

## B1: SSE sendEvent silently drops responses when channel is full [P0]

File: internal/mcp/sse.go, lines 236-243 (sendEvent function)

Current code:
```go
case <-timer.C:
    log.Printf("mcp: session %s event channel full, dropping response", session.id)
```

Problem: When the event channel (capacity 64) is full, the response is logged and dropped. The HTTP handler still returns 202 Accepted, so the client (Claude Desktop/Codex) thinks the request was accepted but never receives a response.

Fix: Instead of silently dropping, write an HTTP 503 error response via the ResponseWriter. But sendEvent() doesn't have access to http.ResponseWriter. 

Better fix: Increase channel capacity to 256 (from 64), AND in HandleSSE, process events faster by adding a non-blocking send with immediate flush. Actually the simplest fix with minimal refactoring: 

1. Increase channel buffer from 64 to 512 in addSession() (line 52)
2. Add a `select` with a short timeout (1 second) instead of 5 seconds
3. When dropping, log a WARNING (not just log.Printf) with more context

Actually the best fix for the architecture is to change sendEvent to accept the http.ResponseWriter and write the 503 directly. But that requires changing HandleSSE's select loop. The pragmatic fix:

In sendEvent (line 236-243), change:
```go
case <-timer.C:
    log.Printf("mcp: session %s event channel full, dropping response", session.id)
```
To:
```go
case <-timer.C:
    log.Printf("mcp: WARNING session %s event channel full after 5s, dropping response (channel depth=%d)", session.id, len(session.events))
    // Write an error response directly to the SSE stream so the client knows
    errMsg := fmt.Sprintf("event: message\ndata: {\"jsonrpc\":\"2.0\",\"error\":{\"code\":-32000,\"message\":\"server overloaded\"}}\n\n")
    w.Write([]byte(errMsg))  // <- this won't work because w is not available here
```

Hmm, the cleanest fix without major refactoring: just increase channel buffer to 512, reduce timeout to 2s, and improve the log message. This is a reasonable mitigation.

Wait, let me think again. The real fix should notify the client. Let me modify the approach:

1. In `sendEvent`, add a `w http.ResponseWriter` parameter
2. Change HandleSSE to pass `w` to sendEvent
3. On timeout, write a JSON-RPC error to the SSE stream

Files to change:
- internal/mcp/sse.go: sendEvent signature, HandleSSE callsites

Here's the exact change:

In sse.go, change sendEvent signature from:
```go
func (s *SSEServer) sendEvent(session *SSESession, resp Response) {
```
To:
```go
func (s *SSEServer) sendEvent(w http.ResponseWriter, session *SSESession, resp Response) {
```

And in the timeout case:
```go
case <-timer.C:
    log.Printf("mcp: WARNING session %s event channel full (depth=%d), sending overload error to client", session.id, len(session.events))
    // Write JSON-RPC error directly to SSE stream so client knows the request failed
    errPayload := `{"jsonrpc":"2.0","error":{"code":-32000,"message":"server overloaded, try again"}}`
    fmt.Fprintf(w, "event: message\ndata: %s\n\n", errPayload)
```

Update callers:
- HandleSSE line ~129: change `s.sendEvent(session, resp)` to `s.sendEvent(w, session, resp)`

Also increase channel buffer from 64 to 256:
- Line 52: `events: make(chan string, 256)`

Reduce timeout from 5s to 3s:
- Line 237: `time.NewTimer(3 * time.Second)`

## B2: tools/call error codes too coarse [P1]

Files: internal/mcp/protocol.go (line 119), internal/mcp/sse.go (line 210)

Current code: All Dispatch() errors return -32603 (Internal error).

Fix: Distinguish between parameter errors (-32602) and internal errors (-32603). The Dispatch() function already returns errors — we need to check the error message pattern:

In protocol.go line 119 and sse.go line 210, change:
```go
resp = NewErrorResponse(req.ID, -32603, err.Error())
```
To:
```go
code := -32603 // Internal error
if strings.Contains(err.Error(), "invalid") || strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "must be") {
    code = -32602 // Invalid params
}
resp = NewErrorResponse(req.ID, code, err.Error())
```

Add `"strings"` to the imports in both files if not already present.

## B3: audit.go high_confidence_inference only checks "analysis" kind [P1]

File: internal/audit/audit.go, line 46

Current code:
```go
if card.Source.Kind == "analysis" && card.Confidence > 0.7 {
    issues = append(issues, "high_confidence_inference")
}
```

Problem: Only checks "analysis" but v0.6 has 12 source kinds. A card with kind="experiment" and confidence=0.9 should also be flagged.

Fix: Expand to check all inference/observation-based source kinds:
```go
// High-confidence inference from non-authoritative sources should be reviewed
inferenceKinds := map[string]bool{"analysis": true, "experiment": true, "observation": true, "inference": true}
if inferenceKinds[card.Source.Kind] && card.Confidence > 0.7 {
    issues = append(issues, "high_confidence_inference")
}
```

---

After all fixes are applied:
1. Run `go build -o bin/pmem.exe ./cmd/pmem` to verify compilation
2. Run `go vet ./...` to check for issues
3. Commit all changes with message: "fix: batch 1 — SSE response dropping, error codes, audit detection"
