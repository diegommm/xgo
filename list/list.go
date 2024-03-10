package list

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// AllocFunc is a function that allocates a new slice that needs to hold at
// least needCap elements. The full capacity of the returned slice will not be
// used. Instead, only the already available elements (i.e. from zero to its
// current length). If the returned slice has a length less than needCap then a
// panic will occur. Implementations should not hold references to the returned
// slice.
type AllocFunc[T any] func(curElems, curCap, needCap int) []T

// AllocDouble is an AllocFunc[T] that allocates a new slice double the size
// of needCap.
func AllocDouble[T any](curElems, curCap, needCap int) []T {
	return make([]T, needCap*2)
}

// List is a slice-based list container, indexed from its back to its front
// starting with zero. Not safe for concurrent use. It zeroes elements that are
// removed from the list.
type List[T any] struct {
	// AllocFunc allows customizing allocation of a new slice when more space
	// is needed. If nil, AllocDouble is used.
	AllocFunc[T]

	// FreeFunc will be called, if set, with the old slice after migrating to a
	// newly allocated slice. This is useful if you want to put a slice back
	// into a pool or do something else with it, but all the items that the
	// list had used will have been zeroed out.
	FreeFunc func([]T)

	// StringFunc allows customizing how elements are converted to string when
	// printing the list. The default is using fmt.Sprintf("%v", element).
	StringFunc func(T) string

	back, len int
	s         []T
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
			s:   s,
			len: len(s),
		}
	}
	return &List[T]{
		s: s,
	}
}

// NewN is like New, but allows specifying the index in the slice that would be
// the back of the list, and the number of elements that would be part of the
// new list. The value of length must be in the range [0, len(s)]. Note that
// you can set length and back in a way that would wrap the slice, which is ok.
// You should not hold references to s after calling this function. The
// elemetns that are not part of the new list are not zeroed.
func NewN[T any](s []T, back, length int) (*List[T], error) {
	if !bound(len(s), back) {
		return nil, fmt.Errorf("index out of range: %d", back)
	}
	if !bound(len(s)+1, length) {
		return nil, fmt.Errorf("amount of elements out of range: %d", back)
	}
	return &List[T]{
		back: back,
		len:  length,
		s:    s,
	}, nil
}

// wraps returns whether the virtual list wraps around the underlying slice.
func (l *List[T]) wraps() bool { return l.len > len(l.s)-l.back }

// absEl returns the absolute position in the underlying slice of the given
// virtual element, which can be negative and possible wrap the list. The
// result is in the range [0, len(l.s)).
func (l *List[T]) absEl(i int) int {
	return absEl(len(l.s), l.back+absEl(l.len, i))
}

// numEls should be used to fix all input arguments that represent an amount of
// elements in the current list before using them for arithmetic.
func (l *List[T]) numEls(n int) int {
	if n > 0 {
		return n % l.len
	}
	return 0
}

func (l *List[T]) alloc(needCap int) []T {
	f := l.AllocFunc
	if f == nil {
		f = AllocDouble[T]
	}
	s := f(l.len, len(l.s), needCap)
	if len(s) < needCap {
		panic("could not allocate enough elements for list")
	}

	return s
}

func (l *List[T]) free() {
	if l.FreeFunc != nil {
		wrapClear(l.s, l.back, l.len)
		l.FreeFunc(l.s)
	}
}

// Cap returns the current total capacity.
func (l *List[T]) Cap() int { return len(l.s) }

// Len returns the number of elements in the list.
func (l *List[T]) Len() int { return l.len }

// Free returns the number of elements that can be added to the list without a
// new allocation.
func (l *List[T]) Free() int { return len(l.s) - l.len }

// Grow increases the list capacity, if necessary, to guarantee space for
// another n elements. It returns the number of elements that can be added
// withhout a new allocation, which will be greater or equal to n.
func (l *List[T]) Grow(n int) int {
	if n < 1 {
		return l.Free()
	}
	needCap := l.len + n

	if len(l.s) < needCap {
		s := l.alloc(needCap)
		wrapCopy(l.s, s, l.back, 0, l.len)
		l.free()
		l.back, l.s = 0, s
	}

	return l.Free()
}

// CopyTo copies at most n elements starting at index i to the given slice, and
// returns the number of copied elements.
func (l *List[T]) CopyTo(i, n int, s []T) int {
	if n = l.numEls(n); n < 1 || !bound(l.len, i) || len(s) == 0 {
		return 0
	}
	n = min(n, l.len, len(s))
	return wrapCopy(l.s, s, l.absEl(i), 0, n)
}

// Swap swaps the i-eth and j-eth elements. If either of the elements is out of
// range, it's a nop. Use SwapOK if you need to know if the elements were
// swapped.
func (l *List[T]) Swap(i, j int) {
	l.SwapOK(i, j)
}

// SwapOK swaps the i-eth and j-eth elements. If either of the elements is out
// of range, it's a nop and returns false. Otherwise, it returns true.
func (l *List[T]) SwapOK(i, j int) bool {
	if i == j || !bound(l.len, i) || !bound(l.len, j) {
		return false
	}
	i, j = l.absEl(i), l.absEl(j)
	l.s[i], l.s[j] = l.s[j], l.s[i]
	return true
}

