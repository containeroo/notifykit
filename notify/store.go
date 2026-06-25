package notify

import "sync"

// Store keeps queued notifications by queue id.
type Store struct {
	mu    sync.RWMutex
	items map[string]Notification
}

// NewStore initializes an empty in-memory notification store.
func NewStore() *Store {
	return &Store{items: make(map[string]Notification)}
}

// Put stores a notification by queue id.
func (s *Store) Put(id string, n Notification) {
	if s == nil || id == "" || n == nil {
		return
	}
	s.mu.Lock()
	s.items[id] = n
	s.mu.Unlock()
}

// Get returns a notification by queue id.
func (s *Store) Get(id string) (Notification, bool) {
	if s == nil {
		return nil, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	n, ok := s.items[id]
	return n, ok
}

// Delete removes a notification by queue id.
func (s *Store) Delete(id string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	delete(s.items, id)
	s.mu.Unlock()
}
