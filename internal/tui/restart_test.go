package tui

import (
	"testing"
	"time"

	"tokensmith/internal/model"
)

func TestManualRestartRequiresConfirm(t *testing.T) {
	m := testModel(t)
	m.state.Models = []model.Model{{Online: true, Users: 5}}
	m.state.Resources.Cash = 12345
	// first X arms the restart but does not reset
	nm, _ := m.Update(key("X"))
	g := nm.(Model)
	if !g.pendingRestart {
		t.Fatal("first X should arm restart confirmation")
	}
	if len(g.state.Models) == 0 {
		t.Fatal("first X should not restart yet")
	}
	// second X confirms → run resets
	nm2, _ := g.Update(key("X"))
	g2 := nm2.(Model)
	if g2.pendingRestart {
		t.Fatal("pending flag should clear after confirm")
	}
	if len(g2.state.Models) != 0 {
		t.Fatal("second X should restart (clear models)")
	}
	if g2.state.Resources.Cash != m.cfg.StartingCash {
		t.Fatalf("cash should reset to start, got %v", g2.state.Resources.Cash)
	}
}

func TestManualRestartCancelledByOtherKey(t *testing.T) {
	m := testModel(t)
	m.state.Models = []model.Model{{Online: true}}
	nm, _ := m.Update(key("X")) // arm
	nm2, _ := nm.(Model).Update(key("1"))
	g := nm2.(Model)
	if g.pendingRestart {
		t.Fatal("a non-X key should cancel the pending restart")
	}
	if len(g.state.Models) == 0 {
		t.Fatal("cancel should not restart the run")
	}
}

func TestBankruptcyAutoRestarts(t *testing.T) {
	m := testModel(t)
	m.state.Models = []model.Model{{Online: true, Users: 1}}
	m.state.Resources.Cash = -(m.cfg.BankruptcyDebtRatio * m.cfg.StartingCash) - 1
	nm, _ := m.Update(tickMsg(time.Unix(0, 0)))
	g := nm.(Model)
	if g.state.Resources.Cash < 0 {
		t.Fatalf("bankruptcy should reset to positive cash, got %v", g.state.Resources.Cash)
	}
	if len(g.state.Models) != 0 {
		t.Fatal("bankruptcy restart should clear models")
	}
	if g.notice == "" {
		t.Fatal("bankruptcy should surface a notice banner")
	}
}

func TestBankruptcySkipsActiveCampaign(t *testing.T) {
	m := testModel(t)
	m.state.Campaign.Doctrine = model.DoctrineConsumer
	m.state.Campaign.Cycle = 4
	m.state.Models = []model.Model{{Online: true, Users: 1}}
	debt := -(m.cfg.BankruptcyDebtRatio * m.cfg.StartingCash) - 1
	m.state.Resources.Cash = debt
	patentsBefore := m.state.Prestige.Patents
	nm, _ := m.Update(tickMsg(time.Unix(0, 0)))
	g := nm.(Model)
	// Tick may nudge cash slightly, but bankruptcy must not call Restart
	// (which would restore StartingCash and clear models/campaign).
	if g.state.Resources.Cash >= 0 || g.state.Resources.Cash == m.cfg.StartingCash {
		t.Fatalf("active campaign must not auto-restart, cash=%v", g.state.Resources.Cash)
	}
	if len(g.state.Models) == 0 {
		t.Fatal("models cleared — bankruptcy restart fired")
	}
	if g.state.Campaign.Doctrine != model.DoctrineConsumer || g.state.Campaign.Cycle != 4 {
		t.Fatalf("campaign state reset on bankruptcy: %+v", g.state.Campaign)
	}
	if g.state.Prestige.Patents != patentsBefore {
		t.Fatalf("patents changed on skipped bankruptcy: %v", g.state.Prestige.Patents)
	}
}
