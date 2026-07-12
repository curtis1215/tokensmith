package sim

import (
	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

// nextRand advances a splitmix64 state and returns the new state plus a
// uniform float64 in [0,1). All event randomness flows through this so the
// sim stays deterministic: same GameState → same rolls.
func nextRand(state uint64) (uint64, float64) {
	state += 0x9E3779B97F4A7C15
	z := state
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	z ^= z >> 31
	return state, float64(z>>11) / float64(1<<53)
}

// eventEffects folds all active event modifiers into one multiplier set
// (neutral when none). TechCostMult is branch-targeted and deliberately NOT
// aggregated here — use eventTechCostMult.
func eventEffects(ns model.GameState, b balance.Config) model.EventEffects {
	agg := model.NeutralEventEffects()
	for _, m := range ns.Events.Active {
		agg.BuildCostMult *= m.Effects.BuildCostMult
		agg.PowerCostMult *= m.Effects.PowerCostMult
		agg.RefPriceMult *= m.Effects.RefPriceMult
		agg.UserGrowthMult *= m.Effects.UserGrowthMult
		agg.TAMMult *= m.Effects.TAMMult
		agg.ValuationMult *= m.Effects.ValuationMult
		agg.SafetyWeightMult *= m.Effects.SafetyWeightMult
		agg.IncidentChanceMult *= m.Effects.IncidentChanceMult
	}
	return agg
}

// eventTechCostMult is the product of active TechCostMult modifiers that
// target the given tech branch.
func eventTechCostMult(ns model.GameState, branch model.TechBranch) float64 {
	mult := 1.0
	for _, m := range ns.Events.Active {
		if m.Effects.TechCostMult != 1 && m.Target == int(branch) {
			mult *= m.Effects.TechCostMult
		}
	}
	return mult
}

// EventChoiceCost returns the cash and R&D cost of a spec's paid option
// (choice 0). Cash scales with revenue so late-game events stay meaningful.
func EventChoiceCost(ns model.GameState, spec balance.EventSpec) (cash, rnd float64) {
	cash = spec.CashCostRevMonths * MonthlyRevenue(ns)
	if cash < spec.CashCostFloor {
		cash = spec.CashCostFloor
	}
	rnd = spec.RnDCostFrac * ns.Resources.RnD
	return cash, rnd
}

// appendLog appends rec to a cloned log, dropping the oldest past cap.
func appendLog(log []model.EventRecord, rec model.EventRecord, cap int) []model.EventRecord {
	out := append(append([]model.EventRecord(nil), log...), rec)
	if cap > 0 && len(out) > cap {
		out = out[len(out)-cap:]
	}
	return out
}

// addModifier appends a modifier to a cloned Active slice.
func addModifier(active []model.ActiveModifier, m model.ActiveModifier) []model.ActiveModifier {
	return append(append([]model.ActiveModifier(nil), active...), m)
}

// removeModifier drops all modifiers with the given event ID (cloned).
func removeModifier(active []model.ActiveModifier, id string) []model.ActiveModifier {
	out := make([]model.ActiveModifier, 0, len(active))
	for _, m := range active {
		if m.EventID != id {
			out = append(out, m)
		}
	}
	return out
}

// replaceModifierEffects swaps the effects of the modifier with the given ID (cloned).
func replaceModifierEffects(active []model.ActiveModifier, id string, e model.EventEffects) []model.ActiveModifier {
	out := append([]model.ActiveModifier(nil), active...)
	for i := range out {
		if out[i].EventID == id {
			out[i].Effects = e
		}
	}
	return out
}

// strongestRival returns the competitor index with the highest capability.
func strongestRival(ns model.GameState) int {
	best, idx := -1.0, -1
	for i, c := range ns.Competitors {
		if c.Quality[model.DimCapability] > best {
			best, idx = c.Quality[model.DimCapability], i
		}
	}
	return idx
}

// scaleRivalDim multiplies one quality dim of one competitor (cloned).
func scaleRivalDim(ns model.GameState, idx int, dim model.QualityDim, mult float64) model.GameState {
	if idx < 0 || idx >= len(ns.Competitors) {
		return ns
	}
	comps := append([]model.Competitor(nil), ns.Competitors...)
	comps[idx].Quality[dim] *= mult
	ns.Competitors = comps
	return ns
}

// scalePlayerUsers multiplies online-model users: entMult for enterprise
// models, mult for the rest (cloned).
func scalePlayerUsers(ns model.GameState, mult, entMult float64) model.GameState {
	models := append([]model.Model(nil), ns.Models...)
	for i := range models {
		if !models[i].Online {
			continue
		}
		if models[i].Segment == model.SegEnterprise {
			models[i].Users *= entMult
		} else {
			models[i].Users *= mult
		}
	}
	ns.Models = models
	return ns
}

// fireEvent applies a triggered event's fire-time effects: rolls its target
// (advancing RandState), applies one-shots, adds sustained modifiers, and
// either queues a pending choice or logs a no-choice event. Pure.
func fireEvent(ns model.GameState, spec balance.EventSpec, b balance.Config) model.GameState {
	now := ns.GameTime
	target := -1
	mod := func(set func(e *model.EventEffects)) model.ActiveModifier {
		e := model.NeutralEventEffects()
		set(&e)
		return model.ActiveModifier{EventID: spec.ID, ExpiresAt: now + spec.DurationSec, Target: target, Effects: e}
	}
	pending := true
	switch spec.ID {
	case balance.EvChipShortage:
		ns.Events.Active = addModifier(ns.Events.Active, mod(func(e *model.EventEffects) {
			e.BuildCostMult = balance.EvChipShortageBuildMult
		}))
	case balance.EvEnergySpike:
		var r float64
		ns.Events.RandState, r = nextRand(ns.Events.RandState)
		if r < 0.5 { // price spike: player may pay to lock the old rate
			target = 0
			ns.Events.Active = addModifier(ns.Events.Active, mod(func(e *model.EventEffects) {
				e.PowerCostMult = balance.EvEnergyUpMult
			}))
		} else { // price drop: pure upside, no decision
			target = 1
			pending = false
			ns.Events.Active = addModifier(ns.Events.Active, mod(func(e *model.EventEffects) {
				e.PowerCostMult = balance.EvEnergyDownMult
			}))
		}
	case balance.EvRivalBreak:
		target = strongestRival(ns)
		ns = scaleRivalDim(ns, target, model.DimCapability, 1+balance.EvRivalBreakQualityPct)
	case balance.EvOpenSourceWar:
		ns.Events.Active = addModifier(ns.Events.Active, mod(func(e *model.EventEffects) {
			e.RefPriceMult = balance.EvOpenSourceRefPrice
		}))
	case balance.EvRivalScandal:
		// Weighted pick by (1 - safety/100): low-safety rivals are likelier.
		weights := make([]float64, len(ns.Competitors))
		total := 0.0
		for i, c := range ns.Competitors {
			w := 1 - c.Quality[model.DimSafety]/100
			if w < 0.05 {
				w = 0.05
			}
			weights[i] = w
			total += w
		}
		var r float64
		ns.Events.RandState, r = nextRand(ns.Events.RandState)
		pick := r * total
		for i, w := range weights {
			pick -= w
			if pick <= 0 {
				target = i
				break
			}
		}
		if target < 0 && len(ns.Competitors) > 0 {
			target = len(ns.Competitors) - 1
		}
		ns = scaleRivalDim(ns, target, model.DimSafety, 1-balance.EvScandalSafetyPct)
	case balance.EvPaper:
		var r float64
		ns.Events.RandState, r = nextRand(ns.Events.RandState)
		target = int(r * float64(model.NumBranches))
		if target >= model.NumBranches {
			target = model.NumBranches - 1
		}
	case balance.EvIncident:
		// User loss is deferred to resolve so the choice governs severity.
	case balance.EvRegulation:
		ns.Events.Active = addModifier(ns.Events.Active, mod(func(e *model.EventEffects) {
			e.SafetyWeightMult = balance.EvRegulationSafetyW
		}))
	case balance.EvMarketCycle:
		pending = false
		var r float64
		ns.Events.RandState, r = nextRand(ns.Events.RandState)
		tam := balance.EvMarketBoomTAM
		target = 0
		if r < 0.5 {
			tam = balance.EvMarketBustTAM
			target = 1
		}
		ns.Events.Active = addModifier(ns.Events.Active, mod(func(e *model.EventEffects) {
			e.TAMMult = tam
		}))
	case balance.EvBubbleTalk:
		ns.Events.Active = addModifier(ns.Events.Active, mod(func(e *model.EventEffects) {
			e.ValuationMult = balance.EvBubbleValuation
		}))
	default:
		return ns // unknown ID (catalog drift): skip, never panic
	}
	ns.Events.FiredCount++
	if spec.NumChoices > 0 && pending {
		ns.Events.Pending = append(append([]model.PendingEvent(nil), ns.Events.Pending...),
			model.PendingEvent{EventID: spec.ID, Target: target, FiredAt: now, Deadline: now + spec.DeadlineSec})
	} else {
		ns.Events.Log = appendLog(ns.Events.Log,
			model.EventRecord{EventID: spec.ID, At: now, Choice: 0, Auto: false}, b.EventLogCap)
	}
	return ns
}

// resolveChoice applies choice to Pending[pendingIndex]: charges the paid
// option's costs, applies choice effects per event, consumes the pending
// entry, and records history. auto marks timeout/offline resolution. Pure.
func resolveChoice(ns model.GameState, pendingIndex, choice int, auto bool, b balance.Config) (model.GameState, error) {
	if pendingIndex < 0 || pendingIndex >= len(ns.Events.Pending) {
		return ns, ErrInvalidEventIndex
	}
	p := ns.Events.Pending[pendingIndex]
	spec, ok := balance.EventByID(b.Events, p.EventID)
	if !ok {
		// Catalog drift (save from another version): drop the entry silently.
		ns.Events.Pending = removePending(ns.Events.Pending, pendingIndex)
		return ns, nil
	}
	if choice < 0 || choice >= spec.NumChoices {
		return ns, ErrInvalidEventChoice
	}
	if choice == 0 {
		cash, rnd := EventChoiceCost(ns, spec)
		if ns.Resources.Cash < cash {
			return ns, ErrInsufficientCash
		}
		if ns.Resources.RnD < rnd {
			return ns, ErrInsufficientRnD
		}
		ns.Resources.Cash -= cash
		ns.Resources.RnD -= rnd
	}
	now := ns.GameTime
	mod := func(target int, set func(e *model.EventEffects)) model.ActiveModifier {
		e := model.NeutralEventEffects()
		set(&e)
		return model.ActiveModifier{EventID: spec.ID, ExpiresAt: now + spec.DurationSec, Target: target, Effects: e}
	}
	switch spec.ID {
	case balance.EvChipShortage, balance.EvEnergySpike:
		if choice == 0 {
			ns.Events.Active = removeModifier(ns.Events.Active, spec.ID)
		}
	case balance.EvRivalBreak:
		if choice == 0 {
			ns.Events.Active = addModifier(ns.Events.Active, mod(-1, func(e *model.EventEffects) {
				e.UserGrowthMult = balance.EvRivalBreakPromoGrowth
			}))
		}
	case balance.EvOpenSourceWar:
		if choice == 0 {
			e := model.NeutralEventEffects()
			e.RefPriceMult = balance.EvOpenSourceFollowRef
			e.UserGrowthMult = balance.EvOpenSourceFollowGrow
			ns.Events.Active = replaceModifierEffects(ns.Events.Active, spec.ID, e)
		}
	case balance.EvRivalScandal:
		growth := balance.EvScandalWatchGrowth
		if choice == 0 {
			growth = balance.EvScandalPoachGrowth
		}
		g := growth
		ns.Events.Active = addModifier(ns.Events.Active, mod(-1, func(e *model.EventEffects) {
			e.UserGrowthMult = g
		}))
	case balance.EvPaper:
		cost := balance.EvPaperAbsorbTechCost
		if choice == 0 {
			cost = balance.EvPaperBetTechCost
		}
		cm := cost
		ns.Events.Active = addModifier(ns.Events.Active, mod(p.Target, func(e *model.EventEffects) {
			e.TechCostMult = cm
		}))
	case balance.EvIncident:
		loss, entLoss := balance.EvIncidentLossPct, balance.EvIncidentEnterprisePct
		if choice == 0 { // public apology halves the loss, no aftermath
			loss, entLoss = loss/2, entLoss/2
		}
		ns = scalePlayerUsers(ns, 1-loss, 1-entLoss)
		if choice == 1 { // 低調: lingering elevated incident chance
			ns.Events.Active = addModifier(ns.Events.Active, mod(-1, func(e *model.EventEffects) {
				e.IncidentChanceMult = balance.EvIncidentQuietChance
			}))
		}
	case balance.EvRegulation:
		if choice == 0 {
			models := append([]model.Model(nil), ns.Models...)
			for i := range models {
				models[i].Quality[model.DimSafety] *= 1 + balance.EvRegulationComplyPct
			}
			ns.Models = models
		}
	case balance.EvBubbleTalk:
		if choice == 0 {
			e := model.NeutralEventEffects()
			e.ValuationMult = balance.EvBubbleCalmValuation
			ns.Events.Active = replaceModifierEffects(ns.Events.Active, spec.ID, e)
		}
	}
	ns.Events.Pending = removePending(ns.Events.Pending, pendingIndex)
	ns.Events.Log = appendLog(ns.Events.Log,
		model.EventRecord{EventID: spec.ID, At: now, Choice: choice, Auto: auto}, b.EventLogCap)
	if auto {
		ns.Events.AutoCount++
	}
	return ns, nil
}

// removePending drops index i from a cloned pending slice.
func removePending(pending []model.PendingEvent, i int) []model.PendingEvent {
	out := make([]model.PendingEvent, 0, len(pending)-1)
	out = append(out, pending[:i]...)
	return append(out, pending[i+1:]...)
}

// advanceEvents is the per-tick event step: expire modifiers, auto-resolve
// overdue pending choices to their free default, then roll for a new trigger
// when the check timer is due. Called by Tick after GameTime advances. Pure.
func advanceEvents(ns model.GameState, b balance.Config) model.GameState {
	now := ns.GameTime
	// 1. Expire sustained modifiers.
	if len(ns.Events.Active) > 0 {
		kept := make([]model.ActiveModifier, 0, len(ns.Events.Active))
		for _, m := range ns.Events.Active {
			if m.ExpiresAt > now {
				kept = append(kept, m)
			}
		}
		ns.Events.Active = kept
	}
	// 2. Auto-resolve overdue pending events with their free default choice.
	for i := 0; i < len(ns.Events.Pending); {
		p := ns.Events.Pending[i]
		if p.Deadline > now {
			i++
			continue
		}
		spec, ok := balance.EventByID(b.Events, p.EventID)
		if !ok { // catalog drift: drop silently
			ns.Events.Pending = removePending(ns.Events.Pending, i)
			continue
		}
		var err error
		ns, err = resolveChoice(ns, i, spec.DefaultChoice, true, b)
		if err != nil { // defensive: the default choice is free and always valid
			i++
		}
	}
	// 3. Trigger roll(s) — loop covers large offline chunks crossing a check.
	if b.EventCheckSec <= 0 {
		return ns
	}
	if ns.Events.NextCheckAt == 0 {
		// Fresh run or pre-events save: schedule the first roll, no fire.
		ns.Events.NextCheckAt = now + b.EventCheckSec
		return ns
	}
	for ns.Events.NextCheckAt <= now {
		var hit, jitter float64
		ns.Events.RandState, hit = nextRand(ns.Events.RandState)
		ns.Events.RandState, jitter = nextRand(ns.Events.RandState)
		ns.Events.NextCheckAt += b.EventCheckSec * (0.75 + 0.5*jitter)
		if hit >= b.EventHitChance {
			continue
		}
		specs, weights, total := eligibleEvents(ns, b)
		if total <= 0 {
			continue
		}
		var pick float64
		ns.Events.RandState, pick = nextRand(ns.Events.RandState)
		x := pick * total
		for k, w := range weights {
			x -= w
			if x <= 0 {
				ns = fireEvent(ns, specs[k], b)
				break
			}
		}
	}
	return ns
}

// eligibleEvents returns the specs currently allowed to fire with their
// state-adjusted weights: gates passed, not pending, not active, and past
// the per-event cooldown since its last history record.
func eligibleEvents(ns model.GameState, b balance.Config) (specs []balance.EventSpec, weights []float64, total float64) {
	now := ns.GameTime
	for _, spec := range b.Events {
		if now < spec.MinGameTime || ns.PeakValuation < spec.MinValuation {
			continue
		}
		if hasPending(ns, spec.ID) || hasActive(ns, spec.ID) || inCooldown(ns, spec.ID, b.EventCooldownSec) {
			continue
		}
		w := eventWeight(ns, spec, b)
		if w <= 0 {
			continue
		}
		specs = append(specs, spec)
		weights = append(weights, w)
		total += w
	}
	return specs, weights, total
}

func hasPending(ns model.GameState, id string) bool {
	for _, p := range ns.Events.Pending {
		if p.EventID == id {
			return true
		}
	}
	return false
}

func hasActive(ns model.GameState, id string) bool {
	for _, m := range ns.Events.Active {
		if m.EventID == id {
			return true
		}
	}
	return false
}

func inCooldown(ns model.GameState, id string, cooldown float64) bool {
	for _, rec := range ns.Events.Log {
		if rec.EventID == id && ns.GameTime-rec.At < cooldown {
			return true
		}
	}
	return false
}

// avgOnlineSafety is the mean safety quality across online models (0 if none).
func avgOnlineSafety(ns model.GameState) float64 {
	var sum float64
	var n int
	for _, m := range ns.Models {
		if m.Online {
			sum += m.Quality[model.DimSafety]
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

// eventWeight is the state-adjusted trigger weight of one event (design §3.2):
// incident chance scales with low model safety × alignment tech × lingering
// aftermath; paper chance scales with research headcount.
func eventWeight(ns model.GameState, spec balance.EventSpec, b balance.Config) float64 {
	switch spec.ID {
	case balance.EvIncident:
		avg := avgOnlineSafety(ns)
		if avg <= 0 {
			return 0 // nothing online → nothing to break
		}
		f := 1 - avg/balance.EvIncidentSafetyRef
		if f <= 0 {
			return 0 // safe enough: incidents effectively off
		}
		return spec.Weight * f * techEffects(ns, b).IncidentMult * eventEffects(ns, b).IncidentChanceMult
	case balance.EvPaper:
		// Weight paper events by roster size (research staff was per-tier counts).
		n := len(ns.Employees)
		m := 1 + float64(n)*0.1
		if m > 3 {
			m = 3
		}
		return spec.Weight * m
	default:
		return spec.Weight
	}
}
