package list

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"strings"
)

// Package level errors for List.
var (
	ErrInvalidPosition   = errors.New("invalid element position")
	ErrInvalidRange      = errors.New("invalid range")
	ErrInvalidAmount     = errors.New("invalid amount of elements")
	ErrInvalidAllocation = errors.New("insufficient space allocated")
)

// AllocFunc is a function that allocates a new slice that needs to hold at
// least min elements. If max >=0, then expect min<=max, and the returned slice
// should not have a length greater than max (see FixAllocSize helper). The
// returned slice will not be resliced further than its returned length.
// Implementations should not hold references to the returned slice. Note that
// the returned slice will be lazily zeroed as needed, so if any references
// need to be clread from previous work, then it should be done before
// returning it.
type AllocFunc[T any] func(min, max int) ([]T, error)

// FixAllocSize is a helper for implementators of AllocFunc to make sure the
// max capacity is not exceeded.
func FixAllocSize(newIntendedCap, maxCap int) int {
	if 0 <= maxCap && maxCap < newIntendedCap {
		return maxCap
	}
	return newIntendedCap
}

// AllocDefault is the default AllocFunc[T].
func AllocDefault[T any](min, max int) ([]T, error) {
	size := min*3/2 + 1
	size = FixAllocSize(size, max)
	return make([]T, size), nil
}

// List is a slice-based list container, indexed from its back to its front
// starting at zero. Not safe for concurrent use. It zeroes elements that are
// removed from the list. The implementation is panic free, so errors are
// returned for the cases where bounds and other input need to be validated.
type List[T any] struct {
	// AllocFunc allows customizing allocation of a new slice when more space
	// is needed. If nil, AllocDefault is used.
	AllocFunc[T]

	// FreeFunc will be called, if set, with the old slice after migrating to a
	// newly allocated slice. This is useful if you want to put a slice back
	// into a pool or do something else with it, but all the items that the
	// list had used will have been zeroed out.
	FreeFunc func([]T)

	// StringFunc allows customizing how elements are converted to string when
	// printing the list. The default is using fmt.Sprintf("%v", element).
	StringFunc func(T) string

	s []T
	view
}

// New creates a new List using the given slice as the underlying data. If
// useValues is true, then the list will use all the elements of the slice as
// if they had been inserted in an empty list with InsertFront. Otherwise, the
// list will ignore the current elements and be empty. You should not hold
// references to s after calling this function. If useValues is false, the
// elements are not zeroed.
func New[T any](s []T, useValues bool) *List[T] {
	if useValues {
		return &List[T]{
			s: s,
			view: view{
				slen: len(s),
				len:  len(s),
			},
		}
	}

	return &List[T]{
		s: s,
		view: view{
			slen: len(s),
		},
	}
}

// NewN is like New, but allows specifying the index in the slice that would be
// the back of the list, and the number of elements that would be part of the
// new list. The value of length must be in the range [0, len(s)]. Note that
// you can set length and back in a way that would wrap the slice, which is ok.
// You should not hold references to s after calling this function. The
// elemetns that are not part of the new list are not zeroed.
func NewN[T any](s []T, back, length int) (*List[T], error) {
	if len(s) < back || back < 0 || (len(s) != 0 && len(s) == back) {
		return nil, ErrInvalidPosition
	}
	if len(s) < length || length < 0 {
		return nil, ErrInvalidAmount
	}
	return &List[T]{
		s: s,
		view: view{
			slen: len(s),
			len:  length,
			back: back,
		},
	}, nil
}

// view is used to provide fast and inlineable arithmetic and checks while
// still ergonomic.
type view struct {
	slen, len, back int
}

// elBound returns whether i is a valid list element position.
func (v view) elBound(i int) bool { return i < v.len && 0 <= i }

// rngBound returns whether i:j is a valid, non-wrapping range of the list.
func (v view) rngBound(i, j int) bool { return j <= v.len && i <= j && 0 <= i }

// xBound returns whether the arguments represent a valid extraction bound.
func (v view) xBound(i, n int) bool {
	return v.elBound(i) && n <= v.len && 0 <= n
}

// abs returns the slice position for a list position. The list position is
// assumed to be in bounds.
func (v view) abs(i int) int {
	i += v.back
	if i >= v.slen {
		return i - v.slen
	}
	return i
}

// fixAbs is like abs, but also fixes the list position first in case it wraps.
func (v view) fixAbs(i int) int {
	// an equivalent, more succint, more readable but non-inlineable version of
	// this is:
	//	return v.abs(fix(v.len, i))
	if v.len == 0 {
		return 0
	}
	i = fix(v.len, i)
	if i > v.slen {
		return i - v.slen
	}
	return i
}

