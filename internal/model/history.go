package model

// DefaultHistoryLength is the default number of retained samples.
const DefaultHistoryLength = 300

// RingBuffer is a bounded FIFO of at most Capacity items.
type RingBuffer[T any] struct {
	capacity int
	items    []T
	start    int
}

// NewRingBuffer creates a buffer retaining at most capacity items.
func NewRingBuffer[T any](capacity int) *RingBuffer[T] {
	if capacity < 1 {
		capacity = 1
	}
	return &RingBuffer[T]{capacity: capacity}
}

// Capacity returns the maximum number of items retained.
func (r *RingBuffer[T]) Capacity() int { return r.capacity }

// Append adds an item, discarding the oldest one when full.
func (r *RingBuffer[T]) Append(item T) {
	if len(r.items) < r.capacity {
		r.items = append(r.items, item)
		return
	}
	r.items[r.start] = item
	r.start = (r.start + 1) % r.capacity
}

// Items returns the retained items oldest-first.
func (r *RingBuffer[T]) Items() []T {
	out := make([]T, 0, len(r.items))
	for i := 0; i < len(r.items); i++ {
		out = append(out, r.items[(r.start+i)%r.capacity])
	}
	return out
}

// Last returns the most recent item and whether one exists.
func (r *RingBuffer[T]) Last() (T, bool) {
	var zero T
	if len(r.items) == 0 {
		return zero, false
	}
	return r.items[(r.start+len(r.items)-1)%r.capacity], true
}

// Len returns the number of retained items.
func (r *RingBuffer[T]) Len() int { return len(r.items) }

// Clear drops all retained items.
func (r *RingBuffer[T]) Clear() {
	r.items = r.items[:0]
	r.start = 0
}
