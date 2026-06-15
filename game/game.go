package game

import (
	"crypto/rand"
	"math/big"

	"github.com/facu/bingo-back/card"
)

// NewBag returns a shuffled slice containing the numbers 1..90.
func NewBag() []int {
	bag := make([]int, 90)
	for i := 0; i < 90; i++ {
		bag[i] = i + 1
	}
	// Fisher-Yates shuffling with crypto/rand
	for i := 89; i > 0; i-- {
		randIndex, _ := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		j := int(randIndex.Int64())
		bag[i], bag[j] = bag[j], bag[i]
	}
	return bag
}

// drawnSet converts a slice of ints to a map for O(1) look-up.
func drawnSet(s []int) map[int]bool {
	m := make(map[int]bool, len(s))
	for _, n := range s {
		m[n] = true
	}
	return m
}

// CardComplete reports whether all 15 numbers of a card are present in drawn.
func CardComplete(c card.Card, drawn []int) bool {
	d := drawnSet(drawn)
	for r := 0; r < 3; r++ {
		for col := 0; col < 9; col++ {
			n := c[r][col]
			if n == 0 {
				continue
			}
			if !d[n] {
				return false
			}
		}
	}
	return true
}

// HasLine reports whether any row of the card has all 5 numbers present in drawn.
func HasLine(c card.Card, drawn []int) bool {
	d := drawnSet(drawn)
	for r := 0; r < 3; r++ {
		ok := true
		for col := 0; col < 9; col++ {
			n := c[r][col]
			if n == 0 {
				continue
			}
			if !d[n] {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}