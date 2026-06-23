package acquiring

import (
	"container/list"
	"sync"
)

// Deduper is the memory of webhook deliveries the handler has already
// processed, so Mono's redeliveries can be short-acknowledged.
//
// Unlike the personal API (which carries a unique transaction ID),
// acquiring webhooks identify a state by the pair (invoiceId,
// modifiedDate): the same invoice produces several webhooks over its
// life (created → processing → success), and each retry of one state
// repeats the same modifiedDate. [DedupKey] builds the composite key
// from an [InvoiceStatusResponse].
//
// The handler calls Has(key) before OnEvent and Add(key) only after
// OnEvent succeeds — recording before would "poison" the key so a
// transient 5xx is never retried.
//
// The default [NewMemoryDeduper] is an in-memory LRU, safe for
// concurrent use. For production across replicas or restarts, plug in
// a persistent implementation (Redis, SQL): an in-memory set loses
// state on restart, which re-opens the replay window (acquiring
// payloads carry no freshness token — see [VerifyWebhookFresh]).
type Deduper interface {
	// Has reports whether key was recorded by a previous Add.
	Has(key string) bool
	// Add records key as processed. Repeated Adds for the same key
	// are a no-op.
	Add(key string)
}

// DedupKey is the canonical dedup key for an acquiring webhook:
// invoiceId joined with modifiedDate. Two deliveries that share both
// describe the same state transition and should run OnEvent once.
// When modifiedDate is empty (rare, older payloads) the key degrades
// to invoiceId alone — distinct states then collapse, which is safer
// than treating every retry as new.
func DedupKey(inv *InvoiceStatusResponse) string {
	if inv == nil {
		return ""
	}
	if inv.ModifiedDate == "" {
		return inv.InvoiceID
	}
	return inv.InvoiceID + "|" + inv.ModifiedDate
}

// NewMemoryDeduper returns an in-memory LRU [Deduper] with the given
// capacity. A capacity ≤ 0 falls back to 1024.
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

// MemoryDeduper is a fixed-size LRU set of strings, safe for
// concurrent use.
type MemoryDeduper struct {
	mu       sync.Mutex
	capacity int
	order    *list.List
	index    map[string]*list.Element
}

// Has reports whether key is currently in the set, refreshing its
// recency (a lookup counts as use).
func (d *MemoryDeduper) Has(key string) bool {
	if key == "" {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	el, ok := d.index[key]
	if ok {
		d.order.MoveToFront(el)
	}
	return ok
}

// Add records key as seen. A no-op for an empty key. When capacity
// is reached, the least-recently-used entry is evicted.
func (d *MemoryDeduper) Add(key string) {
	if key == "" {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	if el, ok := d.index[key]; ok {
		d.order.MoveToFront(el)
		return
	}
	if d.order.Len() == d.capacity {
		if oldest := d.order.Back(); oldest != nil {
			delete(d.index, oldest.Value.(string))
			d.order.Remove(oldest)
		}
	}
	d.index[key] = d.order.PushFront(key)
}

// Len returns the current number of keys (for diagnostics/metrics).
func (d *MemoryDeduper) Len() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.order.Len()
}
