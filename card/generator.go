// Package card generates and validates 90-ball bingo cards (3x9).
package card

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"sort"
)

const (
	rows    = 3
	cols    = 9
	perRow  = 5            // numbers per row
	perCard = rows * perRow // 15 numbers per card
)

// Card is a 3x9 bingo card. A cell with value 0 means empty
// (no bingo number is 0, so it's a safe sentinel).
type Card [rows][cols]int

// GenerateCard produces a random, valid 90-ball bingo card:
//   - 3x9, 15 numbers, exactly 5 per row
//   - 1 to 3 numbers per column
//   - each column covers its range (col 0: 1-9, col 1: 10-19, ... col 8: 80-90)
//   - within each column, numbers are sorted ascending (top to bottom)
func GenerateCard() (Card, error) {
	// Step a: one number per column (guarantees the 1-per-column minimum).
	colNums := make([][]int, cols)
	for c := 0; c < cols; c++ {
		n, err := randomInRange(colRangeMin(c), colRangeMax(c))
		if err != nil {
			return Card{}, err
		}
		colNums[c] = []int{n}
	}

	// Step b: distribute the 6 remaining numbers at random, max 3 per column.
	// Column ranges are disjoint, so two columns can never produce the same
	// number; we only need to avoid duplicates within the same column.
	for placed := 0; placed < perCard-cols; {
		c, err := randomChoice(cols)
		if err != nil {
			return Card{}, err
		}
		if len(colNums[c]) >= 3 {
			continue
		}
		n, err := randomInRange(colRangeMin(c), colRangeMax(c))
		if err != nil {
			return Card{}, err
		}
		if contains(colNums[c], n) {
			continue
		}
		colNums[c] = append(colNums[c], n)
		placed++
	}

	// Sort each column ascending.
	for c := 0; c < cols; c++ {
		sort.Ints(colNums[c])
	}

	// Step c: place numbers into rows with exactly 5 per row.
	var result Card
	needs := [rows]int{perRow, perRow, perRow}
	row := 0
	for c := 0; c < cols; c++ {
		// Pick the rows for this column (round-robin, skipping full rows).
		chosen := make([]int, 0, len(colNums[c]))
		for range colNums[c] {
			for needs[row] == 0 {
				row = (row + 1) % rows
			}
			chosen = append(chosen, row)
			needs[row]--
			row = (row + 1) % rows
		}
		// Sorted numbers into sorted rows => ascending column.
		sort.Ints(chosen)
		for i, n := range colNums[c] {
			result[chosen[i]][c] = n
		}
	}

	if err := validateCard(result); err != nil {
		return Card{}, fmt.Errorf("invalid card: %w", err)
	}
	return result, nil
}

// validateCard checks every constraint of a valid 90-ball card.
func validateCard(c Card) error {
	count := 0
	seen := make(map[int]struct{})
	for r := 0; r < rows; r++ {
		inRow := 0
		for col := 0; col < cols; col++ {
			v := c[r][col]
			if v == 0 {
				continue
			}
			inRow++
			if v < colRangeMin(col) || v > colRangeMax(col) {
				return fmt.Errorf("value %d out of range for column %d", v, col+1)
			}
			if _, ok := seen[v]; ok {
				return fmt.Errorf("duplicate value %d", v)
			}
			seen[v] = struct{}{}
			count++
		}
		if inRow != perRow {
			return fmt.Errorf("row %d has %d numbers, expected %d", r+1, inRow, perRow)
		}
	}
	if count != perCard {
		return fmt.Errorf("total numbers %d, expected %d", count, perCard)
	}

	// Per-column: 1-3 numbers, sorted ascending.
	for col := 0; col < cols; col++ {
		colCount := 0
		last := 0
		for r := 0; r < rows; r++ {
			v := c[r][col]
			if v == 0 {
				continue
			}
			colCount++
			if v < last {
				return fmt.Errorf("column %d is not sorted ascending", col+1)
			}
			last = v
		}
		if colCount < 1 || colCount > 3 {
			return fmt.Errorf("column %d has %d numbers, expected 1-3", col+1, colCount)
		}
	}
	return nil
}

// randomInRange returns a uniform random integer in [min, max].
func randomInRange(min, max int) (int, error) {
	if min > max {
		return 0, fmt.Errorf("invalid range %d-%d", min, max)
	}
	nBig, err := rand.Int(rand.Reader, big.NewInt(int64(max-min+1)))
	if err != nil {
		return 0, err
	}
	return int(nBig.Int64()) + min, nil
}

// randomChoice returns a random integer in [0, n-1].
func randomChoice(n int) (int, error) {
	return randomInRange(0, n-1)
}

func contains(arr []int, val int) bool {
	for _, a := range arr {
		if a == val {
			return true
		}
	}
	return false
}

// colRangeMin/Max define the inclusive number range for each column.
// col 0: 1-9, col 1: 10-19, ..., col 8: 80-90.
func colRangeMin(col int) int {
	if col == 0 {
		return 1
	}
	return col * 10
}

func colRangeMax(col int) int {
	if col == cols-1 {
		return 90
	}
	return col*10 + 9
}