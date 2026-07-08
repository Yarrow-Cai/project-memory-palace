# pmem Batch 4 Fix: B4 — Access tracking lost on reindex

Project: C:\Users\Atop\Desktop\pmem\project-memory-palace\project-memory-palace\

## Problem

`Rebuild()` (internal/index/index.go:441-455) reconstructs the SQLite index from YAML files:

```go
func (idx *MemoryIndex) Rebuild() error {
    cards, err := store.DiscoverCards(idx.projectRoot)  // ← YAML has NO access_count/last_accessed_at
    idx.Initialize()
    idx.Clear()  // ← WIPES existing access tracking data!
    ...
    for _, c := range cards {
        doUpsert(tx, c)  // ← INSERTs with access_count=0, last_accessed_at=''
    }
}
```

MemoryCard struct has NO `access_count` or `last_accessed_at` fields (card.go:22-41), so they survive only in SQLite. When Rebuild wipes SQLite and rebuilds from YAML, all tracking data is lost.

## Fix

Before `Clear()`, save the access tracking data from the existing index. After rebuilding from YAML, restore it.

### Step 1: Add function to save access tracking data

In index.go, add before Rebuild():

```go
// saveAccessData reads access_count and last_accessed_at from the current index
// and returns them as maps keyed by memory ID.
func (idx *MemoryIndex) saveAccessData() (map[string]int, map[string]string, error) {
    db, err := idx.connect()
    if err != nil { return nil, nil, err }
    rows, err := db.Query("SELECT id, access_count, last_accessed_at FROM memories")
    if err != nil { return nil, nil, err }
    defer rows.Close()
    counts := make(map[string]int)
    lastAccess := make(map[string]string)
    for rows.Next() {
        var id, lastAt string
        var count int
        if err := rows.Scan(&id, &count, &lastAt); err != nil { continue }
        counts[id] = count
        lastAccess[id] = lastAt
    }
    return counts, lastAccess, nil
}
```

### Step 2: Add function to restore access tracking data

```go
func (idx *MemoryIndex) restoreAccessData(counts map[string]int, lastAccess map[string]string) error {
    db, err := idx.connect()
    if err != nil { return err }
    for id, count := range counts {
        lastAt := lastAccess[id]
        db.Exec("UPDATE memories SET access_count=?, last_accessed_at=? WHERE id=?", count, lastAt, id)
    }
    return nil
}
```

### Step 3: Modify Rebuild() to preserve access data

```go
func (idx *MemoryIndex) Rebuild() error {
    cards, err := store.DiscoverCards(idx.projectRoot)
    if err != nil { return fmt.Errorf("rebuild: %w", err) }
    idx.Initialize()
    
    // Save access tracking before wiping
    accessCounts, lastAccess, _ := idx.saveAccessData()
    
    idx.Clear()
    db, err := idx.connect()
    if err != nil { return err }
    tx, err := db.Begin()
    if err != nil { return fmt.Errorf("rebuild begin: %w", err) }
    defer tx.Rollback()
    for _, c := range cards {
        if err := doUpsert(tx, c); err != nil { return fmt.Errorf("rebuild %s: %w", c.ID, err) }
    }
    if err := tx.Commit(); err != nil { return err }
    
    // Restore access tracking
    idx.restoreAccessData(accessCounts, lastAccess)
    
    return nil
}
```

After fix:
```bash
go build -o bin/pmem.exe ./cmd/pmem && go vet ./...
```

Commit: "fix: preserve access tracking data across reindex"