// wraps returns whether the list wraps around the underlying slice.
func (v view) wraps() bool { return v.slen-v.back < v.len }

func (l *List[T]) alloc(min, max int) ([]T, error) {
	f := l.AllocFunc
	if f == nil {
		f = AllocDefault[T]
	}
	s, err := f(min, max)
	if err != nil {
		return nil, err
	}
	if ls := len(s); ls < min || FixAllocSize(ls, max) != ls {
		return nil, ErrInvalidAllocation
	}

	return s, nil
}

func (l *List[T]) free(newSlice []T) {
	if l.FreeFunc != nil {
		wrapClear(l.s, l.back, l.len)
		l.FreeFunc(l.s)
	}
	l.s = newSlice
	l.slen = len(newSlice)
}

// Cap returns the current total capacity.
func (l *List[T]) Cap() int { return l.slen }

// Len returns the number of elements in the list.
func (l *List[T]) Len() int { return l.len }

// Free returns the number of elements that can be added to the list without a
// new allocation.
func (l *List[T]) Free() int { return l.slen - l.len }

// Grow makes sure that the list has capacity for at least n new elements. If
// l.Free()<n, then a new slice will be allocated and the list migrated to it.
func (l *List[T]) Grow(n int) error {
	if n < 0 {
		return ErrInvalidAmount
	}

	return l.grow(n, -1)
}

// GrowRange makes sure that the list has capacity for at least min and at most
// max new elements. If l.Free() is not in the range [min, max], then a new
// slice will be allocated and the list migrated to it.
func (l *List[T]) GrowRange(min, max int) error {
	if min < 0 || max < min {
		return ErrInvalidAmount
	}

	return l.grow(min, max)
}

func (l *List[T]) grow(min, max int) error {
	if free := l.Free(); min <= free && (max < 0 || free <= max) {
		return nil
	}

	needCap := l.len + (max-min)/2
	if l.slen < needCap {
		s, err := l.alloc(min, max)
		if err != nil {
			return err
		}
		wrapCopy(l.s, s, l.back, 0, l.len)
		l.free(s)
		l.back = 0
	}

	return nil
}

// CopyTo copies at most n elements starting at index i to the given slice, and
// returns the number of copied elements. If j<i, then it wraps the list.
func (l *List[T]) CopyTo(s []T, i, n int) error {
	if !l.xBound(i, n) {
		return ErrInvalidRange
	}
	if n == 0 {
		return nil
	}
	wrapCopy(l.s, s, l.abs(i), 0, n)

	return nil
}

// Swap swaps the i-eth and j-eth elements. If either of the elements is out of
// range, it's a nop. Use SwapOK if you need to know if the elements were
// swapped.
func (l *List[T]) Swap(i, j int) {
	l.SwapOK(i, j)
}

// SwapOK swaps the i-eth and j-eth elements. If i==j, it's a nop and returns
// false. Otherwise, it returns true and swaps the elements.
func (l *List[T]) SwapOK(i, j int) (bool, error) {
	if l.elBound(i) || l.elBound(j) {
		return false, ErrInvalidPosition
	}
	if i == j {
		return false, nil
	}

	i, j = l.abs(i), l.abs(j)
	l.s[i], l.s[j] = l.s[j], l.s[i]

	return true, nil
}

// Shuffle pseudo-randomizes the order of elements using the default
// math/rand.Source.
func (l *List[T]) Shuffle() {
	rand.Shuffle(l.len, l.Swap)
}

// ShuffleN is like Shuffle, but allows specifying a specific range to be
// shuffled.
func (l *List[T]) ShuffleRange(i, j int) error {
	if !l.rngBound(i, j) {
		return ErrInvalidRange
	}
	if i == j {
		return nil
	}

	ll := *l
	ll.back = l.abs(i)
	rand.Shuffle(j-i, ll.Swap)

	return nil
}

// ShuffleRangeRand is like ShuffleRange but allows specifying an alternative
// *math/rand.Rand.
func (l *List[T]) ShuffleRangeRand(r *rand.Rand, i, j int) error {
	if !l.rngBound(i, j) {
		return ErrInvalidRange
	}
	if i == j {
		return nil
	}

	ll := *l
	ll.back = l.abs(i)
	r.Shuffle(j-i, ll.Swap)

	return nil
}

// Rotate rotates the list n elements, which can be negative. It is O(1).
func (l *List[T]) Rotate(n int) { l.back = l.fixAbs(n) }

