package tui

import (
	"strings"
	"testing"
)

func TestTeamPageShowsOfficeStub(t *testing.T) {
	m := testModel(t)
	m.page = PageTeam
	m.state.Office.Level = 1
	v := renderTeam(m)
	for _, w := range []string{"團隊", "辦公室", "車庫", "在職", "人才市場"} {
		if !strings.Contains(v, w) {
			t.Errorf("team page missing %q:\n%s", w, v)
		}
	}
}
