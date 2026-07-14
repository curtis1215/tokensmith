package metrics

import (
	"sort"
	"time"
)

// ValidDayKey reports whether day is a parseable YYYY-MM-DD key.
func ValidDayKey(day string) bool {
	if len(day) != 10 {
		return false
	}
	for i, c := range day {
		switch i {
		case 4, 7:
			if c != '-' {
				return false
			}
		default:
			if c < '0' || c > '9' {
				return false
			}
		}
	}
	_, err := time.Parse("2006-01-02", day)
	return err == nil
}

// EnsureDay returns a pointer to a mutable copy of the day entry, initializing
// the days map and RnDInflow as needed. Callers must write the point back:
//
//	p := EnsureDay(doc, day)
//	// mutate *p
//	doc.Days[day] = *p
func EnsureDay(doc *Document, day string) *DayPoint {
	if doc.Days == nil {
		doc.Days = map[string]DayPoint{}
	}
	p, ok := doc.Days[day]
	if !ok {
		p = DayPoint{RnDInflow: map[string]float64{}}
	} else if p.RnDInflow == nil {
		p.RnDInflow = map[string]float64{}
	}
	doc.Days[day] = p
	// Copy-on-write: return address of a local copy; callers write back.
	out := doc.Days[day]
	return &out
}

// UpsertSnapshot overwrites stock fields for day. On the first write of a day
// it freezes Open* from the provided values when OpenSet is false.
// Invalid day keys are ignored.
func UpsertSnapshot(doc *Document, day string, users, revenue, rndStock float64, nowUnix int64) {
	if doc == nil || !ValidDayKey(day) {
		return
	}
	p := EnsureDay(doc, day)
	if !p.OpenSet {
		p.OpenUsers = users
		p.OpenRevenue = revenue
		p.OpenRnD = rndStock
		p.OpenSet = true
	}
	p.Users = users
	p.MonthlyRevenue = revenue
	p.RnDStock = rndStock
	doc.Days[day] = *p
	if nowUnix > 0 {
		doc.UpdatedAt = nowUnix
	}
	if doc.SchemaVersion == 0 {
		doc.SchemaVersion = SchemaVersion
	}
}

// AddInflow accumulates amount into the day's RnDInflow for source.
// No-op if amount ≤ 0, source is empty, day is invalid, or doc is nil.
func AddInflow(doc *Document, day, source string, amount float64, nowUnix int64) {
	if doc == nil || amount <= 0 || source == "" || !ValidDayKey(day) {
		return
	}
	p := EnsureDay(doc, day)
	p.RnDInflow[source] += amount
	doc.Days[day] = *p
	if nowUnix > 0 {
		doc.UpdatedAt = nowUnix
	}
	if doc.SchemaVersion == 0 {
		doc.SchemaVersion = SchemaVersion
	}
}

// Prune keeps at most MaxDays newest valid date keys; older keys are dropped.
// Invalid keys present in the map are removed preferentially.
func Prune(doc *Document) {
	if doc == nil || doc.Days == nil {
		return
	}
	// Drop invalid keys first.
	for k := range doc.Days {
		if !ValidDayKey(k) {
			delete(doc.Days, k)
		}
	}
	if len(doc.Days) <= MaxDays {
		return
	}
	keys := make([]string, 0, len(doc.Days))
	for k := range doc.Days {
		keys = append(keys, k)
	}
	// ISO dates sort chronologically; drop oldest.
	sort.Strings(keys)
	drop := len(keys) - MaxDays
	for i := 0; i < drop; i++ {
		delete(doc.Days, keys[i])
	}
}

// SortedDays returns document day keys in ascending date order.
func SortedDays(doc Document) []string {
	if len(doc.Days) == 0 {
		return nil
	}
	keys := make([]string, 0, len(doc.Days))
	for k := range doc.Days {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Series projects pick(DayPoint) over days; missing days yield 0.
func Series(doc Document, days []string, pick func(DayPoint) float64) []float64 {
	out := make([]float64, len(days))
	for i, d := range days {
		if p, ok := doc.Days[d]; ok {
			out[i] = pick(p)
		}
	}
	return out
}
