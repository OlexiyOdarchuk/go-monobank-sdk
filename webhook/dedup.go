package webhook

import (
	"container/list"
	"sync"
)

// Deduper is the memory of transaction IDs the handler has already
// processed successfully, so duplicate deliveries from Mono can be
// short-acknowledged.
//
// Mono retries failed deliveries after 60 and 600 seconds. The
// handler calls Has(id) before running OnEvent and Add(id) only
// after a successful OnEvent. This is critical: if id is recorded
// before OnEvent runs, a transient failure (HTTP 500) "poisons" the
// deduper and the next retry from Mono is silently acked without
// running OnEvent again.
//
// The default implementation ([NewMemoryDeduper]) is an in-memory
// LRU and is safe for concurrent use. Plug in your own
// implementation (Redis, SQLite etc.) through this interface — handy
// when state must be shared across service replicas.
type Deduper interface {
	// Has reports whether id was recorded by a previous Add.
	Has(id string) bool
	// Add records id as processed. Calling Add for the same id
	// multiple times is safe (a no-op for an existing entry).
	Add(id string)
}

// NewMemoryDeduper returns an in-memory LRU [Deduper] with the given
// capacity. A capacity of ≤ 0 falls back to 1024.
func NewMemoryDeduper(capacity int) *MemoryDeduper {
	if capacity <= 0 {
		capacity = 1024
	}
	return &MemoryDeduper{
		capacity: capacity,
		order:    list.New(),
		index:    make(map[string]*list.Element, capacity),
	}
}

// MemoryDeduper is an LRU set of strings with a fixed size; safe for
// concurrent use.
type MemoryDeduper struct {
	mu       sync.Mutex
	capacity int
	order    *list.List
	index    map[string]*list.Element
}

// Has reports whether id is currently in the set. As a side effect
// it refreshes its recency (making it MRU), because a lookup counts
// as "use".
func (d *MemoryDeduper) Has(id string) bool {
	if id == "" {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	_, ok := d.index[id]
	if ok {
		d.order.MoveToFront(d.index[id])
	}
	return ok
}

// Add records id as seen. A no-op for an empty id. When capacity is
// reached, evicts the oldest entry.
func (d *MemoryDeduper) Add(id string) {
	if id == "" {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	if el, ok := d.index[id]; ok {
		d.order.MoveToFront(el)
		return
	}
	if d.order.Len() == d.capacity {
		if oldest := d.order.Back(); oldest != nil {
			delete(d.index, oldest.Value.(string))
			d.order.Remove(oldest)
		}
	}
	d.index[id] = d.order.PushFront(id)
}

// Len returns the current number of ids. Useful for diagnostics
// (metrics, debugging).
func (d *MemoryDeduper) Len() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.order.Len()
}
