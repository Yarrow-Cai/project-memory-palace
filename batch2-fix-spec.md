# pmem Batch 2 Fix: D1 — Decay should affect retrieval behavior

Context: The pmem codebase at C:\Users\Atop\Desktop\pmem\project-memory-palace\project-memory-palace\

## Problem

The `decay.go` module computes `EffectivePriority` based on `lastAccessedAt`, and `RecordAccess()` updates `access_count` and `last_accessed_at` in SQLite. But the decay system is **display-only** — it does not affect any retrieval path:

- `disclosure()` uses `ListRecent` with filter `{"priority": 3}` — this is the MANUAL priority, not effective
- `recall()` uses `Search()` which filters by status only, not priority at all
- `ListRecent()` in index.go queries `WHERE priority >= ?` — manual priority

This means access frequency has NO impact on what cards are surfaced to the Agent. A card accessed 100 times and a card never accessed are treated identically in retrieval.

## Fix Strategy

The pragmatic fix: modify `ListRecent()` and `Search()` to **sort by effective priority** (manual priority × decay factor) instead of manual priority alone. We don't need to change the WHERE clause — we add a computed sort order.

Specifically:

### Step 1: Add effective_priority computation in SQL queries

File: internal/index/index.go

In the `Recent()` function, change the ORDER BY clause to sort by a computed effective priority. The formula is:
```
manual_priority * decay_factor(last_accessed_at)
```

Where decay_factor:
- < 7 days: 1.0
- < 30 days: 0.85  
- < 60 days: 0.6
- < 180 days: 0.4
- >= 180 days or NULL: 0.25

We can express this in SQLite as:
```sql
CASE
  WHEN last_accessed_at IS NULL OR last_accessed_at == '' THEN 0.25
  WHEN julianday('now') - julianday(last_accessed_at) < 7 THEN 1.0
  WHEN julianday('now') - julianday(last_accessed_at) < 30 THEN 0.85
  WHEN julianday('now') - julianday(last_accessed_at) < 60 THEN 0.6
  WHEN julianday('now') - julianday(last_accessed_at) < 180 THEN 0.4
  ELSE 0.25
END
```

The effective priority sort key is: `CAST(priority AS REAL) * <decay_factor>`

### Step 2: Modify Recent() query

In index.go's Recent() function, change the ORDER BY from:
```sql
ORDER BY priority DESC, updated_at DESC
```
To:
```sql
ORDER BY CAST(priority AS REAL) * CASE...END DESC, updated_at DESC
```

### Step 3: Add access_count tiebreaker

When effective priorities are equal, prefer cards with higher access_count:
```sql
ORDER BY CAST(priority AS REAL) * CASE...END DESC, access_count DESC, updated_at DESC
```

### Step 4: Modify Search() query similarly

The FTS-based Search() should also sort results by effective priority + FTS rank. Currently it sorts by priority DESC, updated_at DESC. Add the same decay factor:

```sql
ORDER BY CAST(m.priority AS REAL) * CASE...END DESC, rank
```

### Step 5: Return effective_priority in results

In `rowToMap()` (or wherever search/recent results are assembled into maps), add the computed `effective_priority` field to the result map so the WebUI can display it.

---

After all changes:
1. Run `go build -o bin/pmem.exe ./cmd/pmem` to verify
2. Run `go vet ./...` to check
3. Commit with: "fix: decay now affects retrieval sort order via effective_priority"

IMPORTANT: IF you find that `julianday()` is not available (SQLite compiled without it), use `strftime('%s', last_accessed_at)` instead and compute seconds-based decay with constants (7*86400, 30*86400, etc.).
