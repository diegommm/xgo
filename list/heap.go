package list

import (
	"container/heap"
	"encoding/json"
)

// Heap uses container/heap to implement a heap.
type Heap[T any] struct {
	Ordered[T]
}

// heap initializes and returns a heap from the current Ordered. the list will
// be shared, so if you change it outside of the returned heap, you will need
// to call fix or init on the returned heap to reconcile it.
func (o Ordered[T]) Heap() Heap[T] {
	h := Heap[T]{
		Ordered: o,
	}
	h.Init()
	return h
}

// Heap initializes and returns a heap from the current List. the list will
// be shared, so if you change it outside of the returned heap, you will need
// to call fix or init on the returned heap to reconcile it.
func (l *List[T]) Heap(cmp CompareFunc[T]) Heap[T] {
	return l.Ordered(cmp).Heap()
}

// NewHeap creates a new Heap using the given CompareFunc.
func NewHeap[T any](cmp CompareFunc[T]) Heap[T] {
	return Heap[T]{
		Ordered: NewOrdered(cmp),
	}
}

// Init establishes the heap invariants required by the other heap methods.
// Init is idempotent with respect to the heap invariants and may be called
// whenever the heap invariants may have been invalidated. The complexity is
// O(n), where n = h.Len().
func (h Heap[T]) Init() {
	heap.Init((heapInterface[T])(h))
}

// Fix re-establishes the heap ordering after the element at index i has
// changed its value. Changing the value of the element at index i and then
// calling Fix is equivalent to, but less expensive than, calling Remove(h, i)
// followed by a Push of the new value. The complexity is O(log n) where n =
// h.Len().
func (h Heap[T]) Fix(i int) {
	heap.Fix((heapInterface[T])(h), i)
}

// Push pushes the element x onto the heap. The complexity is O(log n) where n
// = h.Len().
func (h Heap[T]) Push(v T) {
	heap.Push((heapInterface[T])(h), v)
}

// Pop removes and returns the minimum element (according to Less) from the
// heap. The complexity is O(log n) where n = h.Len(). Pop is equivalent to
// Remove(h, 0).
func (h Heap[T]) Pop() (v T, ok bool) {
	if h.len == 0 {
		return
	}
	x := heap.Pop((heapInterface[T])(h))
	v, ok = x.(T)
	return
}

// UnmarshalJSON clears the underlying list and reads a JSON Array as a list of
// elements. It always calls Init afterwards, regardless if unmarshaling
// failed, so that the heap is still usable with the items unmarshaled so far.
func (l Heap[T]) UnmarshalJSON(b []byte) error {
	err := json.Unmarshal(b, l.List)
	// even if we fail to unmarshal, we may have decoded some items, so we call
	// Init so that we fix those items
	l.Init()
	return err
}

type heapInterface[T any] Heap[T]

func (h heapInterface[T]) Push(x any) {
	v, _ := x.(T)
	h.InsertFront(v)
}

func (h heapInterface[T]) Pop() any {
	v, _ := h.PopFront()
	return v
}
