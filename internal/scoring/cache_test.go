package scoring

import (
	"encoding/json"
	"testing"
)

func TestCacheSnapshotStartsEmpty(t *testing.T) {
	c := NewCache()
	if got := c.Snapshot(); string(got) != `{"censored":false,"modules":null,"entries":[]}` {
		t.Errorf("empty snapshot = %s, want empty entries", got)
	}
}

func TestCacheStoreRoundTrip(t *testing.T) {
	c := NewCache()
	pc := 1
	c.store([]Entry{{Rank: 1, Name: "A", PCNumber: &pc, WSI: 854, Award: AwardGold}}, nil, false)
	var out struct {
		Entries []struct {
			Rank     int    `json:"rank"`
			Name     string `json:"name"`
			WSI      int    `json:"wsi"`
			Award    string `json:"award"`
			PCNumber *int   `json:"pc_number"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(c.Snapshot(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Entries) != 1 || out.Entries[0].WSI != 854 || out.Entries[0].Award != AwardGold {
		t.Fatalf("round trip = %+v", out.Entries)
	}
}

func TestCacheStoreCensored(t *testing.T) {
	c := NewCache()
	pc := 1
	c.store([]Entry{{Rank: 1, Name: "A", School: "S", PCNumber: &pc, WSI: 854, Award: AwardGold,
		Scores: map[int64]float64{1: 90}}}, []moduleInfo{{ID: 1, Name: "MA"}}, true)
	var out struct {
		Censored bool `json:"censored"`
		Entries  []struct {
			Rank   int               `json:"rank"`
			Name   string            `json:"name"`
			WSI    int               `json:"wsi"`
			Award  string            `json:"award"`
			Scores map[int64]float64 `json:"scores"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(c.Snapshot(), &out); err != nil {
		t.Fatal(err)
	}
	if !out.Censored || len(out.Entries) != 1 {
		t.Fatalf("censored snapshot = %s", c.Snapshot())
	}
	e := out.Entries[0]
	if e.Name != "A" || e.Rank != 0 || e.WSI != 0 || e.Award != "" || len(e.Scores) != 0 {
		t.Fatalf("censored row leaked data: %+v", e)
	}
}
