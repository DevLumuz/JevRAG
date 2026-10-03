package probe

import (
	"math"
	"sort"
)

// AUC is the probability that a random positive scores above a random
// negative (Mann–Whitney; ties count half). 0.5 = no separation, 1 = perfect.
// Returns NaN if either class is empty.
func AUC(pos, neg []float64) float64 {
	if len(pos) == 0 || len(neg) == 0 {
		return math.NaN()
	}
	type pt struct {
		v   float64
		pos bool
	}
	all := make([]pt, 0, len(pos)+len(neg))
	for _, v := range pos {
		all = append(all, pt{v, true})
	}
	for _, v := range neg {
		all = append(all, pt{v, false})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].v < all[j].v })
	// Average ranks for ties, then the rank-sum formula.
	rankSum := 0.0
	for i := 0; i < len(all); {
		j := i
		for j < len(all) && all[j].v == all[i].v {
			j++
		}
		avg := float64(i+j+1) / 2 // ranks are 1-based: (i+1 + j) / 2
		for k := i; k < j; k++ {
			if all[k].pos {
				rankSum += avg
			}
		}
		i = j
	}
	np, nn := float64(len(pos)), float64(len(neg))
	return (rankSum - np*(np+1)/2) / (np * nn)
}

// Logistic is a logistic-regression combination of features, fit with
// standardized inputs and L2 regularization. It turns several narrow judge
// answers into one ranking score (composite scoring) with learned weights.
type Logistic struct {
	Names   []string
	Mean    []float64
	Std     []float64
	Weights []float64 // per standardized feature
	Bias    float64
}

// FitLogistic learns weights that predict y (1 gold, 0 not) from features.
func FitLogistic(names []string, x [][]float64, y []float64) *Logistic {
	n, d := len(x), len(names)
	m := &Logistic{Names: names, Mean: make([]float64, d), Std: make([]float64, d), Weights: make([]float64, d)}
	for j := 0; j < d; j++ {
		for i := 0; i < n; i++ {
			m.Mean[j] += x[i][j]
		}
		m.Mean[j] /= float64(n)
		for i := 0; i < n; i++ {
			m.Std[j] += (x[i][j] - m.Mean[j]) * (x[i][j] - m.Mean[j])
		}
		m.Std[j] = math.Sqrt(m.Std[j] / float64(n))
		if m.Std[j] == 0 {
			m.Std[j] = 1
		}
	}
	const (
		lr     = 0.1
		l2     = 0.01
		epochs = 2000
	)
	z := make([]float64, d)
	for e := 0; e < epochs; e++ {
		gw := make([]float64, d)
		gb := 0.0
		for i := 0; i < n; i++ {
			for j := range z {
				z[j] = (x[i][j] - m.Mean[j]) / m.Std[j]
			}
			err := m.prob(z) - y[i]
			for j := range gw {
				gw[j] += err * z[j]
			}
			gb += err
		}
		for j := range m.Weights {
			m.Weights[j] -= lr * (gw[j]/float64(n) + l2*m.Weights[j])
		}
		m.Bias -= lr * gb / float64(n)
	}
	return m
}

func (m *Logistic) prob(z []float64) float64 {
	s := m.Bias
	for j, w := range m.Weights {
		s += w * z[j]
	}
	return 1 / (1 + math.Exp(-s))
}

// Score returns the predicted probability for one feature vector.
func (m *Logistic) Score(x []float64) float64 {
	z := make([]float64, len(x))
	for j := range x {
		z[j] = (x[j] - m.Mean[j]) / m.Std[j]
	}
	return m.prob(z)
}

// ClusterMean returns the mean of values and its 95% bootstrap interval,
// resampling whole clusters (e.g. all values of one question together) so
// that correlated values are not counted as independent. Deterministic for a
// given seed.
func ClusterMean(values []float64, clusters []string, reps int, seed uint64) (mean, lo, hi float64) {
	byCluster := map[string][]float64{}
	var ids []string
	for i, v := range values {
		if math.IsNaN(v) {
			continue
		}
		c := clusters[i]
		if _, ok := byCluster[c]; !ok {
			ids = append(ids, c)
		}
		byCluster[c] = append(byCluster[c], v)
	}
	if len(ids) == 0 {
		return math.NaN(), math.NaN(), math.NaN()
	}
	meanOf := func(pick func(int) string) float64 {
		s, n := 0.0, 0
		for i := range ids {
			for _, v := range byCluster[pick(i)] {
				s += v
				n++
			}
		}
		return s / float64(n)
	}
	mean = meanOf(func(i int) string { return ids[i] })
	rng := seed | 1
	next := func() uint64 { // xorshift64
		rng ^= rng << 13
		rng ^= rng >> 7
		rng ^= rng << 17
		return rng
	}
	ms := make([]float64, reps)
	for r := range ms {
		ms[r] = meanOf(func(int) string { return ids[next()%uint64(len(ids))] })
	}
	sort.Float64s(ms)
	return mean, ms[int(0.025*float64(reps))], ms[int(0.975*float64(reps))-1]
}
