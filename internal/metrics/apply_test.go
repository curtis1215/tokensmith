package metrics

import (
	"math"
	"testing"
	"time"
)

func TestUpsertSnapshotSetsOpenOnce(t *testing.T) {
	doc := EmptyDocument()
	UpsertSnapshot(&doc, "2026-07-14", 100, 50, 10, 1)
	UpsertSnapshot(&doc, "2026-07-14", 200, 80, 5, 2)
	p := doc.Days["2026-07-14"]
	if !p.OpenSet || p.OpenUsers != 100 || p.OpenRevenue != 50 || p.OpenRnD != 10 {
		t.Fatalf("open frozen wrong: %+v", p)
	}
	if p.Users != 200 || p.MonthlyRevenue != 80 || p.RnDStock != 5 {
		t.Fatalf("latest stock wrong: %+v", p)
	}
}

func TestAddInflowAccumulates(t *testing.T) {
	doc := EmptyDocument()
	AddInflow(&doc, "2026-07-14", "claude-code", 10, 1)
	AddInflow(&doc, "2026-07-14", "claude-code", 5, 2)
	AddInflow(&doc, "2026-07-14", "staff", 3, 3)
	AddInflow(&doc, "2026-07-14", "claude-code", -1, 4) // ignored
	p := doc.Days["2026-07-14"]
	if math.Abs(p.RnDInflow["claude-code"]-15) > 1e-9 {
		t.Fatalf("claude=%v want 15", p.RnDInflow["claude-code"])
	}
	if math.Abs(p.RnDInflow["staff"]-3) > 1e-9 {
		t.Fatalf("staff=%v want 3", p.RnDInflow["staff"])
	}
}

func TestPruneKeepsNewest90(t *testing.T) {
	doc := EmptyDocument()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 95; i++ {
		day := start.AddDate(0, 0, i).Format("2006-01-02")
		UpsertSnapshot(&doc, day, float64(i), 0, 0, int64(i))
	}
	Prune(&doc)
	if len(doc.Days) != MaxDays {
		t.Fatalf("len=%d want %d", len(doc.Days), MaxDays)
	}
	if _, ok := doc.Days["2026-01-01"]; ok {
		t.Fatal("oldest day should be pruned")
	}
	if _, ok := doc.Days[start.AddDate(0, 0, 94).Format("2006-01-02")]; !ok {
		t.Fatal("newest day should remain")
	}
}

func TestSortedDaysAscending(t *testing.T) {
	doc := EmptyDocument()
	UpsertSnapshot(&doc, "2026-07-12", 1, 0, 0, 1)
	UpsertSnapshot(&doc, "2026-07-10", 1, 0, 0, 1)
	UpsertSnapshot(&doc, "2026-07-11", 1, 0, 0, 1)
	got := SortedDays(doc)
	want := []string{"2026-07-10", "2026-07-11", "2026-07-12"}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("SortedDays=%v want %v", got, want)
	}
}

func TestInvalidDayRejected(t *testing.T) {
	doc := EmptyDocument()
	UpsertSnapshot(&doc, "not-a-day", 1, 2, 3, 1)
	AddInflow(&doc, "2026/07/14", "staff", 9, 1)
	if len(doc.Days) != 0 {
		t.Fatalf("invalid days should be ignored: %+v", doc.Days)
	}
}
