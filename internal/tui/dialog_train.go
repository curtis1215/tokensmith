package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
	"tokensmith/internal/sim"
)

const allocStep = 0.05

// trainFocusCount is 4 alloc dims + 4 boost rows.
const trainFocusCount = model.NumQualityDims * 2

// update applies one key to the dialog, returning the next dialog and whether
// the player confirmed (Enter) or cancelled (Esc).
func (d trainDialog) update(msg tea.KeyMsg) (next trainDialog, confirm, cancel bool) {
	switch msg.String() {
	case "esc":
		return d, false, true
	case "enter":
		return d, true, false
	case "up":
		d.focus = (d.focus + trainFocusCount - 1) % trainFocusCount
	case "down":
		d.focus = (d.focus + 1) % trainFocusCount
	case "left":
		if d.gen > 1 {
			d.gen--
		}
	case "right":
		if d.gen < d.maxGen {
			d.gen++
		}
	case "tab":
		d.segment = (d.segment + 1) % model.NumSegments
	case "+", "=":
		if d.focus < model.NumQualityDims {
			d.alloc[d.focus] += allocStep
			d.normalize()
		} else {
			d.toggleBoost(d.focus - model.NumQualityDims)
		}
	case "-", "_":
		if d.focus < model.NumQualityDims {
			d.alloc[d.focus] -= allocStep
			if d.alloc[d.focus] < 0 {
				d.alloc[d.focus] = 0
			}
			d.normalize()
		} else {
			d.toggleBoost(d.focus - model.NumQualityDims)
		}
	case " ", "space":
		if d.focus >= model.NumQualityDims {
			d.toggleBoost(d.focus - model.NumQualityDims)
		}
	case "1", "2", "3", "4":
		idx := int(msg.String()[0] - '1')
		if idx >= 0 && idx < model.NumQualityDims {
			d.toggleBoost(idx)
		}
	}
	d.errMsg = ""
	return d, false, false
}

func (d *trainDialog) toggleBoost(i int) {
	if i < 0 || i >= model.NumQualityDims {
		return
	}
	d.boosts[i] = !d.boosts[i]
}

// normalize rescales alloc back to sum 1 (StartTraining requires sum≈1).
func (d *trainDialog) normalize() {
	var sum float64
	for i, a := range d.alloc {
		if a < 0 {
			d.alloc[i] = 0
			a = 0
		}
		sum += a
	}
	if sum == 0 {
		for i := range d.alloc {
			d.alloc[i] = 1.0 / model.NumQualityDims
		}
		return
	}
	for i := range d.alloc {
		d.alloc[i] /= sum
	}
}

// trainDialog is the modal training-budget allocator (spec §11.3).
type trainDialog struct {
	gen     int
	maxGen  int // highest generation currently unlocked (tech tree)
	segment model.Segment
	alloc   [model.NumQualityDims]float64
	boosts  [model.NumQualityDims]bool
	focus   int    // 0..7: 0-3 alloc dims, 4-7 boost rows
	errMsg  string // last confirm rejection (cash/R&D); cleared on key edit
}

func newTrainDialog(m Model) trainDialog {
	return trainDialog{
		gen:     1,
		maxGen:  sim.MaxUnlockedGen(m.state, m.cfg),
		segment: model.SegConsumer,
		alloc:   [model.NumQualityDims]float64{0.4, 0.2, 0.2, 0.2},
	}
}

// command builds the StartTraining for the current dialog selection, pricing at
// the target segment's reference price.
func (d trainDialog) command(b balance.Config) model.StartTraining {
	return model.StartTraining{
		Gen:     d.gen,
		Segment: d.segment,
		Alloc:   d.alloc,
		Price:   b.SegmentRefPrice[d.segment],
		Boosts:  d.boosts,
	}
}

var dimNames = [model.NumQualityDims]string{"能力", "成本", "安全", "速度"}

func trainPredictedAppeal(q [model.NumQualityDims]float64, seg model.Segment, cfg balance.Config) float64 {
	w := cfg.SegmentWeights[seg]
	var a float64
	for i := 0; i < model.NumQualityDims; i++ {
		a += q[i] * w[i]
	}
	return a
}

