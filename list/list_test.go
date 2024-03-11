package list

import (
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func assertState[T any](t *testing.T, l *List[T], free int, values []T) {
	t.Helper()

	require.NotNil(t, l)
	assert.Equal(t, l.Len()+free, l.Cap())
	assert.Equal(t, free, l.Free())
	require.Equal(t, len(values), l.Len())
	for i := range l.Len() {
		v, ok := l.Val(i)
		require.True(t, ok)
		require.Equal(t, values[i], v, "element %d", i)
	}
}

func TestNew(t *testing.T) {
	t.Parallel()
	s := []int{1, 2, 3, 4}

	l := New[int](nil, false)
	assertState(t, l, 0, nil)

	l = New[int](nil, true)
	assertState(t, l, 0, nil)

	l = New[int](s, false)
	assertState(t, l, len(s), nil)

	l = New[int](s, true)
	assertState(t, l, 0, s)
}

func TestNewT(t *testing.T) {
	t.Parallel()
	s := []int{1, 2, 3, 4, 5}

	testCases := []struct {
		input  []int
		back   int
		length int

		fails    bool
		expected []int
	}{
		{input: nil, back: -1, length: -1, fails: true},
		{input: nil, back: -1, length: 0, fails: true},
		{input: nil, back: 0, length: -1, fails: true},
		{input: nil, back: 0, length: 1, fails: true},
		{input: nil, back: 1, length: 0, fails: true},
		{input: nil, back: 1, length: 1, fails: true},
		{input: s, back: -1, length: 0, fails: true},
		{input: s, back: 0, length: -1, fails: true},
		{input: s, back: 0, length: len(s) + 1, fails: true},
		{input: s, back: len(s), length: 0, fails: true},

		// next index is 12
		{input: nil, back: 0, length: 0},
		{input: s, back: 0, length: 0},
		{input: s, back: 0, length: 1, expected: s[:1]},
		{input: s, back: 0, length: 3, expected: s[:3]},
		{input: s, back: 0, length: len(s), expected: s},
		{input: s, back: 1, length: 1, expected: s[1:2]},
		{input: s, back: 1, length: 2, expected: s[1:3]},
		{input: s, back: len(s) - 1, length: 1, expected: s[len(s)-1:]},
		{input: s, back: 3, length: 4, expected: []int{4, 5, 1, 2}},
		{input: s, back: 3, length: 5, expected: []int{4, 5, 1, 2, 3}},
		{input: s, back: 3, length: 5, expected: []int{4, 5, 1, 2, 3}},
		{input: s, back: 4, length: 0},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("test index #%d", i), func(t *testing.T) {
			l, err := NewN[int](tc.input, tc.back, tc.length)
			if tc.fails {
				require.Error(t, err)
				require.Nil(t, l)

			} else {
				require.NoError(t, err)
				assertState(t, l, len(tc.input)-tc.length, tc.expected)
			}
		})
	}
}

func TestList_JSON(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		// only fill in output if it differs from input and the test should not
		// fail, otherwise we expect the same as input
		input, output string
		fails         bool
	}{
		{input: "", fails: true},
		{input: ".", fails: true},
		{input: "1", fails: true},
		{input: `"a"`, fails: true},
		{input: `{}`, fails: true},

		{input: "null", output: "[]"},
		{input: "[]"},
		{input: "[1]"},
		{input: "[1, 2, 3, 4, 5, 6, 7, 8, 9]"},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("test index #%d", i), func(t *testing.T) {
			var l List[int]
			err := json.Unmarshal([]byte(tc.input), &l)
			if tc.fails {
				assert.Error(t, err)
				assert.Equal(t, 0, l.Len())

			} else {
				// marshal both pointer non-pointer types should succeed and
				// produce same output
				b1, err := json.Marshal(l)
				require.NoError(t, err)
				b2, err := json.Marshal(&l)
				require.NoError(t, err)
				assert.Equal(t, b1, b2)
				assert.JSONEq(t, cmp.Or(tc.output, tc.input), string(b1))
			}
		})
	}
}

