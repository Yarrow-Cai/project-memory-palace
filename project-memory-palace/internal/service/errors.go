package service

// MemoryNotFoundError is returned when a memory ID cannot be found in the
// project card store.
type MemoryNotFoundError struct {
	ID string
}

func (e *MemoryNotFoundError) Error() string {
	return "memory not found: " + e.ID
}