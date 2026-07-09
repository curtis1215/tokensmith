package tui

type techMeta struct{ Name, Effect string }

var techCatalog = map[string]techMeta{
	"algo-cap-1":       {Name: "能力架構 I", Effect: "能力 +15%"},
	"algo-train-1":     {Name: "訓練效率 I", Effect: "訓練 R&D/工作量優化"},
	"model-gen-2":      {Name: "解鎖 Gen2", Effect: "可訓練第二世代模型"},
	"model-gen-3":      {Name: "解鎖 Gen3", Effect: "可訓練第三世代模型"},
	"model-gen-4":      {Name: "解鎖 Gen4", Effect: "可訓練第四世代模型"},
	"model-gen-5":      {Name: "解鎖 Gen5", Effect: "可訓練第五世代模型"},
	"infra-eff-1":      {Name: "算力效率 I", Effect: "硬體算力 +10%"},
	"infra-density-1":  {Name: "算力密度 I", Effect: "硬體算力 +15%"},
	"process-N5":       {Name: "解鎖製程 N5", Effect: "可租用/建造 N5 晶片"},
	"process-N3":       {Name: "解鎖製程 N3", Effect: "可租用/建造 N3 晶片"},
	"process-N2":       {Name: "解鎖製程 N2", Effect: "可租用/建造 N2 晶片"},
	"biz-growth-1":     {Name: "商業成長 I", Effect: "用戶增長率 +15%"},
	"biz-price-1":      {Name: "定價策略 I", Effect: "推薦定價 +10%"},
	"align-safety-1":   {Name: "對齊安全 I", Effect: "模型安全屬性 +15%"},
	"align-incident-1": {Name: "事故減緩 I", Effect: "重大安全事故率減半"},
}

func techLabel(id string) techMeta {
	if m, ok := techCatalog[id]; ok {
		return m
	}
	return techMeta{Name: id, Effect: ""}
}
