package model

// EventEffects are an industry event's sustained multiplicative modifiers;
// neutral value is 1.0 (same convention as TechEffects).
// TechCostMult only applies to the tech branch in ActiveModifier.Target.
type EventEffects struct {
	BuildCostMult      float64 // self-build capex (BuildServer)
	PowerCostMult      float64 // electricity price
	RefPriceMult       float64 // willingness to pay (reference price)
	UserGrowthMult     float64 // user-target growth
	TechCostMult       float64 // tech unlock cost for the targeted branch
	TAMMult            float64 // market size (SegmentTargetScale)
	ValuationMult      float64 // valuation multiple
	SafetyWeightMult   float64 // safety-dim weight in appeal (player & rivals)
	IncidentChanceMult float64 // model-incident trigger weight
}

// NeutralEventEffects returns effects that change nothing (all 1.0).
func NeutralEventEffects() EventEffects {
	return EventEffects{
		BuildCostMult: 1, PowerCostMult: 1, RefPriceMult: 1, UserGrowthMult: 1,
		TechCostMult: 1, TAMMult: 1, ValuationMult: 1, SafetyWeightMult: 1,
		IncidentChanceMult: 1,
	}
}

// ActiveModifier is a live timed event effect. Target is the event's rolled
// target index (tech branch / competitor / direction); -1 when unused.
type ActiveModifier struct {
	EventID   string
	ExpiresAt float64 // GameTime seconds; removed once GameTime passes it
	Target    int
	Effects   EventEffects
}

// PendingEvent is a fired event awaiting the player's choice. Past Deadline
// it auto-resolves to the catalog's DefaultChoice (always the free option).
type PendingEvent struct {
	EventID  string
	Target   int
	FiredAt  float64
	Deadline float64
}

// EventRecord is one line of resolved-event history (ring, capped in balance).
type EventRecord struct {
	EventID string
	At      float64
	Choice  int  // resolved choice index; 0 for no-choice events
	Auto    bool // true = timeout / offline auto-resolve
}

// EventsState is the industry-event subsystem state carried in GameState.
// RandState is the splitmix64 state; all sim randomness flows through it.
// FiredCount / AutoCount are monotonic counters for offline summaries.
type EventsState struct {
	RandState   uint64
	NextCheckAt float64
	Pending     []PendingEvent
	Active      []ActiveModifier
	Log         []EventRecord
	FiredCount  int
	AutoCount   int
}

// ResolveEvent applies the player's choice to Pending[PendingIndex].
type ResolveEvent struct {
	PendingIndex int
	Choice       int
}

func (ResolveEvent) commandMarker() {}