// Val returns the element at the given position and true, if it exists.
// Otherwise, it returns the zero value and false.
func (l *List[T]) Val(i int) (v T, ok bool) {
	if l.elBound(i) {
		return l.s[l.abs(i)], true
	}
	return
}

// At returns the element at the given position, which can wrap the list from
// either side. This means that At(-1) is the same as At(l.Len()-1). It returns
// the zero value if the list is empty.
func (l *List[T]) At(i int) (v T) {
	if l.len == 0 {
		return
	}
	return l.s[l.fixAbs(i)]
}

// Back returns the element at the back of the list. If the list is empty, it
// returns the zero value. It is equivalent to l.At(0).
func (l *List[T]) Back() T { return l.At(0) }

// Front returns the element at the front of the list. If the list is empty, it
// returns the zero value. It is equivalent to l.At(-1).
func (l *List[T]) Front() T { return l.At(-1) }

// Push pushes the given element to the front of the list.
func (l *List[T]) Push(v T) { l.Replace(l.len, l.len, v) }

// Pop removes the element at the front of the list and returns it. If the list
// is empty, it returns the zero value and does nothing.
func (l *List[T]) Pop() T {
	v, ok := l.Val(l.len - 1)
	if ok {
		l.Replace(l.len-1, 1)
	}
	return v
}

// Clear removes all the elements in the list and returns the number of
// elements removed.
func (l *List[T]) Clear() int {
	cleared := wrapClear(l.s, l.back, l.len)
	l.back, l.len = 0, 0
	return cleared
}

// Insert inserts the given elements at position i.
func (l *List[T]) Insert(i int, s ...T) error {
	if !l.rngBound(i, i) {
		return ErrInvalidPosition
	}

	return l.Replace(i, i, s...)
}

// Append inserts the given elements in the front.
func (l *List[T]) Append(s ...T) error { return l.Replace(l.len, l.len, s...) }

// Delete removes the items in the given range.
func (l *List[T]) Delete(i, j int) error { return l.Replace(i, j) }

// Replace replaces the elements in the given range with the provided ones.
func (l *List[T]) Replace(i, j int, s ...T) error {
	if !l.rngBound(i, j) {
		return ErrInvalidRange
	}

	n := min(l.len-i, j-i)
	if n < 1 && len(s) == 0 {
		return nil // nothing to delete, nothing to insert
	}

	ls := len(s)
	frontEls := l.len - i - n
	needCap := i + ls + frontEls

	if l.slen < needCap {
		// need to move to another slice
		ss, err := l.alloc(needCap, -1)
		if err != nil {
			return err
		}
		wrapCopy(l.s, ss, l.back, 0, i)
		wrapCopy(s, ss, 0, i, ls)
		wrapCopy(l.s, ss, l.fixAbs(-frontEls), i+ls, frontEls)
		l.free(ss)
		l.back, l.len = 0, needCap

		return nil
	}

	// balloon is the leftover range that we need to fix between the deleted
	// elements and the inserted elements
	balloonOffset := n - ls // deleted - inserted
	balloonStart := l.back

	// move front or back to accommodate for the balloon
	if i < frontEls {
		// less elements to copy on the back
		selfWrapCopy(l.s, l.back, i, balloonOffset)
		l.back = l.fixAbs(balloonOffset)
	} else {
		// less elements to copy on the front
		selfWrapCopy(l.s, i+n, frontEls, balloonOffset)
		balloonStart = l.fixAbs(l.len - balloonOffset)
	}

	if 0 < balloonOffset {
		wrapClear(l.s, balloonStart, balloonOffset)
	}

	wrapCopy(s, l.s, 0, l.abs(i), ls)
	l.len = needCap

	return nil
}

// MarshalJSON marshals the list as a JSON Array.
func (l List[T]) MarshalJSON() ([]byte, error) {
	if l.len == 0 {
		return []byte{'[', ']'}, nil
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)

	b.WriteByte('[')
	for i := range l.len {
		if 0 < i {
			b.WriteByte(',')
		}
		if err := enc.Encode(l.At(i)); err != nil {
			return nil, fmt.Errorf("encode list element %d: %w", i, err)
		}
		// remove the annoying extra newline that Encode adds
		if lastBytePos := b.Len() - 1; b.Bytes()[lastBytePos] == '\n' {
			b.Truncate(lastBytePos)
		}
	}
	b.WriteByte(']')

	return b.Bytes(), nil
}