func TestList_StringRange(t *testing.T) {
	t.Parallel()

	testData := []int{1, 2, 3, 4, 5}
	l := New(testData, true)

	testCases := []struct {
		i, n     int
		expected string
		err      error
	}{
		{i: -1, n: 0, err: ErrInvalidRange},
		{i: 0, n: -1, err: ErrInvalidRange},
		{i: l.Len(), n: 0, err: ErrInvalidRange},
		{i: 0, n: l.Len() + 1, err: ErrInvalidRange},

		{i: 0, n: 0, expected: "[]"},
		{i: 0, n: l.Len(), expected: "[1, 2, 3, 4, 5]"},
		{i: l.Len() - 1, n: l.Len(), expected: "[5, 1, 2, 3, 4]"},
		{i: 3, n: 1, expected: "[4]"},
		{i: 3, n: 0, expected: "[]"},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("test index #%d", i), func(t *testing.T) {
			result, err := l.StringRange(tc.i, tc.n)
			if tc.err != nil {
				require.Empty(t, result)
				require.ErrorIs(t, err, tc.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expected, result)
			}
		})
	}
}

func TestView_abs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		slen, i, result int
	}{
		{slen: 0, i: 0, result: 0},

		// TODO: add cases
	}

	var v view
	for i, tc := range testCases {
		t.Run(fmt.Sprintf("test index #%d", i), func(t *testing.T) {
			v.slen = tc.slen
			result := v.abs(tc.i)
			assert.Equal(t, tc.result, result)
		})
	}
}

func TestView_fixAbs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		slen, len, i, result int
	}{
		{slen: 0, len: 0, i: 0, result: 0},

		// TODO: add cases
	}

	var v view
	for i, tc := range testCases {
		t.Run(fmt.Sprintf("test index #%d", i), func(t *testing.T) {
			v.slen, v.len = tc.slen, tc.len
			result := v.fixAbs(tc.i)
			assert.Equal(t, tc.result, result)
		})
	}
}

func TestView_wraps(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		slen, len, back int
		result          bool
	}{
		{slen: 0, len: 0, back: 0, result: false},

		// TODO: add cases
	}

	var v view
	for i, tc := range testCases {
		t.Run(fmt.Sprintf("test index #%d", i), func(t *testing.T) {
			v = view{
				slen: tc.slen,
				len:  tc.len,
				back: tc.back,
			}
			result := v.wraps()
			assert.Equal(t, tc.result, result)
		})
	}
}

func TestView_elBound(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		len, i int
		result bool
	}{
		{len: 0, i: -1, result: false},
		{len: 0, i: 0, result: false},
		{len: 0, i: 1, result: false},
		{len: 0, i: 10, result: false},

		{len: 10, i: -1, result: false},
		{len: 10, i: 0, result: true},
		{len: 10, i: 1, result: true},
		{len: 10, i: 9, result: true},
		{len: 10, i: 10, result: false},
	}

	var v view
	for i, tc := range testCases {
		t.Run(fmt.Sprintf("test index #%d", i), func(t *testing.T) {
			v.len = tc.len
			result := v.elBound(tc.i)
			assert.Equal(t, tc.result, result)
		})
	}
}

func TestView_rngBound(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		len, i, j int
		result    bool
	}{
		{len: 10, i: -1, j: 1, result: false},
		{len: 10, i: 0, j: -1, result: false},
		{len: 10, i: -1, j: -1, result: false},

		{len: 0, i: 0, j: 0, result: true},
		{len: 0, i: 0, j: 1, result: false},
		{len: 0, i: 1, j: 1, result: false},

		{len: 5, i: 0, j: 0, result: true},
		{len: 5, i: 0, j: 4, result: true},
		{len: 5, i: 0, j: 5, result: true},
		{len: 5, i: 0, j: 6, result: false},

		{len: 5, i: 4, j: 0, result: false},
		{len: 5, i: 4, j: 4, result: true},
		{len: 5, i: 4, j: 5, result: true},
		{len: 5, i: 4, j: 6, result: false},

		{len: 5, i: 5, j: 0, result: false},
		{len: 5, i: 5, j: 4, result: false},
		{len: 5, i: 5, j: 5, result: true},
		{len: 5, i: 5, j: 6, result: false},

		{len: 5, i: 6, j: 0, result: false},
		{len: 5, i: 6, j: 4, result: false},
		{len: 5, i: 6, j: 5, result: false},
		{len: 5, i: 6, j: 6, result: false},
	}

	var v view
	for i, tc := range testCases {
		t.Run(fmt.Sprintf("test index #%d", i), func(t *testing.T) {
			v.len = tc.len
			result := v.rngBound(tc.i, tc.j)
			assert.Equal(t, tc.result, result)
		})
	}
}