// Val returns the element at the given position and true, if it exists.
// Otherwise, it returns the zero value and false.
func (l *List[T]) Val(i int) (v T, ok bool) {
	if bound(l.len, i) {
		return l.s[l.absEl(i)], true
	}
	return
}

// At returns the element at the given position, if it exists. Otherwise it
// returns the zero value.
func (l *List[T]) At(i int) T {
	v, _ := l.Val(i)
	return v
}

// Back returns the element in the back, if the list is not empty. Otherwise it
// returns the zero value and false.
func (l *List[T]) Back() (v T, ok bool) { return l.Val(0) }

// Front returns the element in the front, if the list is not empty. Otherwise
// it returns the zero value and false.
func (l *List[T]) Front() (v T, ok bool) { return l.Val(l.len - 1) }

// PopAt removes the element at the given position and returns it with true, if
// it exists. Otherwise, it returns the zero value and false.
func (l *List[T]) PopAt(i int) (v T, ok bool) {
	if v, ok = l.Val(i); ok {
		l.DeleteN(i, 1)
	}
	return
}

// PopBack removes the element in the back and returns it with true, if the
// list is not empty. Otherwise it returns the zero value and false.
func (l *List[T]) PopBack() (v T, ok bool) { return l.PopAt(0) }

// PopFront removes the element in the front and returns it with true, if the
// list is not empty. Otherwise it returns the zero value and false.
func (l *List[T]) PopFront() (v T, ok bool) { return l.PopAt(l.len - 1) }

// Clear removes all the elements in the list.
func (l *List[T]) Clear() int {
	if l.len == 0 {
		return 0
	}
	cleared := wrapClear(l.s, l.back, l.len)
	l.back, l.len = 0, 0
	return cleared
}

// Delete removes the elements in the given range and returns the number of
// removed elements.
func (l *List[T]) Delete(i, j int) int {
	if !boundRange(l.len, i, j) || i == j {
		return 0
	}
	oldLen := l.len
	l.ReplaceN(i, j-i)
	return oldLen - l.len
}

// DeleteN removes at most n elements starting at position i, and returns the
// number of removed elements.
func (l *List[T]) DeleteN(i, n int) int { return l.Delete(i, i+n) }

// DeleteBack removes at most n elements from the list back.
func (l *List[T]) DeleteBack(n int) int { return l.Delete(0, n) }

// DeleteFront removes at most n elements from the list front.
func (l *List[T]) DeleteFront(n int) int {
	n = l.numEls(n)
	return l.Delete(l.len-n, n)
}

// Insert inserts the elements of s at position i, and returns the new length.
func (l *List[T]) Insert(i int, s ...T) int { return l.ReplaceN(i, 0, s...) }

// InsertBack is the same as Insert(0, s...).
func (l *List[T]) InsertBack(s ...T) int { return l.ReplaceN(0, 0, s...) }

// InsertFront is the same as Insert(l.len, s...).
func (l *List[T]) InsertFront(s ...T) int { return l.ReplaceN(l.len, 0, s...) }

// Replace replaces the elements in the given range with the provided ones, and
// returns the new length.
func (l *List[T]) Replace(i, j int, s ...T) int {
	return l.ReplaceN(i, j-i, s...)
}

// ReplaceN replaces the n elements after index i with the provided elements,
// and returns the new length.
func (l *List[T]) ReplaceN(i, n int, s ...T) int {
	if !bound(l.len+1, i) {
		return 0
	}

	n = l.numEls(n)
	n = min(l.len-i, n)
	if n < 1 && len(s) == 0 {
		return 0 // nothing to delete, nothing to insert
	}

	ls := len(s)
	frontEls := l.len - i - n
	needCap := i + ls + frontEls

	if len(l.s) < needCap {
		// need to move to another slice
		ss := l.alloc(needCap)
		wrapCopy(l.s, ss, l.back, 0, i)
		wrapCopy(s, ss, 0, i, ls)
		wrapCopy(l.s, ss, l.absEl(-frontEls), i+ls, frontEls)
		l.free()
		l.back, l.len, l.s = 0, needCap, ss

		return l.len
	}

	// balloon is the leftover range that we need to fix between the deleted
	// elements and the inserted elements
	balloonOffset := n - ls // deleted - inserted
	balloonStart := l.back

	// move front or back to accommodate for the balloon
	if i < frontEls {
		// less elements to copy on the back
		selfWrapCopy(l.s, l.back, i, balloonOffset)
		l.back = l.absEl(balloonOffset)

	} else {
		// less elements to copy on the front
		selfWrapCopy(l.s, i+n, frontEls, balloonOffset)
		balloonStart = l.absEl(l.len - balloonOffset)
	}

	if balloonOffset > 0 {
		wrapClear(l.s, balloonStart, balloonOffset)
	}

	wrapCopy(s, l.s, 0, l.absEl(i), ls)
	l.len = needCap

	return l.len
}