// UnmarshalJSON clears the list and reads a JSON Array as a list of elements.
func (l *List[T]) UnmarshalJSON(b []byte) error {
	// release the current slice, since json.Unmarshal will make its own
	// allocation
	l.back = 0
	l.free(nil)
	if err := json.Unmarshal(b, &l.s); err != nil {
		// if T is or contain a pointer type and some items were decoded before
		// returning the error, then we need to clear the slice to remove those
		// unnecessary references
		clear(l.s)

		return err
	}

	l.slen = len(l.s)
	l.len = l.slen
	// take advantage of the whole capacity of the slice allocated by
	// json.Unmarshal
	l.s = l.s[:cap(l.s)]

	return nil
}

// StringRange is like String but only for the given range. If j<i, then it
// wraps the list.
func (l *List[T]) StringRange(i, n int) (string, error) {
	if !l.xBound(i, n) {
		return "", ErrInvalidRange
	}
	if n == 0 {
		return "[]", nil
	}

	f := l.StringFunc
	if f == nil {
		f = toString[T]
	}

	var b strings.Builder
	b.WriteByte('[')
	for j := range n {
		if j > 0 {
			b.WriteString(", ")
		}
		b.WriteString(f(l.At(i + j)))
	}
	b.WriteByte(']')

	return b.String(), nil
}

// String converts a list into a human-readable form. You can control how
// individual elements are printed by setting the StringFunc member of the
// list.
func (l *List[T]) String() string {
	s, _ := l.StringRange(0, l.len)
	return s
}

func toString[T any](v T) string {
	return fmt.Sprintf("%v", v)
}

// selfWrapCopy copies the elements in [i, i+n) m positions to either left (if
// m<0) or right (if m>0). If n<1, n>=len(s) or m>=len(s) it's a nop.
func selfWrapCopy[S ~[]T, T any](s S, i, n, m int) {
	l := len(s)
	if l == 0 || n < 1 || l <= n || l <= m || m == 0 {
		return
	}

	i = fix(l, i)
	targetI := fix(l, i+m)

	if targetI < i {
		// copy left part first
		wrapCopy(s, s, i, targetI, n)
	} else {
		// copy right part first
		var copied int
		for j, targetJ := i+n, targetI+n; 0 < n; n -= copied {
			if targetJ -= copied; targetJ < 1 {
				targetJ += l
			}
			if j -= copied; j < 1 {
				j += l
			}
			copied = min(j, targetJ, n)
			copy(s[targetJ-copied:], s[j-copied:j])
		}
	}
}

// wrapCopy copies n elements from s1 starting at i1 into s2 starting at i2,
// wrapping either of them if needed. It returns the number of elements copied,
// since it won't copy more elements than the length of either slice. The
// slices are assumed to be different, if you need to copy into the same slice,
// use selfWrapCopy instead.
func wrapCopy[S ~[]T, T any](s1, s2 S, i1, i2, n int) int {
	l1, l2 := len(s1), len(s2)
	if l1 == 0 || l2 == 0 || n < 1 {
		return 0
	}
	i1, i2 = fix(l1, i1), fix(l2, i2)
	n = min(n, l1, l2)
	for left := n; 0 < left; {
		copied := min(left, l1-i1, l2-i2)
		copy(s1[i1:i1+copied], s2[i2:i2+copied])
		i1 = fix(l1, i1+copied)
		i2 = fix(l2, i2+copied)
		left -= copied
	}
	return n
}

// wrapClear clears at most n elements with respect to index i, which can wrap
// the slice either ways. If n<0, it means "the n elements before i". Returns
// the number of cleared elements.
func wrapClear[S ~[]T, T any](s S, i, n int) int {
	l := len(s)
	if l == 0 || n == 0 {
		return 0
	}

	if n < 0 {
		i += n
		n *= -1
	}

	if l <= n {
		clear(s)
		return l
	}

	for cleared, left := 0, n; 0 < left; {
		i = fix(l, i+cleared)
		left -= cleared
		j := min(i+left, l)
		clear(s[i:j])
		cleared += j - i
	}

	return n
}

// fix removes redundancy from an element position i of a list or slice with
// length elements, where i could wrap the length either positive or
// negatively. The result will be zero if length is zero, or in the range [0,
// length) otherwise. The value of length is expected to be >=0.
func fix(length, i int) int {
	if length == 0 {
		return 0
	}

	// fast cases first, expected to be the most common
	if 0 <= i {
		if i < length {
			return i
		}
		if i < 2*length {
			return i - length
		}
	} else if -length < i {
		return i + length
	}

	// fallback to more complex arithmetic
	return ((i % length) + length) % length
}
