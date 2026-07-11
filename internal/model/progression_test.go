package model

import (
	"encoding/json"
	"testing"
)

func TestProgressionStateConstruction(t *testing.T) {
	p := ProgressionState{
		MaxUnlockedGen: 6,
		IndustryTime:   7200,
		Frontier: FrontierProject{
			Active:             true,
			TargetGen:          7,
			RnDTotal:           500e6,
			RnDRemaining:       250e6,
			WorkTotal:          1e9,
			WorkRemaining:      5e8,
			RecommendedCompute: 1200,
			AllocationPct:      40,
		},
		Eras: []EraProgress{
			{Era: 3, HasPrimary: true, Primary: BranchAlgo, UnlockedMask: 0b0101},
		},
		Rivals: RivalEraState{Era: 3, Leaders: []string{"OpenAI", "Anthropic"}},
	}
	if p.MaxUnlockedGen != 6 || p.IndustryTime != 7200 {
		t.Fatalf("scalar fields wrong: %+v", p)
	}
	if !p.Frontier.Active || p.Frontier.TargetGen != 7 || p.Frontier.AllocationPct != 40 {
		t.Fatalf("frontier fields wrong: %+v", p.Frontier)
	}
	if len(p.Eras) != 1 || p.Eras[0].Primary != BranchAlgo || p.Eras[0].UnlockedMask != 0b0101 {
		t.Fatalf("era progress wrong: %+v", p.Eras)
	}
	if p.Rivals.Era != 3 || len(p.Rivals.Leaders) != 2 {
		t.Fatalf("rivals wrong: %+v", p.Rivals)
	}
	var s GameState
	s.Progression = p
	if s.Progression.MaxUnlockedGen != 6 {
		t.Fatalf("GameState.Progression not usable: %+v", s.Progression)
	}
}

func TestProgressionStateJSONRoundTrip(t *testing.T) {
	in := ProgressionState{
		MaxUnlockedGen: 8,
		IndustryTime:   43200,
		Frontier: FrontierProject{
			Active:             true,
			TargetGen:          9,
			RnDTotal:           4.5e9,
			RnDRemaining:       1e9,
			WorkTotal:          2e10,
			WorkRemaining:      1e10,
			RecommendedCompute: 7500,
			AllocationPct:      60,
		},
		Eras: []EraProgress{
			{Era: 3, HasPrimary: true, Primary: BranchInfra, UnlockedMask: 0b0011},
			{Era: 4, HasPrimary: false, Primary: BranchAlgo, UnlockedMask: 0},
		},
		Rivals: RivalEraState{Era: 4, Leaders: []string{"Google", "Meta", "xAI"}},
	}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out ProgressionState
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.MaxUnlockedGen != in.MaxUnlockedGen || out.IndustryTime != in.IndustryTime {
		t.Fatalf("scalars: got %+v want %+v", out, in)
	}
	if out.Frontier != in.Frontier {
		t.Fatalf("frontier: got %+v want %+v", out.Frontier, in.Frontier)
	}
	if len(out.Eras) != len(in.Eras) {
		t.Fatalf("eras len: got %d want %d", len(out.Eras), len(in.Eras))
	}
	for i := range in.Eras {
		if out.Eras[i] != in.Eras[i] {
			t.Fatalf("eras[%d]: got %+v want %+v", i, out.Eras[i], in.Eras[i])
		}
	}
	if out.Rivals.Era != in.Rivals.Era || len(out.Rivals.Leaders) != len(in.Rivals.Leaders) {
		t.Fatalf("rivals: got %+v want %+v", out.Rivals, in.Rivals)
	}
	for i := range in.Rivals.Leaders {
		if out.Rivals.Leaders[i] != in.Rivals.Leaders[i] {
			t.Fatalf("leaders[%d]: got %q want %q", i, out.Rivals.Leaders[i], in.Rivals.Leaders[i])
		}
	}
}

func TestProgressionCommandsImplementCommand(t *testing.T) {
	var cmds []Command
	cmds = append(cmds,
		StartFrontierProject{TargetGen: 6},
		SetFrontierAllocation{Percent: 50},
		UnlockEraBreakthrough{Era: 3, Branch: BranchBusiness},
	)
	if len(cmds) != 3 {
		t.Fatalf("commands not assignable to Command interface: len=%d", len(cmds))
	}
	if c, ok := cmds[0].(StartFrontierProject); !ok || c.TargetGen != 6 {
		t.Fatalf("StartFrontierProject: %+v ok=%v", cmds[0], ok)
	}
	if c, ok := cmds[1].(SetFrontierAllocation); !ok || c.Percent != 50 {
		t.Fatalf("SetFrontierAllocation: %+v ok=%v", cmds[1], ok)
	}
	if c, ok := cmds[2].(UnlockEraBreakthrough); !ok || c.Era != 3 || c.Branch != BranchBusiness {
		t.Fatalf("UnlockEraBreakthrough: %+v ok=%v", cmds[2], ok)
	}
}

func TestCompetitorMomentumFields(t *testing.T) {
	c := Competitor{Name: "Rival", MomentumCycles: 4}
	c.MomentumPct[DimCapability] = 0.05
	c.MomentumPct[DimSpeed] = 0.02
	if c.MomentumCycles != 4 || c.MomentumPct[DimCapability] != 0.05 || c.MomentumPct[DimSpeed] != 0.02 {
		t.Fatalf("momentum fields wrong: %+v", c)
	}
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out Competitor
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.MomentumCycles != 4 || out.MomentumPct[DimCapability] != 0.05 {
		t.Fatalf("momentum JSON round-trip: %+v", out)
	}
}
