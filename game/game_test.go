package game

import (
	"testing"

	"github.com/facu/bingo-back/card"
)

// a fixed 90-ball card used across tests (15 numbers, 5 per row)
var testCard = card.Card{
	{0, 10, 0, 32, 42, 0, 61, 0, 80},
	{0, 0, 24, 35, 0, 58, 0, 71, 89},
	{2, 18, 0, 0, 47, 59, 0, 78, 0},
}

func TestNewBag(t *testing.T) {
	bag := NewBag()
	if len(bag) != 90 {
		t.Fatalf("bag length = %d, want 90", len(bag))
	}
	seen := make(map[int]bool, 90)
	for _, n := range bag {
		if n < 1 || n > 90 {
			t.Fatalf("bag contains out-of-range number %d", n)
		}
		if seen[n] {
			t.Fatalf("bag contains duplicate %d", n)
		}
		seen[n] = true
	}
	if len(seen) != 90 {
		t.Fatalf("bag covers %d distinct numbers, want 90", len(seen))
	}

	// two consecutive bags should not come out in the same order
	// (probability of a match is 1/90!, effectively zero)
	other := NewBag()
	same := true
	for i := range bag {
		if bag[i] != other[i] {
			same = false
			break
		}
	}
	if same {
		t.Fatal("two consecutive bags came out identical; shuffle looks broken")
	}
}

func TestCardComplete(t *testing.T) {
	all := []int{10, 32, 42, 61, 80, 24, 35, 58, 71, 89, 2, 18, 47, 59, 78}

	if !CardComplete(testCard, all) {
		t.Fatal("CardComplete = false with all 15 numbers drawn, want true")
	}
	if CardComplete(testCard, all[:len(all)-1]) {
		t.Fatal("CardComplete = true with one number missing, want false")
	}
	if CardComplete(testCard, nil) {
		t.Fatal("CardComplete = true with empty drawn, want false")
	}
}

func TestHasLine(t *testing.T) {
	fullRow := []int{24, 35, 58, 71, 89} // middle row of testCard
	if !HasLine(testCard, fullRow) {
		t.Fatal("HasLine = false with a full row drawn, want true")
	}

	// 4 of each row, no row complete
	partial := []int{10, 32, 42, 61, 24, 35, 58, 71, 2, 18, 47, 59}
	if HasLine(testCard, partial) {
		t.Fatal("HasLine = true with no complete row, want false")
	}
	if HasLine(testCard, nil) {
		t.Fatal("HasLine = true with empty drawn, want false")
	}
}