package tui

import (
	"fmt"
	"strings"

	"tokensmith/internal/model"
)

func renderModels(m Model) string {
	var b strings.Builder
	b.WriteString("營運中模型\n")
	if len(m.state.Models) == 0 {
		b.WriteString("  （無 — 按 t 訓練第一個模型）\n")
	}
	for _, md := range m.state.Models {
		b.WriteString(fmt.Sprintf("  Gen%d · %s · 用戶 %s · 價 $%.0f · 能力 %.0f\n",
			md.Gen, segmentName(md.Segment), human(md.Users), md.Price, md.Quality[model.DimCapability]))
	}
	b.WriteString(helpStyle.Render("\n[t]訓練新模型 [Tab]切頁"))
	return b.String()
}
