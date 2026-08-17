package scoring

import (
	"encoding/json"
	"math/rand"
	"sync"
	"sync/atomic"

	"github.com/fzrilsh/lks-judge/internal/store"
)

// row is the public JSON shape of one leaderboard line. Separate from Entry so
// the wire format is explicit and stable regardless of Entry's internals.
type row struct {
	Rank     int               `json:"rank"`
	PCNumber *int              `json:"pc_number,omitempty"`
	Name     string            `json:"name"`
	School   string            `json:"school"`
	WSI      int               `json:"wsi"`
	Award    string            `json:"award,omitempty"`
	Scores   map[int64]float64 `json:"scores,omitempty"`
}

// moduleInfo tells the client the column order and names for the per-module
// score cells. Marshaled alongside the rows in the leaderboard payload.
type moduleInfo struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// Cache holds the pre-rendered leaderboard JSON. Refresh recomputes it from the
// population after every score write; Snapshot hands out the current bytes with
// no DB read, so many concurrent leaderboard loads cost nothing.
type Cache struct {
	json    atomic.Pointer[[]byte]
	refresh sync.Mutex // serializes Refresh so the last write wins the last snapshot
}

// NewCache returns a Cache pre-seeded with an empty leaderboard, so Snapshot is
// safe to call before the first Refresh.
func NewCache() *Cache {
	c := &Cache{}
	c.store(nil, nil, false)
	return c
}

// store marshals entries to the wire shape and swaps in the bytes. When censored,
// rank/wsi/scores/award are blanked and rows are shuffled, so the public page
// leaks neither standing nor score while the jury holds results back.
func (c *Cache) store(entries []Entry, modules []moduleInfo, censored bool) {
	if censored {
		rand.Shuffle(len(entries), func(i, j int) { entries[i], entries[j] = entries[j], entries[i] })
	}
	rows := make([]row, 0, len(entries))
	for _, e := range entries {
		if censored {
			rows = append(rows, row{PCNumber: e.PCNumber, Name: e.Name, School: e.School})
			continue
		}
		rows = append(rows, row{
			Rank: e.Rank, PCNumber: e.PCNumber, Name: e.Name,
			School: e.School, WSI: e.WSI, Award: e.Award, Scores: e.Scores,
		})
	}
	b, err := json.Marshal(struct {
		Censored bool         `json:"censored"`
		Modules  []moduleInfo `json:"modules"`
		Entries  []row        `json:"entries"`
	}{censored, modules, rows})
	if err != nil {
		b = []byte(`{"censored":false,"modules":null,"entries":[]}`) // marshal of plain strings/ints cannot fail; belt and braces
	}
	c.json.Store(&b)
}

// Refresh reloads the population, ranks it and re-renders the JSON. The mutex
// serializes the read+store so two concurrent writes cannot let an earlier
// population overwrite a later one (lost update).
func (c *Cache) Refresh(st *store.Store, competitionID int64) error {
	c.refresh.Lock()
	defer c.refresh.Unlock()
	totals, err := st.ListParticipantTotals(competitionID)
	if err != nil {
		return err
	}
	modules, err := st.ListModules(competitionID)
	if err != nil {
		return err
	}
	cells, err := st.ScoresByParticipantModule(competitionID)
	if err != nil {
		return err
	}
	mods := make([]moduleInfo, len(modules))
	for i, m := range modules {
		mods[i] = moduleInfo{ID: m.ID, Name: m.Name}
	}
	entries := make([]Entry, len(totals))
	for i, t := range totals {
		entries[i] = Entry{
			ParticipantID: t.ParticipantID, Name: t.Name, School: t.School,
			PCNumber: t.PCNumber, TotalRaw: t.TotalRaw, Scores: cells[t.ParticipantID],
		}
	}
	censored := false
	if comp := st.CompetitionCache.Load(); comp != nil {
		censored = comp.Censored
	}
	c.store(Rank(entries), mods, censored)
	return nil
}

// Snapshot returns the current leaderboard JSON. Never nil.
func (c *Cache) Snapshot() []byte {
	return *c.json.Load()
}

// Clear empties the leaderboard. Used after a nuclear reset, where Refresh can't
// help because the competition it needs is gone.
func (c *Cache) Clear() {
	c.refresh.Lock()
	defer c.refresh.Unlock()
	c.store(nil, nil, false)
}
