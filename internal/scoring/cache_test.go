package scoring

import (
	"encoding/json"
	"testing"
)

func TestCacheSnapshotStartsEmpty(t *testing.T) {
	c := NewCache()
	if got := c.Snapshot(); string(got) != `{"modules":null,"entries":[]}` {
		t.Errorf("empty snapshot = %s, want empty entries", got)
	}
}

func TestCacheStoreRoundTrip(t *testing.T) {
	c := NewCache()
	pc := 1
	c.store([]Entry{{Rank: 1, Name: "A", PCNumber: &pc, WSI: 854, Award: AwardGold}}, nil)
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
