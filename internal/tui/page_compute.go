package tui

import (
	"fmt"
	"strings"

	"tokensmith/internal/sim"
)

func renderCompute(m Model) string {
	s := m.state
	var b strings.Builder

	// Effective pool capacities (self-built + rented, engineer/tech/star mults applied).
	trainCap := sim.EffectiveTraining(s, m.cfg)
	trainUtil := 0.0
	if s.HasTraining {
		trainUtil = 1 // a job fully occupies the training pool in v0
	}
	b.WriteString(fmt.Sprintf("訓練池  %s %.0f%%   有效算力 %.0f\n",
		progressBar(trainUtil, 12), trainUtil*100, trainCap))

	infCap := sim.EffectiveInference(s, m.cfg)
	infUtil := 0.0
	if infCap > 0 {
		infUtil = s.Compute.InferenceLoad / infCap
	}
	warn := ""
	if infUtil >= 0.9 {
		warn = " ⚠接近上限"
	}
	b.WriteString(fmt.Sprintf("推理池  %s %.0f%%   有效算力 %.0f%s\n",
		progressBar(infUtil, 12), infUtil*100, infCap, warn))

	// Process-node table: cursor, compute/rent, per-process rented counts, lock state.
	b.WriteString(fmt.Sprintf("\n製程算力                                可用 R&D %s\n", human(s.Resources.RnD)))
	b.WriteString("              算力  租金/秒*  訓練張  推理張  解鎖\n")
	for i, p := range m.cfg.Processes {
		cursor := " "
		if i == m.procCursor {
			cursor = "▸"
		}
		unlockState := "✓"
		if !sim.ProcessUnlocked(s, m.cfg, p.ID) {
			unlockState = "🔒需 " + p.UnlockTech
		}
		rentedT := s.Compute.RentedTraining[p.ID]
		rentedI := s.Compute.RentedInference[p.ID]
		b.WriteString(fmt.Sprintf("%s %-8s %4.0f  $%.4f   %5d   %5d   %s\n",
			cursor, p.Name, p.Compute, p.RentPerSec, rentedT, rentedI, unlockState))
	}
	b.WriteString(helpStyle.Render(fmt.Sprintf("* 推理租金/張/秒；訓練池 ×%.3f\n", m.cfg.TrainRentMult)))

	// Datacenter power / space.
	var usedPower, usedSlots float64
	for _, sv := range s.Servers {
		usedPower += sv.PowerKW
		usedSlots += sv.Slots
	}
	b.WriteString(fmt.Sprintf("\n機房  電力 %.0f/%.0f kW · 空間 %.0f/%.0f\n",
		usedPower, s.Datacenter.PowerCapacity, usedSlots, s.Datacenter.SlotCapacity))

	b.WriteString(helpStyle.Render("\n[↑↓]選製程 [r/R]±訓練 [i/I]±推理 [b/B]建訓練/推理伺服器 [e]擴機房 [Tab]切頁"))
	return b.String()
}
