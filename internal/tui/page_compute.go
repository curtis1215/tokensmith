package tui

import (
	"fmt"

	"tokensmith/internal/sim"
)

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
		Bar(trainUtil, 12), trainUtil*100, trainCap)

	infBarStr := fmt.Sprintf("推理池: %s %.0f%% (有效算力 %.0f)",
		Bar(infUtil, 12), infUtil*100, infCap)
	if infUtil >= 0.9 {
		infBarStr = styleWarn.Render(infBarStr)
	}

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

	causalCard := Card("池狀態", VStack(causalLines...))

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
	procCard := Card(procTitle, VStack(procTableStr, "", helpStyle.Render(fmt.Sprintf("* 推理租金/張/秒；訓練池 ×%.3f", m.cfg.TrainRentMult))))

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
		fmt.Sprintf("電力: %s %s/%s kW", Bar(powerUtil, 10), human(usedPower), human(s.Datacenter.PowerCapacity)),
		fmt.Sprintf("空間: %s %s/%s", Bar(slotsUtil, 10), human(usedSlots), human(s.Datacenter.SlotCapacity)),
	)
	dcCard := Card("機房與電力", dcBody)

	// Combine columns (footer is fixed shell chrome)
	leftCol := VStack(causalCard, dcCard)
	return ResponsiveRow(m.width, 2, leftCol, procCard)
}
