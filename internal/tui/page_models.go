package tui

import (
	"fmt"
	"strings"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
	"tokensmith/internal/sim"
)

func renderModels(m Model) string {
	s := m.state
	cw := m.contentWidth()
	if len(s.Models) == 0 {
		body := "（無 — 按 t 訓練第一個模型）"
		listCard := CardIn(CardDefault, cw, "模型列表", body)
		return listCard
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
		users := md.Users
		if m.dispReady && i < len(m.disp.ModelUsers) {
			users = m.disp.ModelUsers[i]
		}
		b.WriteString(fmt.Sprintf("%s[%d] 「%s」 Gen%d · %s · 用戶 %s · $%.0f · %s\n",
			cur, i, name, md.Gen, segmentName(md.Segment), human(users), md.Price, status))
	}
	if !anyLive {
		b.WriteString("  （無）\n")
	}
	listCard := CardIn(CardDefault, cw, "模型列表", b.String())

	// 2. Detail card
	detailCard := renderModelDetail(m, m.modelCursor)

	// Combine into horizontal row (footer is fixed shell chrome)
	return ResponsiveRow(cw, 2, listCard, detailCard)
}

func renderModelDetail(m Model, idx int) string {
	if idx < 0 || idx >= len(m.state.Models) {
		return CardIn(CardDefault, m.contentWidth(), "模型詳情", "無選取模型")
	}
	md := m.state.Models[idx]
	capVal := 0.0
	if spec, err := balance.Generation(md.Gen); err == nil {
		capVal = spec.QualityScale
	}

	// 1. Quality Dims — bars use generation QualityScale, never re-normalize stored values.
	qNames := [4]string{"能力", "成本", "安全", "速度"}
	fv := sim.ModelFrontierView(m.state, idx, m.cfg)
	var qLines []string
	for d := 0; d < 4; d++ {
		val := md.Quality[d] // stored absolute; never rewritten
		var barStr string
		if capVal > 0 {
			barStr = Bar(val/capVal, 8)
		} else {
			barStr = fmt.Sprintf("%.0f (無上限)", val)
		}
		// Relative frontier delta (explanatory only).
		rel := ""
		if fv.Active && fv.GlobalFrontier[d] > 0 {
			pct := fv.FrontierDeltaPct[d] * 100
			if pct >= 0 {
				rel = fmt.Sprintf(" · 相對前沿 %+.1f%%", pct)
			} else {
				rel = styleWarn.Render(fmt.Sprintf(" · 落後 %.1f%%", -pct))
			}
			rel += fmt.Sprintf("（前沿 %.0f）", fv.GlobalFrontier[d])
		}
		qLines = append(qLines, fmt.Sprintf("%s: %s %.0f%s", qNames[d], barStr, val, rel))
	}
	if fv.Active {
		gap := fv.GenerationGap
		gapStr := fmt.Sprintf("約 %+.1f 世代", gap)
		if gap < -0.05 {
			gapStr = styleWarn.Render(fmt.Sprintf("約落後 %.1f 世代", -gap))
		} else if gap > 0.05 {
			gapStr = styleGain.Render(fmt.Sprintf("約領先 %.1f 世代", gap))
		}
		qLines = append(qLines, fmt.Sprintf("等效世代 %.1f · 本體 Gen%d · %s",
			fv.EquivalentGen, fv.ModelGen, gapStr))
	}
	qualityBlock := VStack(qLines...)

	// 2. Info Block
	var infoLines []string
	if md.Online {
		users := md.Users
		if m.dispReady && idx < len(m.disp.ModelUsers) {
			users = m.disp.ModelUsers[idx]
		}
		est := sim.EstimateUserTarget(m.state, idx, md.Price, m.cfg)
		monthly := users * md.Price
		loadContrib := users * m.cfg.InferenceLoadPerUser

		infCap := sim.EffectiveInference(m.state, m.cfg)
		util := 0.0
		if infCap > 0 {
			util = m.state.Compute.InferenceLoad / infCap
		}
		if m.dispReady {
			util = m.disp.InfUtil
		}

		infoLines = append(infoLines,
			KV("用戶數", fmt.Sprintf("%s / 預估上限 ~%s", human(users), human(est))),
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
	return CardIn(CardDefault, m.contentWidth(), "模型詳情", body)
}
