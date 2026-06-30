package notify

import "sync"

// store keeps queued notifications by queue id.
type store struct {
	mu    sync.RWMutex
	items map[string]Notification
}

// newStore initializes an empty in-memory notification store.
func newStore() *store {
	return &store{items: make(map[string]Notification)}
}

// put stores a notification by queue id.
func (s *store) put(id string, n Notification) {
	if s == nil || id == "" || n == nil {
		return
	}
	s.mu.Lock()
	s.items[id] = n
	s.mu.Unlock()
}

// get returns a notification by queue id.
func (s *store) get(id string) (Notification, bool) {
	if s == nil {
		return nil, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	n, ok := s.items[id]
	return n, ok
}

// delete removes a notification by queue id.
func (s *store) delete(id string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	delete(s.items, id)
	s.mu.Unlock()
}