// MarshalJSON marshals the list as a JSON Array.
func (l *List[T]) MarshalJSON() ([]byte, error) {
	if l.len == 0 {
		return []byte{'[', ']'}, nil
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)

	b.WriteByte('[')
	for i := 0; i < l.len; i++ {
		if i > 0 {
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
	l.Clear()
	dec := json.NewDecoder(bytes.NewReader(b))

	if err := expectJSONDelim(dec, '['); err != nil {
		return err
	}

	for dec.More() {
		var t T
		if err := dec.Decode(&t); err != nil {
			return fmt.Errorf("decode list element %d: %w", l.Len(), err)
		}
		l.InsertFront(t)
	}

	if err := expectJSONDelim(dec, ']'); err != nil {
		return err
	}

	return nil
}

func expectJSONDelim(dec *json.Decoder, d json.Delim) error {
	t, err := dec.Token()
	if err == io.EOF {
		return io.ErrUnexpectedEOF
	}
	if err != nil {
		return fmt.Errorf("read token: %w", err)
	}

	dd, ok := t.(json.Delim)
	if !ok || dd != d {
		return fmt.Errorf("unexpected token: %v", t)
	}

	return nil
}

// StringN is like String, but only prints n elements with respect to position
// i. The value of n can be negative.
func (l *List[T]) StringN(i, n int) string {
	if n = l.numEls(n); n < 1 || !bound(l.len, i) {
		return ""
	}
	f := l.StringFunc
	if f == nil {
		f = toString[T]
	}

	var b strings.Builder
	b.WriteByte('[')
	for ; i < i+n; i++ {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(f(l.s[l.absEl(i)]))
	}
	b.WriteByte(']')

	return b.String()
}

// StringRange is the same as StringN(i, j-i).
func (l *List[T]) StringRange(i, j int) string {
	return l.StringN(i, j-i)
}

// String converts a list into a human-readable form. You can control how
// individual elements are printed by setting the StringFunc member of the
// list.
func (l *List[T]) String() string {
	return l.StringN(0, l.len)
}

func toString[T any](v T) string {
	return fmt.Sprintf("%v", v)
}

// selfWrapCopy copies the elements in [i, i+n) m positions to either left (if
// m<0) or right (if m>0). If n<1, n>=len(s) or m>=len(s) it's a nop.
func selfWrapCopy[S ~[]T, T any](s S, i, n, m int) {
	l := len(s)
	if n < 1 || n >= l || m >= l {
		return
	}

	i = absEl(l, i)
	targetI := absEl(l, i+m)
	if targetI < i {
		// copy left part first
		wrapCopy(s, s, i, targetI, n)

	} else {
		// copy right part first
		var copied int
		for j, targetJ := i+n, targetI+n; n > 0; n -= copied {
			if targetJ = (targetJ - copied) % l; targetJ == 0 {
				targetJ = l
			}
			if j = (j - copied) % l; j == 0 {
				j = l
			}
			copied = min(j, targetJ, n)
			copy(s[targetJ-copied:], s[j-copied:j])
		}
	}
}

// wrapCopy copies n elements from s1 starting at i1 into s2 starting at i2,
// wrapping either of them if needed. It returns the number of elements copied,
// since it won't copy more elements than the lenght of either slice. The
// slices are assumed to be different, if you need to copy into the same slice,
// use selfWrapCopy instead.
func wrapCopy[S ~[]T, T any](s1, s2 S, i1, i2, n int) int {
	if n < 1 {
		return 0
	}
	l1, l2 := len(s1), len(s2)
	i1, i2 = absEl(l1, i1), absEl(l2, i2)
	n = min(n, l1, l2)
	for left := n; left > 0; {
		copied := min(left, l1-i1, l2-i2)
		copy(s1[i1:i1+copied], s2[i2:i2+copied])
		i1 = absEl(l1, i1+copied)
		i2 = absEl(l2, i2+copied)
		left -= copied
	}
	return n
}

// wrapClear clears the elements in [i, i+n). Returns the number of cleared
// elements.
func wrapClear[S ~[]T, T any](s S, i, n int) int {
	if n < 0 {
		i += n
		n *= -1
	}

	l := len(s)
	if n >= l {
		clear(s)
		return l
	}

	i = absEl(l, i)
	for cleared, left := 0, n; left > 0; {
		j := min(i+left, l)
		clear(s[i:j])
		cleared += j - i
		i = absEl(l, i+cleared)
		left -= cleared
	}
	return n
}

// absEl removes redundancy from a zero-based position i of an element list
// with l elements, where i could wrap the list and be negative. The result
// will be in the range [0, l).
func absEl(l, i int) int { return ((i % l) + l) % l }

// bound returns whether i is in the range [0, l).
func bound(l int, i int) bool {
	return l > 0 && i >= 0 && i < l
}

// boundRange return whether i is in the range [0, l), j in [0, l] and i<=j.
func boundRange(l, i, j int) bool {
	return bound(l, i) && bound(l+1, j) && i <= j
}
