# pmem Batch 3: WebUI v0.6 Sync

Project: C:\Users\Atop\Desktop\pmem\project-memory-palace\project-memory-palace\

## Problem

The WebUI is stuck at v0.5 while MCP already has v0.6 features. The Web UI:
- Cannot see access_count, last_accessed_at, effective_priority
- Cannot see knowledge_kind (fact/interpretation/rule)
- Cannot see source_agent (which agent wrote this)
- Cannot edit most fields (only status via /api/update)
- Has no decay visualization

## Fixes Required

### 1. Web API: Expand /api/update to support all updatable fields

File: cmd/pmem/main.go (or internal/tray/tray.go — check both HTTP handler registrations)

Current /api/update only supports:
```
status=expired (or active/stale/superseded)
```

Add support for:
- confidence=0.0-1.0
- tags=tag1,tag2 (comma-separated)
- source_agent=name
- knowledge_kind=fact|interpretation|rule
- expires_at=ISO8601
- priority=1-5

Implementation: parse all query params, build updates map, call service.UpdateMemory().

### 2. Web UI: Show v0.6 fields in card detail panel

File: web/index.html

Add to the detail panel (around where card fields are displayed):
- access_count (icon: 👁️)
- last_accessed_at (icon: 🕐, formatted as relative time like "2h ago")
- effective_priority (shown as "3→2.6" using the decay formula, with a visual bar)
- knowledge_kind badge (fact=📊, interpretation=💡, rule=📐)
- source_agent badge (if non-empty)

### 3. Web UI: Add decay visualization in sidebar

File: web/index.html

In the card list/sidebar, next to each card's title, show a small decay indicator:
- A colored dot or bar showing effective vs manual priority
- Green if effective == manual (fresh)
- Yellow if effective is 60-99% of manual
- Red if effective < 60% of manual (stale)

### 4. Web UI: Add edit capability

Add an "Edit" button in the card detail panel that opens an inline form allowing:
- Edit tags (add/remove)
- Change status
- Edit confidence
- Edit priority
- Edit knowledge_kind dropdown

POST changes to /api/update

---

Implementation approach:
1. First fix the Go backend (API handlers)
2. Then fix the HTML/JS frontend
3. Build and verify

After changes:
```bash
cd C:\Users\Atop\Desktop\pmem\project-memory-palace\project-memory-palace
go build -o bin/pmem.exe ./cmd/pmem
```

Test:
```bash
# Start server
./bin/pmem.exe serve-web .
# Open http://127.0.0.1:8147
# Verify: detail panel shows new fields, edit button works, decay indicators visible
```
