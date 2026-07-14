package tui

import (
	"math"

	"tokensmith/internal/model"
	"tokensmith/internal/sim"
)

// displayAlpha is the exponential approach factor per tick (α ≈ 0.3).
const displayAlpha = 0.3

// tokenPulseTicks is how many ticks the token flash stays lit (~3s at the
// 250ms tick interval) before fading — long enough to actually notice,
// unlike the old 4-tick (~1s) blink-and-miss window.
const tokenPulseTicks = 12

// displayState holds presentation-only values that trail sim truth for smooth
// motion. It must never feed back into sim tick or Apply.
type displayState struct {
	Cash, RnD, Valuation float64
	TotalUsers           float64
	TrainUtil, InfUtil   float64
	// ModelUsers parallels state.Models[i].Users for detail views.
	ModelUsers []float64
	// ConsumerShares parallels top SegmentShareBars for SegConsumer.
	ConsumerShares []float64
	PulseToken     int
	PulseNotice    int
}

func lerp(a, b, α float64) float64 {
	return a + α*(b-a)
}

// approachScalar moves cur toward target by α, snapping when within eps.
func approachScalar(cur, target, α, eps float64) float64 {
	v := lerp(cur, target, α)
	if math.Abs(v-target) <= eps {
		return target
	}
	// Near-zero absolute values: snap to avoid lingering float noise.
	if math.Abs(target) < eps && math.Abs(v) < eps*10 {
		return target
	}
	return v
}

func (d *displayState) approach(truth displayState, α float64) {
	d.Cash = approachScalar(d.Cash, truth.Cash, α, 0.01)
	d.RnD = approachScalar(d.RnD, truth.RnD, α, 0.01)
	d.Valuation = approachScalar(d.Valuation, truth.Valuation, α, 0.01)
	d.TotalUsers = approachScalar(d.TotalUsers, truth.TotalUsers, α, 0.5)
	d.TrainUtil = approachScalar(d.TrainUtil, truth.TrainUtil, α, 1e-4)
	d.InfUtil = approachScalar(d.InfUtil, truth.InfUtil, α, 1e-4)

	// Resize model-user slice if model count changed; new slots snap to truth.
	if len(d.ModelUsers) != len(truth.ModelUsers) {
		d.ModelUsers = make([]float64, len(truth.ModelUsers))
		copy(d.ModelUsers, truth.ModelUsers)
	} else {
		for i := range truth.ModelUsers {
			d.ModelUsers[i] = approachScalar(d.ModelUsers[i], truth.ModelUsers[i], α, 0.5)
		}
	}

	if len(d.ConsumerShares) != len(truth.ConsumerShares) {
		d.ConsumerShares = make([]float64, len(truth.ConsumerShares))
		copy(d.ConsumerShares, truth.ConsumerShares)
	} else {
		for i := range truth.ConsumerShares {
			d.ConsumerShares[i] = approachScalar(d.ConsumerShares[i], truth.ConsumerShares[i], α, 1e-4)
		}
	}
	// Pulse counters are managed separately on the tick path.
}

func (d *displayState) snap(truth displayState) {
	pulseT, pulseN := d.PulseToken, d.PulseNotice
	*d = truth
	// Deep-copy slices so later truth rebuilds don't alias.
	if truth.ModelUsers != nil {
		d.ModelUsers = append([]float64(nil), truth.ModelUsers...)
	}
	if truth.ConsumerShares != nil {
		d.ConsumerShares = append([]float64(nil), truth.ConsumerShares...)
	}
	d.PulseToken = pulseT
	d.PulseNotice = pulseN
}

// truthDisplay builds the instantaneous display values from sim truth.
func truthDisplay(m Model) displayState {
	s := m.state
	trainUtil := 0.0
	if s.HasTraining {
		trainUtil = 1
	}
	infUtil := 0.0
	if cap := sim.EffectiveInference(s, m.cfg); cap > 0 {
		infUtil = s.Compute.InferenceLoad / cap
	}
	modelUsers := make([]float64, len(s.Models))
	for i, md := range s.Models {
		modelUsers[i] = md.Users
	}
	bars := sim.SegmentShareBars(s, m.cfg, model.SegConsumer)
	shares := make([]float64, len(bars))
	for i, b := range bars {
		shares[i] = b.Share
	}
	return displayState{
		Cash:           s.Resources.Cash,
		RnD:            s.Resources.RnD,
		Valuation:      sim.Valuation(s, m.cfg),
		TotalUsers:     sim.TotalUsers(s),
		TrainUtil:      trainUtil,
		InfUtil:        infUtil,
		ModelUsers:     modelUsers,
		ConsumerShares: shares,
	}
}

// advanceDisplay updates displayState after a sim tick.
func (m *Model) advanceDisplay() {
	prevCash := m.disp.Cash
	wasReady := m.dispReady
	truth := truthDisplay(*m)
	if !m.dispReady {
		m.disp.snap(truth)
		m.dispReady = true
	} else {
		m.disp.approach(truth, displayAlpha)
	}
	if wasReady {
		instant := (m.disp.Cash - prevCash) * ticksPerRealSec
		m.cashRate = approachScalar(m.cashRate, instant, displayAlpha, 0.001)
	}
	if m.tokensThisTick {
		m.disp.PulseToken = tokenPulseTicks
	} else if m.disp.PulseToken > 0 {
		m.disp.PulseToken--
	}
	if m.disp.PulseNotice > 0 {
		m.disp.PulseNotice--
	}
	m.sparkTick++
	if m.sparkTick%4 == 0 {
		m.sparkValuation.push(m.disp.Valuation)
		m.sparkUsers.push(m.disp.TotalUsers)
		m.sparkRnD.push(sim.RnDRatePerSec(m.state, m.cfg) * gameSecPerRealSec)
		// Dashboard short-window stocks (capacity 120; ~1s sample interval).
		users := sim.TotalUsers(m.state)
		if m.dispReady {
			users = m.disp.TotalUsers
		}
		m.dashUsers.push(users)
		m.dashRevenue.push(sim.MonthlyRevenue(m.state))
		m.dashRnDStock.push(m.state.Resources.RnD)
	}
	if len(m.banners) > 0 {
		m.bannerTicks--
		if m.bannerTicks <= 0 {
			m.banners = m.banners[1:]
			if len(m.banners) > 0 {
				m.bannerTicks = bannerShowTicks
			}
		}
	}
}

// snapDisplay forces display to match truth (restart / prestige / load).
func (m *Model) snapDisplay() {
	m.disp.snap(truthDisplay(*m))
	m.dispReady = true
}

// setNotice sets a transient notice and arms its short highlight pulse.
func (m *Model) setNotice(msg string) {
	m.notice = msg
	if msg != "" {
		m.disp.PulseNotice = 4
	}
}
