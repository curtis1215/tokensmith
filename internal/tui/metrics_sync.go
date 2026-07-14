package tui

import (
	"time"

	"tokensmith/internal/metrics"
	"tokensmith/internal/sim"
)

// metricsFlushEveryTicks is ~30s at 250ms tick interval (120 * 250ms).
const metricsFlushEveryTicks = 120

// metricsSnapshotNow upserts stock KPIs for the local day of now.
// Open* freezes on the first write of each day (see metrics.UpsertSnapshot).
func (m *Model) metricsSnapshotNow(now time.Time) {
	day := metrics.DayKey(now)
	users := sim.TotalUsers(m.state)
	rev := sim.MonthlyRevenue(m.state)
	rnd := m.state.Resources.RnD
	metrics.UpsertSnapshot(&m.metricsDoc, day, users, rev, rnd, now.Unix())
}

// metricsMaybeRollDay flushes when the local calendar day changes so the
// previous day's stocks/inflows are persisted before a new day key is used.
// Inflow for the new day starts empty; callers snapshot stocks on the next tick.
func (m *Model) metricsMaybeRollDay(now time.Time) {
	day := metrics.DayKey(now)
	if day == m.metricsDay {
		return
	}
	if m.metricsDirty {
		m.metricsFlush()
	}
	m.metricsDay = day
}

// metricsFlush writes metricsDoc to disk when dirty. Nil store is a no-op.
// Clears dirty on success; optional notice on failure (does not block gameplay).
func (m *Model) metricsFlush() {
	if m.metricsStore == nil || !m.metricsDirty {
		return
	}
	if err := m.metricsStore.Save(m.metricsDoc); err != nil {
		m.setNotice("⚠ 指標歷史寫入失敗")
		return
	}
	m.metricsDirty = false
}
