package tui

import (
	"fmt"
	"strings"

	"tokensmith/internal/model"
	"tokensmith/internal/sim"
)

func renderModels(m Model) string {
	var b strings.Builder
	b.WriteString("模型\n")
	if len(m.state.Models) == 0 {
		b.WriteString("  （無 — 按 t 訓練第一個模型）\n")
		b.WriteString(helpStyle.Render("\n[t]訓練新模型 [Tab]切頁"))
		return b.String()
	}
	draftN := 0
	for _, md := range m.state.Models {
		if sim.IsDraft(md) {
			draftN++
		}
	}
	if draftN > 0 {
		b.WriteString(fmt.Sprintf("有 %d 個待發佈\n", draftN))
	}
	b.WriteString("── 待發佈 ──\n")
	anyDraft := false
	for i, md := range m.state.Models {
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
	for i, md := range m.state.Models {
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
	b.WriteString(helpStyle.Render("\n[↑↓]選模型 [p]發佈 [t]訓練 [$]改價 [Tab]切頁"))
	return b.String()
}