func TestSelfWrapCopy(t *testing.T) {
	t.Parallel()

	const testDataLen = 5
	testData := make([]int, testDataLen)
	for i := range testDataLen {
		testData[i] = i + 1
	}

	testCases := []struct {
		i, n, m  int
		expected []int
	}{
		{i: 0, n: 0, m: 0, expected: []int{1, 2, 3, 4, 5}},

		// TODO: add cases
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("test index #%d", i), func(t *testing.T) {
			require.Len(t, tc.expected, len(testData)) // dumb-proof
			expected := slices.Clone(testData)
			selfWrapCopy(expected, tc.i, tc.n, tc.m)
			assert.Equal(t, tc.expected, expected)
		})
	}
}

func TestWrapCopy(t *testing.T) {
	t.Parallel()

	const testDataLen = 5
	s1, s2 := make([]int, testDataLen), make([]int, testDataLen)
	for i := range testDataLen {
		s1[i], s2[i] = 1, 2
	}

	testCases := []struct {
		i1, i2, n, copied int
		expected          []int
	}{
		{i1: 0, i2: 0, n: 0, copied: 0, expected: []int{2, 2, 2, 2, 2}},

		// TODO: add cases
	}

	witness := slices.Clone(s1)
	require.Equal(t, len(s1), len(s2)) // dumb-proof
	for i, tc := range testCases {
		t.Run(fmt.Sprintf("test index #%d", i), func(t *testing.T) {
			require.Len(t, tc.expected, len(s1)) // dumb-proof
			expected := slices.Clone(s2)
			copied := wrapCopy(s1, expected, tc.i1, tc.i2, tc.n)
			assert.Equal(t, tc.copied, copied)
			assert.Equal(t, tc.expected, expected)
			require.Equal(t, witness, s1) // dumb-proof
		})
	}
}

func TestWrapClear(t *testing.T) {
	t.Parallel()

	const testDataLen = 5
	testData := make([]int, testDataLen)
	for i := range testDataLen {
		testData[i] = 1
	}

	testCases := []struct {
		i, n, cleared int
		expected      []int
	}{
		{i: 0, n: 0, cleared: 0, expected: []int{1, 1, 1, 1, 1}},

		// TODO: add cases
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("test index #%d", i), func(t *testing.T) {
			require.Len(t, tc.expected, len(testData)) // dumb-proof
			expected := slices.Clone(testData)
			cleared := wrapClear(expected, tc.i, tc.n)
			assert.Equal(t, tc.cleared, cleared)
			assert.Equal(t, tc.expected, expected)
		})
	}
}

func TestFix(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		length, i, result int
	}{
		{length: 0, i: -11, result: 0},
		{length: 0, i: -10, result: 0},
		{length: 0, i: -9, result: 0},
		{length: 0, i: -6, result: 0},
		{length: 0, i: -5, result: 0},
		{length: 0, i: -4, result: 0},
		{length: 0, i: -1, result: 0},
		{length: 0, i: 0, result: 0},
		{length: 0, i: 1, result: 0},
		{length: 0, i: 4, result: 0},
		{length: 0, i: 5, result: 0},
		{length: 0, i: 6, result: 0},
		{length: 0, i: 9, result: 0},
		{length: 0, i: 10, result: 0},
		{length: 0, i: 11, result: 0},

		{length: 5, i: -11, result: 4},
		{length: 5, i: -10, result: 0},
		{length: 5, i: -9, result: 1},
		{length: 5, i: -6, result: 4},
		{length: 5, i: -5, result: 0},
		{length: 5, i: -4, result: 1},
		{length: 5, i: -1, result: 4},
		{length: 5, i: 0, result: 0},
		{length: 5, i: 1, result: 1},
		{length: 5, i: 4, result: 4},
		{length: 5, i: 5, result: 0},
		{length: 5, i: 6, result: 1},
		{length: 5, i: 9, result: 4},
		{length: 5, i: 10, result: 0},
		{length: 5, i: 11, result: 1},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("test index #%d", i), func(t *testing.T) {
			result := fix(tc.length, tc.i)
			assert.Equal(t, tc.result, result)
		})
	}
}
