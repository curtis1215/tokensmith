package metrics

import (
	"time"

	"tokensmith/internal/dailyusage"
)

const (
	SchemaVersion = 1
	MaxDays       = 90
	SourceStaff   = "staff"
)

// SourceOrder is the canonical display order for R&D inflow sources.
var SourceOrder = []string{"claude-code", "codex", "grok", "opencode", SourceStaff}

// DayPoint is one local day's KPI stock snapshot plus cumulative inflow.
type DayPoint struct {
	Users          float64            `json:"users"`
	MonthlyRevenue float64            `json:"monthlyRevenue"`
	RnDStock       float64            `json:"rndStock"`
	RnDInflow      map[string]float64 `json:"rndInflow,omitempty"`
	OpenUsers      float64            `json:"openUsers,omitempty"`
	OpenRevenue    float64            `json:"openRevenue,omitempty"`
	OpenRnD        float64            `json:"openRnd,omitempty"`
	OpenSet        bool               `json:"openSet,omitempty"`
}

// Document is the versioned metrics-history file contents.
type Document struct {
	SchemaVersion int                 `json:"schemaVersion"`
	UpdatedAt     int64               `json:"updatedAt"`
	Days          map[string]DayPoint `json:"days"`
}

// EmptyDocument returns a v1 document with an empty days map.
func EmptyDocument() Document {
	return Document{SchemaVersion: SchemaVersion, Days: map[string]DayPoint{}}
}

// DayKey returns the YYYY-MM-DD key for at via dailyusage (local day).
func DayKey(at time.Time) string { return dailyusage.DayKey(at) }
