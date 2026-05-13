package webhook

import (
	"container/list"
	"sync"
)

// Deduper — інтерфейс пам'яті ID-шників транзакцій, які handler уже
// успішно обробив, щоб коротко відповісти на повторні доставки Mono.
//
// Mono ретраїть невдалі доставки через 60 і 600 секунд. Handler викликає
// Has(id) перед запуском OnEvent і Add(id) — тільки після успішного
// OnEvent. Це критично: якщо записати id до запуску OnEvent, тимчасовий
// збій (HTTP 500) «отруїть» deduper і наступний ретрай від Mono буде
// тихо ack-нутий без повторного запуску OnEvent.
//
// Реалізація за замовчуванням ([NewMemoryDeduper]) — in-memory LRU,
// safe for concurrent use. Свою реалізацію (Redis, SQLite тощо) можна
// підставити через цей інтерфейс — корисно, коли треба шерити стан між
// репліками сервісу.
type Deduper interface {
	// Has повертає true, якщо id було збережено попереднім Add.
	Has(id string) bool
	// Add реєструє id як оброблений. Викликати Add для одного id кілька
	// разів — безпечно (no-op для існуючого).
	Add(id string)
}

// NewMemoryDeduper повертає in-memory LRU [Deduper] із заданою ємністю.
// Ємність ≤ 0 фолбекає на 1024.
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

// MemoryDeduper — LRU-множина рядків фіксованого розміру, safe for
// concurrent use.
type MemoryDeduper struct {
	mu       sync.Mutex
	capacity int
	order    *list.List
	index    map[string]*list.Element
}

// Has повідомляє, чи id зараз у множині. Як побічний ефект освіжає його
// recency (роблячи MRU), бо звернення = «використання».
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

// Add реєструє id як побачений. No-op для порожнього id. При досягненні
// capacity витісняє найдавніший елемент.
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

// Len повертає кількість поточних id-шників. Корисно для діагностики
// (метрики, дебаг).
func (d *MemoryDeduper) Len() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.order.Len()
}
