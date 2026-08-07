package scoring

import (
	"encoding/json"
	"sync/atomic"

	"github.com/fzrilsh/lks-judge/internal/store"
)

// row is the public JSON shape of one leaderboard line. Separate from Entry so
// the wire format is explicit and stable regardless of Entry's internals.
type row struct {
	Rank     int    `json:"rank"`
	PCNumber *int   `json:"pc_number,omitempty"`
	Name     string `json:"name"`
	School   string `json:"school"`
	WSI      int    `json:"wsi"`
	Award    string `json:"award,omitempty"`
}

// Cache holds the pre-rendered leaderboard JSON. Refresh recomputes it from the
// population after every score write; Snapshot hands out the current bytes with
// no DB read, so many concurrent leaderboard loads cost nothing.
type Cache struct {
	json atomic.Pointer[[]byte]
}

// NewCache returns a Cache pre-seeded with an empty leaderboard, so Snapshot is
// safe to call before the first Refresh.
func NewCache() *Cache {
	c := &Cache{}
	c.store(nil)
	return c
}

// store marshals entries to the wire shape and swaps in the bytes.
func (c *Cache) store(entries []Entry) {
	rows := make([]row, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, row{
			Rank: e.Rank, PCNumber: e.PCNumber, Name: e.Name,
			School: e.School, WSI: e.WSI, Award: e.Award,
		})
	}
	b, err := json.Marshal(struct {
		Entries []row `json:"entries"`
	}{rows})
	if err != nil {
		b = []byte(`{"entries":[]}`) // marshal of plain strings/ints cannot fail; belt and braces
	}
	c.json.Store(&b)
}

// Refresh reloads the population, ranks it and re-renders the JSON.
func (c *Cache) Refresh(st *store.Store, competitionID int64) error {
	totals, err := st.ListParticipantTotals(competitionID)
	if err != nil {
		return err
	}
	entries := make([]Entry, len(totals))
	for i, t := range totals {
		entries[i] = Entry{
			ParticipantID: t.ParticipantID, Name: t.Name, School: t.School,
			PCNumber: t.PCNumber, TotalRaw: t.TotalRaw,
		}
	}
	c.store(Rank(entries))
	return nil
}

// Snapshot returns the current leaderboard JSON. Never nil.
func (c *Cache) Snapshot() []byte {
	return *c.json.Load()
}
