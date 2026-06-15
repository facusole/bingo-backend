package card

import "testing"

func TestGenerateCard(t *testing.T) {
	// Expected column ranges, hardcoded ON PURPOSE (not derived from
	// colRangeMin), so a bug in that helper can't make the test pass.
	wantMin := [cols]int{1, 10, 20, 30, 40, 50, 60, 70, 80}
	wantMax := [cols]int{9, 19, 29, 39, 49, 59, 69, 79, 90}

	for i := 0; i < 10000; i++ {
		c, err := GenerateCard()
		if err != nil {
			t.Fatalf("card %d: GenerateCard returned error: %v", i, err)
		}

		total := 0
		seen := map[int]bool{}
		var rowCount [rows]int
		var colCount [cols]int

		for r := 0; r < rows; r++ {
			for col := 0; col < cols; col++ {
				v := c[r][col]
				if v == 0 {
					continue
				}
				total++
				rowCount[r]++
				colCount[col]++

				if v < wantMin[col] || v > wantMax[col] {
					t.Fatalf("card %d: value %d in column %d out of range [%d,%d]",
						i, v, col, wantMin[col], wantMax[col])
				}
				if seen[v] {
					t.Fatalf("card %d: duplicate value %d", i, v)
				}
				seen[v] = true
			}
		}

		if total != 15 {
			t.Fatalf("card %d: got %d numbers, want 15", i, total)
		}
		for r := 0; r < rows; r++ {
			if rowCount[r] != perRow {
				t.Fatalf("card %d: row %d has %d numbers, want %d", i, r, rowCount[r], perRow)
			}
		}
		for col := 0; col < cols; col++ {
			if colCount[col] < 1 || colCount[col] > 3 {
				t.Fatalf("card %d: column %d has %d numbers, want 1-3", i, col, colCount[col])
			}
			// numbers must be ascending top-to-bottom within the column
			last := 0
			for r := 0; r < rows; r++ {
				v := c[r][col]
				if v == 0 {
					continue
				}
				if v < last {
					t.Fatalf("card %d: column %d not sorted ascending", i, col)
				}
				last = v
			}
		}
	}
}