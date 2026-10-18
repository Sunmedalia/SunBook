package ui

import (
	"fmt"
	"testing"
)

func TestLineNumberMatchesSprintf(t *testing.T) {
	for _, width := range []int{-5, -1, 0, 1, 2, 3, 7, 63, 64, 65} {
		for _, n := range []int{0, 1, 9, 10, 99, 12345, 1 << 40, -3} {
			if got, want := lineNumber(n, width), fmt.Sprintf("%*d", width, n); got != want {
				t.Fatalf("lineNumber(%d, %d) = %q, want %q", n, width, got, want)
			}
		}
	}
}

func TestMatchingBrackets(t *testing.T) {
	s := "f(a[1], {b}) )"
	for _, tc := range []struct{ head, a, b int }{{1, 1, 11}, {2, 1, 11}, {12, 11, 1}, {4, 3, 5}, {8, 8, 10}, {0, -1, -1}, {14, -1, -1}, {6, 5, 3}} {
		if a, b := matchingBrackets(s, tc.head); a != tc.a || b != tc.b {
			t.Fatalf("head %d: %d %d, want %d %d", tc.head, a, b, tc.a, tc.b)
		}
	}
}
