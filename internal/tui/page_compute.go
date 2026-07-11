package tui

import (
	"fmt"

	"tokensmith/internal/sim"
)

// renderComputeFrontierLines shows training-pool split for frontier research.
func renderComputeFrontierLines(m Model) []string {
	v := sim.FrontierProgressView(m.state, m.cfg)
	if !v.Active {
		return []string{styleMuted.Render("前沿研究 未啟動（科技頁）")}
	}
	lines := []string{
		fmt.Sprintf("前沿 Gen%d  分配 %d%% / 訓練 %d%%", v.TargetGen, v.AllocationPct, v.ModelAllocationPct),
		fmt.Sprintf("有效%.0f → 折合%.0f · 建議%.0f", v.AllocatedCompute, v.DiminishedCompute, v.RecommendedCompute),
		fmt.Sprintf("進度 %s %.0f%%", Bar(v.WorkFraction, 10), v.WorkFraction*100),
	}
	if v.UnavailableReason != "" {
		lines = append(lines, styleWarn.Render("停滯 · "+frontierStallCopy(v.UnavailableReason)))
	} else if v.ETASec > 0 {
		lines = append(lines, KV("ETA", formatETASec(v.ETASec)))
	}
	return lines
}

func renderCompute(m Model) string {
	s := m.state

	// 1. Causal Card ("池狀態")
	trainCap := sim.EffectiveTraining(s, m.cfg)
	trainUtil := 0.0
	if s.HasTraining {
		trainUtil = 1.0
	}
	infCap := sim.EffectiveInference(s, m.cfg)
	infUtil := 0.0
	if infCap > 0 {
		infUtil = s.Compute.InferenceLoad / infCap
	}
	if m.dispReady {
		trainUtil, infUtil = m.disp.TrainUtil, m.disp.InfUtil
	}
	trainBarStr := fmt.Sprintf("訓練池: %s %.0f%% (有效算力 %.0f)",
		LoadBar(trainUtil, 12), trainUtil*100, trainCap)

	infBarStr := fmt.Sprintf("推理池: %s %.0f%% (有效算力 %.0f)",
		LoadBar(infUtil, 12), infUtil*100, infCap)

	servable := sim.ServableUsers(s, m.cfg)
	totalUsers := sim.TotalUsers(s)
	if m.dispReady {
		totalUsers = m.disp.TotalUsers
	}

	causalLines := []string{
		trainBarStr,
		infBarStr,
		"",
		KV("可撐用戶", fmt.Sprintf("~%s", human(servable))),
		KV("現況用戶", human(totalUsers)),
	}

	if infCap == 0 {
		causalLines = append(causalLines, styleAccent.Render("未配置推理 · grace（不因超載砍用戶）"))
	} else if totalUsers > servable {
		causalLines = append(causalLines, styleWarn.Render("超載 · 建議加推理"))
	}

	// Frontier allocation (authoritative sim view; keys for rent/build unchanged).
	causalLines = append(causalLines, "")
	causalLines = append(causalLines, renderComputeFrontierLines(m)...)

	causalCard := CardIn(CardDefault, 0, "池狀態", VStack(causalLines...))

	// 2. Process table card
	var procLines []string
	procLines = append(procLines, "              算力  租金/秒*  訓練張  推理張  解鎖")
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
		procLines = append(procLines, fmt.Sprintf("%s %-8s %4.0f  $%.4f   %5d   %5d   %s",
			cursor, p.Name, p.Compute, p.RentPerSec, rentedT, rentedI, unlockState))
	}
	procTableStr := VStack(procLines...)
	procTitle := fmt.Sprintf("製程算力 (可用 R&D: %s)", human(s.Resources.RnD))
	procCard := CardIn(CardDefault, 0, procTitle, VStack(procTableStr, "", helpStyle.Render(fmt.Sprintf("* 推理租金/張/秒；訓練池 ×%.3f", m.cfg.TrainRentMult))))

	// 3. Datacenter power & space
	var usedPower, usedSlots float64
	for _, sv := range s.Servers {
		usedPower += sv.PowerKW
		usedSlots += sv.Slots
	}

	powerUtil := 0.0
	if s.Datacenter.PowerCapacity > 0 {
		powerUtil = usedPower / s.Datacenter.PowerCapacity
	}
	slotsUtil := 0.0
	if s.Datacenter.SlotCapacity > 0 {
		slotsUtil = usedSlots / s.Datacenter.SlotCapacity
	}

	dcBody := VStack(
		fmt.Sprintf("電力: %s %s/%s kW", LoadBar(powerUtil, 10), human(usedPower), human(s.Datacenter.PowerCapacity)),
		fmt.Sprintf("空間: %s %s/%s", LoadBar(slotsUtil, 10), human(usedSlots), human(s.Datacenter.SlotCapacity)),
	)
	dcCard := CardIn(CardDefault, 0, "機房與電力", dcBody)

	// Combine columns (footer is fixed shell chrome)
	leftCol := VStack(causalCard, dcCard)
	return ResponsiveRow(m.contentWidth(), 2, leftCol, procCard)
}
