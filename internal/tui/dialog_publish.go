package tui

import (
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"tokensmith/internal/model"
	"tokensmith/internal/sim"
)

// publishDialog is the name+price modal for onlining a draft.
type publishDialog struct {
	index     int
	name      string
	price     float64
	refPrice  float64
	gen       int
	segment   model.Segment
	quality   [model.NumQualityDims]float64
	priceOnly bool
}

func newPublishDialog(m Model, index int) (publishDialog, bool) {
	if index < 0 || index >= len(m.state.Models) {
		return publishDialog{}, false
	}
	md := m.state.Models[index]
	if !sim.IsDraft(md) {
		return publishDialog{}, false
	}
	ref := sim.EffectiveRefPrice(m.state, md.Segment, m.cfg)
	price := md.Price
	if price <= 0 {
		price = ref
	}
	name := fmt.Sprintf("Gen%d-%s", md.Gen, segmentName(md.Segment))
	return publishDialog{
		index:    index,
		name:     name,
		price:    price,
		refPrice: ref,
		gen:      md.Gen,
		segment:  md.Segment,
		quality:  md.Quality,
	}, true
}

func (d publishDialog) command() model.PublishModel {
	return model.PublishModel{ModelIndex: d.index, Name: d.name, Price: d.price}
}

func (d publishDialog) update(msg tea.KeyMsg) (next publishDialog, confirm, cancel bool) {
	switch msg.String() {
	case "esc":
		return d, false, true
	case "enter":
		return d, true, false
	case "left":
		d.price -= 1
		if d.price < 1 {
			d.price = 1
		}
	case "right":
		d.price += 1
	case "shift+left":
		d.price -= 5
		if d.price < 1 {
			d.price = 1
		}
	case "shift+right":
		d.price += 5
	case "backspace":
		if d.priceOnly {
			return d, false, false
		}
		if r := []rune(d.name); len(r) > 0 {
			d.name = string(r[:len(r)-1])
		}
	default:
		if d.priceOnly {
			return d, false, false
		}
		// Append single runes from plain typing; ignore multi-rune/control.
		if len(msg.Runes) == 1 {
			r := msg.Runes[0]
			if r >= 32 && utf8.RuneCountInString(d.name) < 24 {
				d.name += string(r)
			}
		}
	}
	return d, false, false
}

func renderPublishDialog(d publishDialog, m Model) string {
	est := sim.EstimateUserTarget(m.state, d.index, d.price, m.cfg)
	demand := 1.0
	if d.price > 0 && d.refPrice > 0 {
		demand = math.Pow(d.refPrice/d.price, m.cfg.PriceElasticity)
	}
	var b strings.Builder
	title := "發佈模型"
	if d.priceOnly {
		title = "修改定價"
	}
	b.WriteString(fmt.Sprintf(
		"%s\n  Gen%d · %s · 能力 %.0f / 成本 %.0f / 安全 %.0f / 速度 %.0f\n\n",
		title, d.gen, segmentName(d.segment),
		d.quality[0], d.quality[1], d.quality[2], d.quality[3],
	))
	if d.priceOnly {
		b.WriteString(fmt.Sprintf("  名稱  %s\n", d.name))
	} else {
		b.WriteString(fmt.Sprintf("  名稱  %s▌\n", d.name))
	}
	b.WriteString(fmt.Sprintf("  定價  $%.0f   （推薦 $%.0f）\n\n", d.price, d.refPrice))
	b.WriteString(fmt.Sprintf("  預估  需求 ×%.2f · 封頂用戶 ~%s\n\n", demand, human(est)))

	if d.priceOnly {
		b.WriteString(helpStyle.Render("[←→]調價 [Shift+←→]±5  [Enter]確認 [Esc]取消"))
	} else {
		b.WriteString(helpStyle.Render("[←→]調價 [Shift+←→]±5  輸入名稱  [Enter]發佈 [Esc]取消"))
	}
	return boxStyle.Render(b.String())
}
