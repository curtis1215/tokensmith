package tui

import (
	"fmt"
	"strings"

	"tokensmith/internal/model"
	"tokensmith/internal/sim"
)

func renderCompute(m Model) string {
	s := m.state
	var b strings.Builder

	// Training pool: a job fully occupies it in v0.
	trainCap := sim.EffectiveTraining(s, m.cfg)
	trainUtil := 0.0
	if s.HasTraining {
		trainUtil = 1
	}
	b.WriteString(fmt.Sprintf("訓練池  %s %.0f%%   容量 %.0f GPU\n",
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
	b.WriteString(fmt.Sprintf("推理池  %s %.0f%%   容量 %.0f GPU%s\n",
		progressBar(infUtil, 12), infUtil*100, infCap, warn))

	// Rent vs self-build split.
	var selfTrain, selfInf float64
	for _, sv := range s.Servers {
		if sv.Pool == model.PoolTraining {
			selfTrain += sv.Compute
		} else {
			selfInf += sv.Compute
		}
	}
	b.WriteString(fmt.Sprintf("\n自建  訓練 %.0f · 推理 %.0f （伺服器 %d 台）\n",
		selfTrain, selfInf, len(s.Servers)))

	// Datacenter power / space.
	var usedPower, usedSlots float64
	for _, sv := range s.Servers {
		usedPower += sv.PowerKW
		usedSlots += sv.Slots
	}
	b.WriteString(fmt.Sprintf("機房  電力 %.0f/%.0f kW · 空間 %.0f/%.0f\n",
		usedPower, s.Datacenter.PowerCapacity, usedSlots, s.Datacenter.SlotCapacity))

	// Chip market.
	b.WriteString("\n晶片市場\n")
	for _, ch := range m.cfg.Chips {
		pool := "推理"
		if ch.Pool == model.PoolTraining {
			pool = "訓練"
		}
		b.WriteString(fmt.Sprintf("  %-12s %s · 算力 %.0f · %.0fkW · $%s\n",
			ch.Name, pool, ch.Compute, ch.PowerKW, human(ch.Price)))
	}

	b.WriteString(helpStyle.Render("\n[r/R]±訓練 [i/I]±推理 [b]組伺服器 [e]擴機房 [Tab]切頁"))
	return b.String()
}
