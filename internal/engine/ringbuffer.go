package engine

import "sync"

// RingBuffer is a fixed-size circular buffer that overwrites oldest entries
// when full. It is safe for concurrent use.
type RingBuffer[T any] struct {
	mu    sync.RWMutex
	data  []T
	head  int // next write position
	count int
	cap   int
}

// NewRingBuffer creates a new ring buffer with the given capacity.
func NewRingBuffer[T any](capacity int) *RingBuffer[T] {
	if capacity < 1 {
		capacity = 1
	}
	return &RingBuffer[T]{
		data: make([]T, capacity),
		cap:  capacity,
	}
}

// Push adds an item to the buffer, overwriting the oldest if full.
func (r *RingBuffer[T]) Push(item T) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.data[r.head] = item
	r.head = (r.head + 1) % r.cap
	if r.count < r.cap {
		r.count++
	}
}

// Len returns the number of items in the buffer.
func (r *RingBuffer[T]) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.count
}

// All returns all items in the buffer, oldest first.
func (r *RingBuffer[T]) All() []T {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.count == 0 {
		return nil
	}

	result := make([]T, r.count)
	start := 0
	if r.count == r.cap {
		start = r.head // oldest item is at head when buffer is full
	}

	for i := 0; i < r.count; i++ {
		idx := (start + i) % r.cap
		result[i] = r.data[idx]
	}

	return result
}

// Last returns the most recently added item.
func (r *RingBuffer[T]) Last() (T, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var zero T
	if r.count == 0 {
		return zero, false
	}

	idx := (r.head - 1 + r.cap) % r.cap
	return r.data[idx], true
}

// LastN returns the N most recently added items, newest first.
func (r *RingBuffer[T]) LastN(n int) []T {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if n > r.count {
		n = r.count
	}
	if n == 0 {
		return nil
	}

	result := make([]T, n)
	for i := 0; i < n; i++ {
		idx := (r.head - 1 - i + r.cap) % r.cap
		result[i] = r.data[idx]
	}
	return result
}

// Clear empties the buffer.
func (r *RingBuffer[T]) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	var zero T
	for i := range r.data {
		r.data[i] = zero
	}
	r.head = 0
	r.count = 0
}
