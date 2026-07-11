// internal/tui/theme.go
package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// HUD 調色盤 — 純 hex；lipgloss colorprofile 在低色數終端自動降級。
var (
	colorCyan   = lipgloss.Color("#00D7FF") // 主 HUD 色
	colorPurple = lipgloss.Color("#B48CFF") // 次要
	colorGain   = lipgloss.Color("#50FA7B") // 漲 / 成功
	colorLoss   = lipgloss.Color("#FF5555") // 跌 / 威脅
	colorAmber  = lipgloss.Color("#FFB86C") // 警告 / 倒數
	colorGold   = lipgloss.Color("#FFD75F") // 慶祝 / 里程碑
	colorDim    = lipgloss.Color("#6B7280") // 邊框灰
	colorInk    = lipgloss.Color("#0B1220") // 反白文字用深底色

	styleCyan   = lipgloss.NewStyle().Foreground(colorCyan)
	stylePurple = lipgloss.NewStyle().Foreground(colorPurple)
	styleGain   = lipgloss.NewStyle().Foreground(colorGain)
	styleLoss   = lipgloss.NewStyle().Foreground(colorLoss)
	styleAmber  = lipgloss.NewStyle().Foreground(colorAmber)
	styleGold   = lipgloss.NewStyle().Foreground(colorGold)
)

// CardKind 選擇作戰室卡片變體。
type CardKind int

const (
	CardDefault CardKind = iota // 灰細邊：一般資訊
	CardAccent                  // 青粗邊：作戰室重點
	CardThreat                  // 紅邊：宿敵 / 危機
	CardGold                    // 金雙線邊：慶祝
)

func cardStyle(kind CardKind) lipgloss.Style {
	base := lipgloss.NewStyle().Padding(0, 1)
	switch kind {
	case CardAccent:
		return base.Border(lipgloss.ThickBorder()).BorderForeground(colorCyan)
	case CardThreat:
		return base.Border(lipgloss.RoundedBorder()).BorderForeground(colorLoss)
	case CardGold:
		return base.Border(lipgloss.DoubleBorder()).BorderForeground(colorGold)
	default:
		return base.Border(lipgloss.RoundedBorder()).BorderForeground(colorDim)
	}
}

func cardTitleStyle(kind CardKind) lipgloss.Style {
	switch kind {
	case CardThreat:
		return lipgloss.NewStyle().Bold(true).Foreground(colorLoss)
	case CardGold:
		return lipgloss.NewStyle().Bold(true).Foreground(colorGold)
	default:
		return lipgloss.NewStyle().Bold(true).Foreground(colorCyan)
	}
}

// CardIn renders a card variant; width > 0 forces the total rendered width
// (borders included) so grid rows align flush.
func CardIn(kind CardKind, width int, title, body string) string {
	st := cardStyle(kind)
	if width > 0 {
		st = st.Width(width - 2) // Style.Width 是內容寬；左右邊框各 +1
	}
	return st.Render(cardTitleStyle(kind).Render(title) + "\n" + body)
}

// filledCells clamps frac to [0,1] and returns the filled cell count.
func filledCells(frac float64, width int) int {
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	return int(frac * float64(width))
}

func loadColor(frac float64) lipgloss.Color {
	switch {
	case frac >= 0.9:
		return colorLoss
	case frac >= 0.7:
		return colorAmber
	default:
		return colorCyan
	}
}

// lerpHex linearly interpolates two #RRGGBB colors.
func lerpHex(a, b string, t float64) string {
	pa, _ := strconv.ParseUint(strings.TrimPrefix(a, "#"), 16, 32)
	pb, _ := strconv.ParseUint(strings.TrimPrefix(b, "#"), 16, 32)
	c := func(x, y uint64) uint64 { return uint64(float64(x) + t*(float64(y)-float64(x))) }
	return fmt.Sprintf("#%02X%02X%02X",
		c(pa>>16&0xFF, pb>>16&0xFF), c(pa>>8&0xFF, pb>>8&0xFF), c(pa&0xFF, pb&0xFF))
}

func gradientBar(frac float64, width int, from, to string) string {
	n := filledCells(frac, width)
	var sb strings.Builder
	for i := 0; i < n; i++ {
		t := 0.0
		if width > 1 {
			t = float64(i) / float64(width-1)
		}
		sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(lerpHex(from, to, t))).Render("█"))
	}
	sb.WriteString(styleMuted.Render(strings.Repeat("░", width-n)))
	return sb.String()
}

// LoadBar colors by utilisation thresholds (<0.7 cyan, <0.9 amber, else red).
func LoadBar(frac float64, width int) string {
	n := filledCells(frac, width)
	return lipgloss.NewStyle().Foreground(loadColor(frac)).Render(strings.Repeat("█", n)) +
		styleMuted.Render(strings.Repeat("░", width-n))
}

// GoldBar is the milestone / achievement bar.
func GoldBar(frac float64, width int) string {
	return gradientBar(frac, width, "#FFD75F", "#FFB86C")
}
