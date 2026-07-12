// internal/tui/feedback.go
package tui

import (
	"fmt"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

// MomentLevel grades feedback intensity.
type MomentLevel int

const (
	LevelMinor MomentLevel = iota // setNotice
	LevelMajor                    // 金色橫幅佇列
	LevelEpic                     // 全螢幕 overlay
)

// Moment is a detected feedback-worthy game event. Title is only used by the
// Epic overlay; empty falls back to the default celebration title.
type Moment struct {
	Level MomentLevel
	Text  string
	Title string
}

// detectMoments compares pre/post-tick states and returns feedback moments.
// Pure and read-only; never mutates sim truth.
func detectMoments(prev, next model.GameState, cfg balance.Config) []Moment {
	var out []Moment
	if prev.HasTraining && !next.HasTraining && len(next.Models) > len(prev.Models) {
		md := next.Models[len(next.Models)-1]
		out = append(out, Moment{Level: LevelMajor,
			Text: fmt.Sprintf("🧪 Gen%d 訓練完成！草稿已就緒——模型頁按 p 發佈", md.Gen)})
	}
	for i := prev.MilestonesReached; i < next.MilestonesReached && i < len(cfg.ValuationMilestones); i++ {
		out = append(out, Moment{Level: LevelMajor,
			Text: fmt.Sprintf("🏁 里程碑達成：估值 $%s！", human(cfg.ValuationMilestones[i]))})
	}
	for _, e := range newReportEntries(prev, next) {
		if mo, ok := reportMoment(e); ok {
			out = append(out, mo)
		}
	}
	return out
}

func reportMoment(e model.CampaignReportEntry) (Moment, bool) {
	switch e.Kind {
	case model.ReportStageAdvanced:
		return Moment{Level: LevelMajor, Text: "📈 階段推進：" + campaignStageLabel(model.CampaignStage(e.SubjectID))}, true
	case model.ReportShowdown:
		return Moment{Level: LevelMajor, Text: "⚔ 決勝開始！頂住主要宿敵 2 次攻勢即可奪下路線"}, true
	case model.ReportVictory:
		return Moment{Level: LevelEpic, Text: "🏆 路線勝利：" + doctrineLabel(model.Doctrine(e.SubjectID)) + "！按 P 結算"}, true
	case model.ReportRivalAction:
		if e.Countered {
			return Moment{Level: LevelMajor, Text: "🛡 反制奏效：" + rivalActionLabel(e.DetailID) + " 衝擊減半！"}, true
		}
		return Moment{Level: LevelMajor, Text: "🚨 宿敵行動：" + rivalActionLabel(e.DetailID)}, true
	case model.ReportFinancialRisk:
		return Moment{Level: LevelMajor, Text: "🩸 財務風險：現金為負——董事會已提高關注"}, true
	}
	return Moment{}, false
}

// newReportEntries returns board-report entries added between prev and next.
// Reports are append-only per cycle (capped at 20 total), so compare against
// the previous last cycle + entry count.
func newReportEntries(prev, next model.GameState) []model.CampaignReportEntry {
	nr := next.Campaign.Reports
	if len(nr) == 0 {
		return nil
	}
	lastCycle := -1
	prevLastLen := 0
	if pr := prev.Campaign.Reports; len(pr) > 0 {
		lastCycle = pr[len(pr)-1].Cycle
		prevLastLen = len(pr[len(pr)-1].Entries)
	}
	var out []model.CampaignReportEntry
	for _, r := range nr {
		switch {
		case r.Cycle > lastCycle:
			out = append(out, r.Entries...)
		case r.Cycle == lastCycle && len(r.Entries) > prevLastLen:
			out = append(out, r.Entries[prevLastLen:]...)
		}
	}
	return out
}
