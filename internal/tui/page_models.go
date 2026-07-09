package tui

import (
	"fmt"
	"strings"

	"tokensmith/internal/model"
	"tokensmith/internal/sim"
)

func renderModels(m Model) string {
	s := m.state
	if len(s.Models) == 0 {
		body := "（無 — 按 t 訓練第一個模型）"
		listCard := Card("模型列表", body)
		return VStack(listCard, Footer("[t]訓練"))
	}

	// 1. List card
	var b strings.Builder
	draftN := 0
	for _, md := range s.Models {
		if sim.IsDraft(md) {
			draftN++
		}
	}
	if draftN > 0 {
		b.WriteString(fmt.Sprintf("有 %d 個待發佈\n", draftN))
	}
	b.WriteString("── 待發佈 ──\n")
	anyDraft := false
	for i, md := range s.Models {
		if !sim.IsDraft(md) {
			continue
		}
		anyDraft = true
		cur := "  "
		if i == m.modelCursor {
			cur = "▸ "
		}
		b.WriteString(fmt.Sprintf("%s[%d] Gen%d · %s · 能力 %.0f\n",
			cur, i, md.Gen, segmentName(md.Segment), md.Quality[model.DimCapability]))
	}
	if !anyDraft {
		b.WriteString("  （無）\n")
	}
	b.WriteString("── 營運中 ──\n")
	anyLive := false
	for i, md := range s.Models {
		if sim.IsDraft(md) {
			continue
		}
		anyLive = true
		cur := "  "
		if i == m.modelCursor {
			cur = "▸ "
		}
		name := md.Name
		if name == "" {
			name = "（未命名）"
		}
		status := "上線"
		if !md.Online {
			status = "離線"
		}
		b.WriteString(fmt.Sprintf("%s[%d] 「%s」 Gen%d · %s · 用戶 %s · $%.0f · %s\n",
			cur, i, name, md.Gen, segmentName(md.Segment), human(md.Users), md.Price, status))
	}
	if !anyLive {
		b.WriteString("  （無）\n")
	}
	listCard := Card("模型列表", b.String())

	// 2. Detail card
	detailCard := renderModelDetail(m, m.modelCursor)

	// Combine into horizontal row plus footer
	row := ResponsiveRow(m.width, 2, listCard, detailCard)
	return VStack(row, Footer("[↑↓]選模型 [p]發佈 [t]訓練 [$]改價"))
}

func renderModelDetail(m Model, idx int) string {
	if idx < 0 || idx >= len(m.state.Models) {
		return Card("模型詳情", "無選取模型")
	}
	md := m.state.Models[idx]
	capVal := m.cfg.GenQualityCap[md.Gen]

	// 1. Quality Dims
	qNames := [4]string{"能力", "成本", "安全", "速度"}
	var qLines []string
	for d := 0; d < 4; d++ {
		val := md.Quality[d]
		var barStr string
		if capVal > 0 {
			barStr = Bar(val/capVal, 8)
		} else {
			barStr = fmt.Sprintf("%.0f (無上限)", val)
		}
		qLines = append(qLines, fmt.Sprintf("%s: %s %.0f", qNames[d], barStr, val))
	}
	qualityBlock := VStack(qLines...)

	// 2. Info Block
	var infoLines []string
	if md.Online {
		est := sim.EstimateUserTarget(m.state, idx, md.Price, m.cfg)
		monthly := md.Users * md.Price
		loadContrib := md.Users * m.cfg.InferenceLoadPerUser

		infCap := sim.EffectiveInference(m.state, m.cfg)
		util := 0.0
		if infCap > 0 {
			util = m.state.Compute.InferenceLoad / infCap
		}

		infoLines = append(infoLines,
			KV("用戶數", fmt.Sprintf("%s / 預估上限 ~%s", human(md.Users), human(est))),
			KV("月營收", fmt.Sprintf("$%s", human(monthly))),
			KV("負載貢獻", fmt.Sprintf("%.2f 算力 (約占全公司推理 %.0f%%)", loadContrib, util*100)),
		)
	} else {
		if sim.IsDraft(md) {
			infoLines = append(infoLines, styleWarn.Render("⚠️ 待發佈草稿 — 請按 p 鍵進行發佈"))
		} else {
			infoLines = append(infoLines, "狀態: 離線")
		}
	}

	// Price vs EffectiveRefPrice
	refPrice := sim.EffectiveRefPrice(m.state, md.Segment, m.cfg)
	priceCompare := fmt.Sprintf("$%.0f (推薦 $%.0f)", md.Price, refPrice)
	if md.Price > refPrice {
		priceCompare += " [偏高]"
	} else if md.Price < refPrice && md.Price > 0 {
		priceCompare += " [偏低]"
	}
	infoLines = append(infoLines, KV("定價", priceCompare))

	body := VStack(
		qualityBlock,
		"",
		VStack(infoLines...),
	)
	return Card("模型詳情", body)
}
