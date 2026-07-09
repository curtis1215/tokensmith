package tui

import "tokensmith/internal/balance"

// eventMeta is the Chinese copy for one industry event. Choices[0] is the
// paid/active option, Choices[1] the free/passive default — matching the
// balance.EventSpec choice convention.
type eventMeta struct {
	Name    string
	Desc    string
	Choices [2]string
}

var eventCatalog = map[string]eventMeta{
	balance.EvChipShortage: {
		Name: "晶片短缺", Desc: "供應鏈吃緊，自建服務器成本上漲",
		Choices: [2]string{"囤貨鎖價（花錢免疫漲價）", "轉租度過（承受漲價）"},
	},
	balance.EvEnergySpike: {
		Name: "能源價波動", Desc: "電價劇烈波動，影響機房電費",
		Choices: [2]string{"簽長約鎖價（花錢固定電價）", "觀望（承受波動）"},
	},
	balance.EvRivalBreak: {
		Name: "對手重大發表", Desc: "對手模型能力躍升，前沿被推高",
		Choices: [2]string{"限時促銷（花錢拉用戶成長）", "觀望"},
	},
	balance.EvOpenSourceWar: {
		Name: "開源價格戰", Desc: "開源模型衝擊付費意願",
		Choices: [2]string{"跟進降價（搶量、犧牲單價）", "守高階定位"},
	},
	balance.EvRivalScandal: {
		Name: "對手安全爭議", Desc: "低安全對手爆出爭議，用戶外流",
		Choices: [2]string{"花錢搶客（大幅拉成長）", "觀望（自然吸收）"},
	},
	balance.EvPaper: {
		Name: "突破論文", Desc: "研究突破，某科技分支解鎖成本下降",
		Choices: [2]string{"押注加碼（花 R&D 換 5 折）", "常規吸收（7 折）"},
	},
	balance.EvIncident: {
		Name: "模型安全事故", Desc: "你的模型出事，用戶信任受損",
		Choices: [2]string{"公開道歉（花錢，流失減半）", "低調處理（省錢，留後遺症）"},
	},
	balance.EvRegulation: {
		Name: "AI 監管新法", Desc: "新法上路，安全維度權重提高",
		Choices: [2]string{"投資合規（花錢，安全 +10%）", "硬扛"},
	},
	balance.EvMarketCycle: {
		Name: "市場榮枯", Desc: "宏觀週期轉向，市場規模波動",
	},
	balance.EvBubbleTalk: {
		Name: "AI 泡沫論", Desc: "市場質疑估值，估值倍數下修",
		Choices: [2]string{"釋出實績穩信心（花錢減緩）", "觀望"},
	},
}

func eventLabel(id string) eventMeta {
	if m, ok := eventCatalog[id]; ok {
		return m
	}
	return eventMeta{Name: id}
}
