package list

import (
	"slices"
	"sort"
)

// CompareFunc shoud return:
//
//	-1 if x is less than y,
//	0 if x equals y,
//	+1 if x is greater than y.
//
// It must be a strict weak ordering for sorting methods to work properly. See
// https://en.wikipedia.org/wiki/Weak_ordering#Strict_weak_orderings.
type CompareFunc[T any] func(x T, y T) int

// Inverse inverts the operands of f to achieve the opposite ordering.
func (f CompareFunc[T]) Inverse(x, y T) int { return f(y, x) }

// Ordered is a List whose elements can be compared, and it satisfies
// sort.Interface.
type Ordered[T any] struct {
	*List[T]
	cmp CompareFunc[T]
}

// Ordered returns an Ordered based on this list and the given CompareFunc.
func (l *List[T]) Ordered(cmp CompareFunc[T]) Ordered[T] {
	o := NewOrdered[T](cmp)
	o.List = l
	return o
}

// NewOrdered creates a new Ordered using the given CompareFunc.
func NewOrdered[T any](cmp CompareFunc[T]) Ordered[T] {
	return Ordered[T]{
		cmp:  cmp,
		List: new(List[T]),
	}
}

// Invert returns an Ordered with the comparison function inverted, reusing the
// same underlying data.
// Example:
//
//	o := NewOrdered[int](cmp.Compare)
//	o.InsertFront(5, 3, 8, 9)
//	fmt.Println(o) // [5, 3, 8, 9]
//	fmt.Println(o.Heap().Pop())  // 3, true
//	fmt.Println(o.Invert().Heap().Pop())  // 9, true
//	o.Sort()
//	fmt.Println(o) // [5, 8]
//	o.Invert().Sort()
//	fmt.Println(o) // [8, 5]
func (o Ordered[T]) Invert() Ordered[T] {
	o.cmp = o.cmp.Inverse
	return o
}

// Less returns whether the given element at i is less than the element at j.
// If the list is empty, it always returns false.
func (o Ordered[T]) Less(i, j int) bool {
	return o.cmp(o.At(i), o.At(j)) < 0
}

// Compare returns:
//
//	-1 if the element at position i is less than the element at position j,
//	0 if the element at position i equals the element at position j, or if the
//	list is empty,
//	+1 if the element at position i is greater than the element at position j.
func (o Ordered[T]) Compare(i, j int) int {
	return o.cmp(o.At(i), o.At(j))
}

// Sort sorts data in ascending order as determined by the Less method. The
// sort is not guaranteed to be stable.
func (o Ordered[T]) Sort() {
	if !o.wraps() {
		slices.SortFunc(o.s[o.back:o.back+o.len], o.cmp)
	} else {
		sort.Sort(o)
	}
}

// SortStable sorts data in ascending order as determined by the Less method,
// while keeping the original order of equal elements.
func (o Ordered[T]) SortStable() {
	if !o.wraps() {
		slices.SortStableFunc(o.s[o.back:o.back+o.len], o.cmp)
	} else {
		sort.Stable(o)
	}
}

// IsSorted reports whether data is sorted in ascending order.
func (o Ordered[T]) IsSorted() bool {
	if !o.wraps() {
		return slices.IsSortedFunc(o.s[o.back:o.back+o.len], o.cmp)
	}
	return sort.IsSorted(o)
}

// Find uses binary search to find and return the smallest index i at which the
// list element is >= v.
func (o Ordered[T]) Find(v T) (i int, found bool) {
	return sort.Find(o.len, func(i int) int {
		return o.cmp(o.At(i), v)
	})
}

// Contains returns whether v is found in the data.
func (o Ordered[T]) Contains(v T) bool {
	if i, found := o.Find(v); found {
		return o.cmp(o.At(i), v) == 0
	}
	return false
}
