package datasets_test

import (
	"fmt"
	"testing"

	"jev/internal/datasets"
)

func makeRows(n int) []*datasets.KoBLEXRow {
	rows := make([]*datasets.KoBLEXRow, n)
	for i := range rows {
		rows[i] = &datasets.KoBLEXRow{ID: fmt.Sprintf("qa_%d", i)}
	}
	return rows
}

func TestSplitDevTest(t *testing.T) {
	rows := makeRows(226)
	dev, test := datasets.SplitDevTest(rows, 0.3)

	if len(dev)+len(test) != len(rows) {
		t.Fatalf("dev %d + test %d != %d", len(dev), len(test), len(rows))
	}
	// Hash-based split: close to the fraction, not exact.
	if len(dev) < 45 || len(dev) > 90 {
		t.Errorf("len(dev) = %d, want roughly 30%% of 226", len(dev))
	}

	seen := map[string]bool{}
	for _, r := range dev {
		seen[r.ID] = true
	}
	for _, r := range test {
		if seen[r.ID] {
			t.Errorf("%s is in both splits", r.ID)
		}
	}
}

func TestSplitDevTest_Deterministic(t *testing.T) {
	// Same IDs, different order and an extra row → same assignment for old rows.
	a := makeRows(50)
	b := append(makeRows(51)[1:], makeRows(1)...)
	devA, _ := datasets.SplitDevTest(a, 0.3)
	devB, _ := datasets.SplitDevTest(b, 0.3)

	inB := map[string]bool{}
	for _, r := range devB {
		inB[r.ID] = true
	}
	for _, r := range devA {
		if !inB[r.ID] {
			t.Errorf("%s changed split when order/rows changed", r.ID)
		}
	}
}
