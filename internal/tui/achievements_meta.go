package tui

import (
	"tokensmith/internal/model"
	"tokensmith/internal/sim"
)

// achievement is a player-level badge; Check is a pure read over Model.
type achievement struct {
	ID    string
	Name  string
	Desc  string
	Check func(m Model) bool
}

func maxTrainedGen(s model.GameState) int {
	g := 0
	for _, md := range s.Models {
		if md.Gen > g {
			g = md.Gen
		}
	}
	return g
}

func anyOnline(s model.GameState) bool {
	for _, md := range s.Models {
		if md.Online {
			return true
		}
	}
	return false
}

func totalTokens(m Model) int {
	in, out := sumSourceTotals(m.consumed)
	return in + out
}

func anyCountered(s model.GameState) bool {
	for _, r := range s.Campaign.Reports {
		for _, e := range r.Entries {
			if e.Countered {
				return true
			}
		}
	}
	return false
}

func distinctBadges(s model.GameState) int {
	seen := map[model.Doctrine]bool{}
	for _, d := range s.Prestige.RouteBadges {
		seen[d] = true
	}
	return len(seen)
}

func milestoneCheck(n int) func(Model) bool {
	return func(m Model) bool { return m.state.MilestonesReached >= n }
}

func genCheck(g int) func(Model) bool {
	return func(m Model) bool { return maxTrainedGen(m.state) >= g }
}

var achievementCatalog = []achievement{
	// 進度（0-11）
	{"first-online", "首航", "第一個模型上線", func(m Model) bool { return anyOnline(m.state) }},
	{"gen-2", "第二世代", "訓練出 Gen2 模型", genCheck(2)},
	{"gen-3", "第三世代", "訓練出 Gen3 模型", genCheck(3)},
	{"gen-4", "Gen4 大師", "訓練出 Gen4 模型", genCheck(4)},
	{"gen-5", "Gen5 神話", "訓練出 Gen5 模型", genCheck(5)},
	{"ms-1m", "百萬俱樂部", "估值達 $1M", milestoneCheck(1)},
	{"ms-10m", "千萬格局", "估值達 $10M", milestoneCheck(2)},
	{"ms-100m", "億級玩家", "估值達 $100M", milestoneCheck(3)},
	{"ms-1b", "獨角獸", "估值達 $1B", milestoneCheck(4)},
	{"ms-10b", "十倍獨角獸", "估值達 $10B", milestoneCheck(5)},
	{"ms-100b", "科技巨頭", "估值達 $100B", milestoneCheck(6)},
	{"ms-1t", "兆元傳說", "估值達 $1T", milestoneCheck(7)},
	// 習慣（12-17）
	{"streak-3", "三日連寫", "連續寫程式 3 天", func(m Model) bool { return m.streakDays >= 3 }},
	{"streak-7", "七日成習", "連續寫程式 7 天", func(m Model) bool { return m.streakDays >= 7 }},
	{"streak-10", "十日爐火", "連續寫程式 10 天（加成封頂）", func(m Model) bool { return m.streakDays >= 10 }},
	{"tokens-1m", "百萬鍛造", "累計收成 1M tokens", func(m Model) bool { return totalTokens(m) >= 1_000_000 }},
	{"tokens-10m", "千萬鍛造", "累計收成 10M tokens", func(m Model) bool { return totalTokens(m) >= 10_000_000 }},
	{"tokens-100m", "億級鍛造", "累計收成 100M tokens", func(m Model) bool { return totalTokens(m) >= 100_000_000 }},
	// 經營（18-21）
	{"star-first", "首位明星", "簽下第一位明星員工", func(m Model) bool { return len(m.state.HiredStars) >= 1 }},
	{"star-all", "全明星陣容", "簽下所有明星員工", func(m Model) bool {
		return len(m.cfg.Stars) > 0 && len(m.state.HiredStars) >= len(m.cfg.Stars)
	}},
	{"team-full", "四職能齊備", "研究/工程/營運/行銷都有人", func(m Model) bool {
		s := m.state
		r := 0
		for _, n := range s.Research.Researchers {
			r += n
		}
		return r > 0 && s.Engineers > 0 && s.Ops > 0 && s.Marketing > 0
	}},
	{"triple-crown", "三冠王", "三個市場同時排名第一", func(m Model) bool {
		for seg := 0; seg < model.NumSegments; seg++ {
			if rank, _ := sim.MarketRank(m.state, m.cfg, model.Segment(seg)); rank != 1 {
				return false
			}
		}
		return true
	}},
	// 戰役（22-25）
	{"doctrine-chosen", "定調", "選定第一個公司戰略", func(m Model) bool { return m.state.Campaign.Doctrine != model.DoctrineNone }},
	{"showdown-win", "決勝守成", "頂住宿敵攻勢贏得路線勝利", func(m Model) bool {
		return m.state.Campaign.Victory != model.DoctrineNone || m.state.Campaign.Stage == model.CampaignStageWon
	}},
	{"counter-hit", "反制奏效", "成功反制一次宿敵行動", func(m Model) bool { return anyCountered(m.state) }},
	{"endless", "無盡征途", "進入無盡模式", func(m Model) bool { return m.state.Campaign.Endless }},
	// 輪迴（26-28）
	{"prestige-first", "首次傳承", "完成第一次傳承重開", func(m Model) bool { return m.state.Prestige.Patents > 0 }},
	{"patents-10", "專利大戶", "累計專利達 10", func(m Model) bool { return m.state.Prestige.Patents >= 10 }},
	{"badge-grand-slam", "路線大滿貫", "三條路線徽章集滿", func(m Model) bool { return distinctBadges(m.state) >= 3 }},
}

// achievementCategories slices the catalog for the badge-wall page.
var achievementCategories = []struct {
	Title    string
	From, To int // [From, To)
}{
	{"進度", 0, 12},
	{"習慣", 12, 18},
	{"經營", 18, 22},
	{"戰役", 22, 26},
	{"輪迴", 26, 29},
}
