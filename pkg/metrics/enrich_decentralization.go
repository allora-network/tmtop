// pkg/metrics/enrich_decentralization.go    (WS-A)
package metrics

import (
	"sort"

	"main/pkg/types"
)

func enrichDecentralization(nh *NetworkHealth, v types.TMValidators) {
	p := make([]int64, 0, len(v))
	for _, val := range v {
		if val.CometValidator != nil {
			p = append(p, val.CometValidator.VotingPower)
		}
	}
	if len(p) == 0 {
		return
	}
	nh.Nakamoto33, nh.Nakamoto66 = nakamoto(p)
	nh.Gini = gini(p)
	nh.Top10Pct = topNShare(p, 10)
}

// nakamoto returns the minimum validator counts whose cumulative power exceeds
// 1/3 and 2/3 of the total (descending power).
func nakamoto(p []int64) (n33, n66 int) {
	s := append([]int64(nil), p...)
	sort.Slice(s, func(i, j int) bool { return s[i] > s[j] })
	var total int64
	for _, x := range s {
		total += x
	}
	var cum int64
	for i, x := range s {
		cum += x
		if n33 == 0 && cum*3 > total {
			n33 = i + 1
		}
		if n66 == 0 && cum*3 > total*2 {
			n66 = i + 1
			break
		}
	}
	return n33, n66
}

// gini computes the Gini coefficient of the power distribution.
func gini(p []int64) float64 {
	n := len(p)
	if n == 0 {
		return 0
	}
	s := append([]int64(nil), p...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	var sum, weighted int64
	for i, x := range s {
		sum += x
		weighted += int64(i+1) * x
	}
	if sum == 0 {
		return 0
	}
	// G = (2*Σ i*x_i)/(n*Σ x_i) - (n+1)/n
	return (2.0*float64(weighted))/(float64(n)*float64(sum)) - float64(n+1)/float64(n)
}

func topNShare(p []int64, topN int) float64 {
	s := append([]int64(nil), p...)
	sort.Slice(s, func(i, j int) bool { return s[i] > s[j] })
	var total, top int64
	for i, x := range s {
		total += x
		if i < topN {
			top += x
		}
	}
	if total == 0 {
		return 0
	}
	return 100.0 * float64(top) / float64(total)
}
