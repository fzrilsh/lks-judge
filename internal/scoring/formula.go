// Package scoring computes WorldSkills-style scaled results (WSI) from raw
// per-participant totals. The scale is a robust standardised score: it centres
// 700 on the population median and measures spread with the median absolute
// deviation (MAD), so a few outlier top scores do not distort the middle.
// Nothing here touches the database; callers pass totals in and get scaled
// results out. WSI is never persisted.
package scoring

import (
	"math"
	"sort"
)

// Award labels, assigned by WSI-descending rank.
const (
	AwardGold      = "Gold"
	AwardSilver    = "Silver"
	AwardBronze    = "Bronze"
	AwardMedallion = "Medallion for Excellence"
)

// Entry is one participant's row on the leaderboard. Rank, WSI and Award are
// filled by Rank(); the caller supplies the identity fields and TotalRaw.
type Entry struct {
	ParticipantID int64
	Name          string
	School        string
	PCNumber      *int
	TotalRaw      float64
	WSI           int
	Rank          int
	Award         string
	Scores        map[int64]float64 // per-module raw score, keyed by module ID; nil/absent module => 0
}

// Median returns the median of xs. Even count returns the mean of the two
// middle values. It sorts a copy, so the caller's slice is untouched. Empty
// input returns 0.
func Median(xs []float64) float64 {
	n := len(xs)
	if n == 0 {
		return 0
	}
	c := make([]float64, n)
	copy(c, xs)
	sort.Float64s(c)
	if n%2 == 1 {
		return c[n/2]
	}
	return (c[n/2-1] + c[n/2]) / 2
}

// MAD is the median absolute deviation from the given median.
func MAD(xs []float64, median float64) float64 {
	dev := make([]float64, len(xs))
	for i, x := range xs {
		dev[i] = math.Abs(x - median)
	}
	return Median(dev)
}

// StdDev is the population standard deviation of xs. Used as the spread
// fallback when MAD collapses to 0 (a majority of tied totals, e.g. many
// participants still on 0), which would otherwise flatten everyone to 700.
func StdDev(xs []float64) float64 {
	n := len(xs)
	if n == 0 {
		return 0
	}
	var sum float64
	for _, x := range xs {
		sum += x
	}
	mean := sum / float64(n)
	var ss float64
	for _, x := range xs {
		d := x - mean
		ss += d * d
	}
	return math.Sqrt(ss / float64(n))
}

// ScaleScore maps a raw total to the robust standardised WSI:
//
//	700 + 30 * (raw - center) / spread
//
// clamped to [0, 1000] and rounded. 30 sets the points-per-unit spread. The
// caller supplies the spread (1.4826*MAD, or a std-dev fallback); a spread of 0
// means no dispersion in the population, so every raw maps to the centre 700.
func ScaleScore(raw, center, spread float64) int {
	if spread == 0 {
		return 700
	}
	s := 700 + 30*(raw-center)/spread
	if s < 0 {
		s = 0
	}
	if s > 1000 {
		s = 1000
	}
	return int(math.Round(s))
}

// Rank fills WSI, Rank and Award for every entry and returns them sorted by
// WSI descending. Median and MAD are computed from the entries' TotalRaw. The
// sort is stable, so equal WSI keeps input order (callers pass entries in
// pc_number order). Awards: rank 1/2/3 -> Gold/Silver/Bronze; rank > 3 with
// WSI >= 700 -> Medallion for Excellence; otherwise none.
func Rank(entries []Entry) []Entry {
	totals := make([]float64, len(entries))
	for i, e := range entries {
		totals[i] = e.TotalRaw
	}
	median := Median(totals)
	mad := MAD(totals, median)
	// Robust spread is 1.4826*MAD. When more than half the totals tie (common
	// early on with many participants on 0), MAD is 0 and would flatten every
	// WSI to 700; fall back to std dev so real score gaps still separate.
	spread := 1.4826 * mad
	if spread == 0 {
		spread = StdDev(totals)
	}

	out := make([]Entry, len(entries))
	copy(out, entries)
	for i := range out {
		out[i].WSI = ScaleScore(out[i].TotalRaw, median, spread)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].WSI > out[j].WSI })
	for i := range out {
		out[i].Rank = i + 1
		switch {
		case i == 0:
			out[i].Award = AwardGold
		case i == 1:
			out[i].Award = AwardSilver
		case i == 2:
			out[i].Award = AwardBronze
		case out[i].WSI >= 700:
			out[i].Award = AwardMedallion
		}
	}
	return out
}
