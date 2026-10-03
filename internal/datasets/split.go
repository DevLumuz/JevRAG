package datasets

import "hash/fnv"

// SplitDevTest partitions questions into a dev split (for tuning thresholds)
// and a test split (untouched until thresholds are frozen). Assignment depends
// only on each row's ID hash, so it is stable across runs, row order, and
// newly added rows. devFraction is approximate.
func SplitDevTest(rows []*KoBLEXRow, devFraction float64) (dev, test []*KoBLEXRow) {
	for _, r := range rows {
		if inDev(r.ID, devFraction) {
			dev = append(dev, r)
		} else {
			test = append(test, r)
		}
	}
	return dev, test
}

// InDev reports whether id falls in the dev split; SplitDevTest uses it, and
// other datasets use it directly for the same stable, hash-based assignment.
func InDev(id string, devFraction float64) bool { return inDev(id, devFraction) }

func inDev(id string, devFraction float64) bool {
	h := fnv.New32a()
	h.Write([]byte(id))
	return float64(h.Sum32()%1000) < devFraction*1000
}