func renderTrainDialog(d trainDialog, m Model) string {
	cfg := m.cfg
	var b strings.Builder
	genHint := fmt.Sprintf("（上限 Gen%d，更高需科技樹／前沿研究解鎖）", d.maxGen)
	spec, _ := balance.Generation(d.gen)
	b.WriteString(fmt.Sprintf("世代 ‹ Gen%d ›%s   主打區隔 ‹ %s ›\n\n預算分配（可用 R&D %s）\n",
		d.gen, genHint, segmentName(d.segment), human(m.state.Resources.RnD)))
	for i := 0; i < model.NumQualityDims; i++ {
		cursor := " "
		if d.focus == i {
			cursor = "▸"
		}
		est := d.alloc[i] * spec.QualityScale
		b.WriteString(fmt.Sprintf("%s %s %s %.0f%%  → 預估 %.0f\n",
			cursor, dimNames[i], Bar(d.alloc[i], 10), d.alloc[i]*100, est))
	}

	refMonthly := sim.TrainBoostRefMonthly(m.state, cfg)
	rawMonthly := sim.BoostRefMonthlyCash(m.state, cfg)
	totalCost, _ := sim.QuoteTrainBoostCost(m.state, d.gen, d.boosts, cfg)

	b.WriteString("\n訓練投資（可選）\n")
	catalog := cfg.TrainBoosts
	for i := 0; i < model.NumQualityDims; i++ {
		cursor := " "
		if d.focus == model.NumQualityDims+i {
			cursor = "▸"
		}
		mark := "[ ]"
		if d.boosts[i] {
			mark = "[x]"
		}
		name := dimNames[i]
		if i < len(catalog) && catalog[i].NameZH != "" {
			name = catalog[i].NameZH
		}
		base, _ := balance.TrainBoostBasePrice(d.gen, refMonthly, model.QualityDim(i), cfg)
		b.WriteString(fmt.Sprintf("%s %s %s  %s\n", cursor, mark, name, human(base)))
	}

	floorBadge := ""
	if rawMonthly < cfg.TrainBoostFloorMonthly {
		floorBadge = "  [floor]"
	}
	years := 0.0
	if refMonthly > 0 {
		years = totalCost / (12 * refMonthly)
	}
	b.WriteString(fmt.Sprintf("\n參考月現金（標竿價） %s%s\n", human(refMonthly), floorBadge))
	b.WriteString(fmt.Sprintf("投資總額 %s", human(totalCost)))
	if totalCost > 0 {
		b.WriteString(fmt.Sprintf("  （約 %.1f 年有效現金）", years))
	}
	b.WriteString("\n")

	var none [model.NumQualityDims]bool
	qBefore, _ := sim.PredictedTrainQuality(m.state, d.gen, d.alloc, none, cfg)
	qAfter, _ := sim.PredictedTrainQuality(m.state, d.gen, d.alloc, d.boosts, cfg)
	aBefore := trainPredictedAppeal(qBefore, d.segment, cfg)
	aAfter := trainPredictedAppeal(qAfter, d.segment, cfg)
	b.WriteString(fmt.Sprintf("預測吸引力 %.1f → %.1f\n", aBefore, aAfter))

	b.WriteString(fmt.Sprintf("\n成本 %s R&D + %.0f GPU·s", human(spec.TrainRnD), spec.TrainWork))
	if totalCost > 0 {
		b.WriteString(fmt.Sprintf(" + %s 現金", human(totalCost)))
	}
	b.WriteString("\n")
	if d.errMsg != "" {
		b.WriteString("\n" + styleWarn.Render(d.errMsg) + "\n")
	}
	b.WriteString("\n" + helpStyle.Render(
		"[←→]世代 [Tab]區隔 [↑↓]列 [+/-]分配/投資 [Space]切換 [1-4]投資 [Enter]開訓 [Esc]取消"))
	return Card("訓練新模型", b.String())
}
