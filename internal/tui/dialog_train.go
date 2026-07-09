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

// update applies one key to the dialog, returning the next dialog and whether
// the player confirmed (Enter) or cancelled (Esc).
func (d trainDialog) update(msg tea.KeyMsg) (next trainDialog, confirm, cancel bool) {
	switch msg.String() {
	case "esc":
		return d, false, true
	case "enter":
		return d, true, false
	case "up":
		d.dim = (d.dim + model.NumQualityDims - 1) % model.NumQualityDims
	case "down":
		d.dim = (d.dim + 1) % model.NumQualityDims
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
		d.alloc[d.dim] += allocStep
		d.normalize()
	case "-", "_":
		d.alloc[d.dim] -= allocStep
		if d.alloc[d.dim] < 0 {
			d.alloc[d.dim] = 0
		}
		d.normalize()
	}
	return d, false, false
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
	dim     int // currently selected quality dimension
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
	return model.StartTraining{Gen: d.gen, Segment: d.segment, Alloc: d.alloc, Price: b.SegmentRefPrice[d.segment]}
}

var dimNames = [model.NumQualityDims]string{"能力", "成本", "安全", "速度"}

func renderTrainDialog(d trainDialog, m Model) string {
	var b strings.Builder
	genHint := ""
	if d.maxGen < balance.MaxGen {
		genHint = fmt.Sprintf("（上限 Gen%d，更高需科技樹解鎖）", d.maxGen)
	}
	b.WriteString(fmt.Sprintf("世代 ‹ Gen%d ›%s   主打區隔 ‹ %s ›\n\n預算分配（可用 R&D %s）\n",
		d.gen, genHint, segmentName(d.segment), human(m.state.Resources.RnD)))
	for i := 0; i < model.NumQualityDims; i++ {
		cursor := " "
		if i == d.dim {
			cursor = "▸"
		}
		est := d.alloc[i] * m.cfg.GenQualityCap[d.gen]
		b.WriteString(fmt.Sprintf("%s %s %s %.0f%%  → 預估 %.0f\n",
			cursor, dimNames[i], Bar(d.alloc[i], 10), d.alloc[i]*100, est))
	}
	b.WriteString(fmt.Sprintf("\n成本 %s R&D + %.0f GPU·s\n\n", human(m.cfg.GenRnDCost[d.gen]), m.cfg.GenTrainWorkGPUSec[d.gen]))
	b.WriteString(helpStyle.Render("[←→]世代 [Tab]區隔 [↑↓]維度 [+/-]分配 [Enter]開訓 [Esc]取消"))
	return Card("訓練新模型", b.String())
}
