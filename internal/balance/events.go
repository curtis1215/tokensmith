package balance

// Industry-event IDs (design §4). The TUI keys Chinese copy off these; the
// sim keys effect application off them.
const (
	EvChipShortage  = "chip-shortage"
	EvEnergySpike   = "energy-spike"
	EvRivalBreak    = "rival-breakthrough"
	EvOpenSourceWar = "open-source-war"
	EvRivalScandal  = "rival-scandal"
	EvPaper         = "breakthrough-paper"
	EvIncident      = "model-incident"
	EvRegulation    = "regulation"
	EvMarketCycle   = "market-cycle"
	EvBubbleTalk    = "bubble-talk"
)

// Per-event effect magnitudes (v0 calibration, design §4; tune in playtest).
const (
	EvChipShortageBuildMult = 1.18
	EvEnergyUpMult          = 1.3
	EvEnergyDownMult        = 0.7
	EvRivalBreakQualityPct  = 0.15 // rival capability one-shot jump
	EvRivalBreakPromoGrowth = 1.25
	EvOpenSourceRefPrice    = 0.8
	EvOpenSourceFollowRef   = 0.75
	EvOpenSourceFollowGrow  = 1.2
	EvScandalSafetyPct      = 0.20 // rival safety one-shot drop
	EvScandalPoachGrowth    = 1.3
	EvScandalWatchGrowth    = 1.1
	EvPaperBetTechCost      = 0.5
	EvPaperAbsorbTechCost   = 0.7
	EvIncidentLossPct       = 0.08 // one-shot user loss (consumer/developer)
	EvIncidentEnterprisePct = 0.15 // one-shot user loss (enterprise)
	EvIncidentQuietChance   = 1.5  // lingering IncidentChanceMult after低調
	EvRegulationSafetyW     = 1.5
	EvRegulationComplyPct   = 0.10 // one-shot player safety-quality boost
	EvMarketBoomTAM         = 1.25
	EvMarketBustTAM         = 0.8
	EvBubbleValuation       = 0.75
	EvBubbleCalmValuation   = 0.9
	// EvIncidentSafetyRef is the online-model safety quality at which the
	// incident trigger weight reaches zero (linear ramp below it).
	EvIncidentSafetyRef = 50.0
)

// EventSpec is one industry event's tuning entry. Effect application is a
// per-ID switch in internal/sim (design §3.3); this holds the numbers.
// Choice convention: index 0 = paid/active option, index 1 = free/passive
// option; DefaultChoice is always the free option so timeouts never spend.
type EventSpec struct {
	ID            string
	Weight        float64 // base trigger weight
	MinGameTime   float64 // gate: GameTime must exceed this (0 = none)
	MinValuation  float64 // gate: PeakValuation must exceed this (0 = none)
	DurationSec   float64 // sustained-modifier length (game seconds)
	DeadlineSec   float64 // decision window for choice events
	NumChoices    int     // 0 = no player choice
	DefaultChoice int
	// Choice-0 costs, charged at resolve. Cash cost scales with revenue so
	// late-game events stay meaningful: max(Floor, RevMonths×MonthlyRevenue).
	CashCostRevMonths float64
	CashCostFloor     float64
	RnDCostFrac       float64 // fraction of current R&D (breakthrough-paper)
}

// Pacing constants (game seconds). Online the TUI advances 14400 game-sec per
// real second (tickDT=3600 / 250ms), so: 5 game-days ≈ 30 real-sec (check),
// 20 game-days ≈ 2 real-min (deadline), 30 game-days ≈ 3 real-min (duration).
const (
	evDay              = 86400.0
	evDefaultDuration  = 30 * evDay
	evDefaultDeadline  = 20 * evDay
	evMarketCycleLen   = 60 * evDay
	evRegulationMinAge = 90 * evDay
)

// DefaultEvents returns the v0 industry-event catalog (design §4).
func DefaultEvents() []EventSpec {
	twoChoice := func(id string, weight, cashRevMonths, cashFloor float64) EventSpec {
		return EventSpec{
			ID: id, Weight: weight,
			DurationSec: evDefaultDuration, DeadlineSec: evDefaultDeadline,
			NumChoices: 2, DefaultChoice: 1,
			CashCostRevMonths: cashRevMonths, CashCostFloor: cashFloor,
		}
	}
	chip := twoChoice(EvChipShortage, 1.0, 0.5, 20000)
	energy := twoChoice(EvEnergySpike, 1.0, 0.4, 15000)
	rivalBreak := twoChoice(EvRivalBreak, 0.8, 0.8, 30000)
	openSource := twoChoice(EvOpenSourceWar, 0.7, 0, 0) // choice 0 is free (跟進降價)
	scandal := twoChoice(EvRivalScandal, 0.8, 0.6, 25000)
	paper := twoChoice(EvPaper, 1.0, 0, 0)
	paper.RnDCostFrac = 0.25
	incident := twoChoice(EvIncident, 1.2, 1.0, 40000)
	regulation := twoChoice(EvRegulation, 0.6, 1.0, 50000)
	regulation.MinGameTime = evRegulationMinAge
	bubble := twoChoice(EvBubbleTalk, 0.6, 0.8, 50000)
	bubble.MinValuation = 5e8
	market := EventSpec{ID: EvMarketCycle, Weight: 0.9, DurationSec: evMarketCycleLen}
	return []EventSpec{
		chip, energy, rivalBreak, openSource, scandal,
		paper, incident, regulation, market, bubble,
	}
}

// EventByID looks up an event spec by ID within evs.
func EventByID(evs []EventSpec, id string) (EventSpec, bool) {
	for _, e := range evs {
		if e.ID == id {
			return e, true
		}
	}
	return EventSpec{}, false
}
