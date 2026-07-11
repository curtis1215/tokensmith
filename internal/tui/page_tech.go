package tui

import (
	"errors"
	"fmt"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
	"tokensmith/internal/sim"
)

var branchNames = [model.NumBranches]string{"演算法", "硬體基建", "商業營運", "對齊安全"}

var eraRoman = []string{"", "I", "II", "III", "IV", "V", "VI", "VII", "VIII", "IX", "X"}

func eraTitle(era int) string {
	if era >= 1 && era < len(eraRoman) {
		return "時代 " + eraRoman[era]
	}
	return fmt.Sprintf("時代 %d", era)
}

// techEntryKind discriminates visible tech-page rows (no ID string parsing at dispatch).
type techEntryKind int

const (
	techEntryFixed techEntryKind = iota
	techEntryGeneration
	techEntryBreakthrough
)

// techEntry is a TUI-only adapter over fixed catalog nodes, frontier generations,
// and era breakthroughs.
type techEntry struct {
	kind       techEntryKind
	fixedIndex int // TechNodes index when kind==fixed
	targetGen  int // frontier generation when kind==generation
	era        int
	branch     model.TechBranch
	label      string
	detail     string
}

// techCurrentEra is the player's progression era from MaxUnlockedGen.
func techCurrentEra(m Model) int {
	max := sim.MaxUnlockedGen(m.state, m.cfg)
	e, err := balance.EraForGen(max)
	if err != nil || e < 1 {
		return 1
	}
	return e
}

func techSelectedEra(m Model) int {
	if m.techEra > 0 {
		return m.techEra
	}
	return techCurrentEra(m)
}

// fixedNodeEra assigns fixed tech-tree nodes to Era I or II (or Gen5→III).
func fixedNodeEra(node model.TechNode) int {
	for g := 2; g <= 5; g++ {
		if node.ID == balance.GenUnlockNodeID(g) {
			e, err := balance.EraForGen(g)
			if err != nil {
				return 1
			}
			return e
		}
	}
	switch node.ID {
	case "process-N5", "process-N3", "process-N2", "infra-density-1", "align-incident-1":
		return 2
	}
	return 1
}

// techEraEntries builds interactive entries for an expanded era.
func techEraEntries(m Model, era int) []techEntry {
	var out []techEntry
	// Fixed catalog nodes for this era (Eras I–II and Gen5 in III).
	for i, node := range m.cfg.TechNodes {
		if fixedNodeEra(node) != era {
			continue
		}
		meta := techLabel(node.ID)
		out = append(out, techEntry{
			kind:       techEntryFixed,
			fixedIndex: i,
			era:        era,
			branch:     node.Branch,
			label:      meta.Name,
			detail:     meta.Effect,
		})
	}
	// Procedural content from Era III: frontier gens + breakthroughs.
	if era >= 3 {
		start, err1 := balance.EraStartGen(era)
		end, err2 := balance.EraEndGen(era)
		if err1 == nil && err2 == nil {
			for g := start; g <= end; g++ {
				if g < 6 {
					continue // Gen5 uses fixed model-gen-5
				}
				out = append(out, techEntry{
					kind:      techEntryGeneration,
					targetGen: g,
					era:       era,
					label:     generationEntryLabel(g),
					detail:    generationEntryDetail(g),
				})
			}
		}
		for b := 0; b < model.NumBranches; b++ {
			br := model.TechBranch(b)
			out = append(out, techEntry{
				kind:   techEntryBreakthrough,
				era:    era,
				branch: br,
				label:  breakthroughEntryLabel(br),
				detail: breakthroughEntryDetail(era, br, m),
			})
		}
	}
	return out
}

func generationEntryLabel(gen int) string {
	return fmt.Sprintf("前沿研究 · Gen%d", gen)
}

func generationEntryDetail(gen int) string {
	spec, err := balance.Generation(gen)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("R&D %s · 建議算力 %.0f", human(spec.FrontierRnD), spec.RecommendedCompute)
}

func breakthroughEntryLabel(br model.TechBranch) string {
	return fmt.Sprintf("%s 突破", branchNames[br])
}

func breakthroughEntryDetail(era int, br model.TechBranch, m Model) string {
	cost, err := sim.EraBreakthroughCost(m.state, era, br)
	if err != nil {
		if !sim.EraOpen(m.state, era) {
			return "時代未開放"
		}
		return "—"
	}
	// Ownership
	if eraBreakthroughOwned(m.state, era, br) {
		return "✓ 已解鎖"
	}
	return fmt.Sprintf("%s R&D", human(cost))
}

func eraBreakthroughOwned(s model.GameState, era int, br model.TechBranch) bool {
	for _, ep := range s.Progression.Eras {
		if ep.Era == era && ep.UnlockedMask&(1<<br) != 0 {
			return true
		}
	}
	return false
}

// techVisibleEntries are the cursor-navigable rows for the selected era.
// Past eras are collapsed (no rows); current is expanded; next is a non-empty
// preview list (still navigable for reading, Enter may no-op if locked).
func techVisibleEntries(m Model) []techEntry {
	sel := techSelectedEra(m)
	cur := techCurrentEra(m)
	if sel < cur {
		return nil
	}
	if sel > cur+1 {
		return nil
	}
	return techEraEntries(m, sel)
}

func renderTech(m Model) string {
	s := m.state
	cw := m.contentWidth()
	inner := m.cardInnerWidth()
	cur := techCurrentEra(m)
	sel := techSelectedEra(m)

	var rows []string
	// Header: R&D + frontier allocation
	alloc := 0
	if s.Progression.Frontier.Active {
		alloc = s.Progression.Frontier.AllocationPct
	}
	head := fmt.Sprintf("科技 · 可用 R&D %s · 前沿分配 %d%%  [+]/[-]", human(s.Resources.RnD), alloc)
	rows = append(rows, TruncateWidth(head, cw))

	// Era ladder strip: past collapsed, current expanded, next preview.
	// Always render each card's own era entries; cursor only on the selected era.
	maxShow := cur + 1
	if maxShow < 2 {
		maxShow = 2 // always show at least I–II structure
	}
	for era := 1; era <= maxShow; era++ {
		title := eraTitle(era)
		marker := " "
		if era == sel {
			marker = "▸"
		}
		switch {
		case era < cur:
			// Collapsed past summary.
			sum := fmt.Sprintf("%s %s  ·  已完成  %s", marker, title, eraProgressSummary(m, era))
			rows = append(rows, CardIn(CardDefault, cw, sum, styleMuted.Render("（收合 · [ ] 瀏覽）")))
		case era == cur:
			// Current era always shows its own entries (not selected-era entries).
			curEntries := techEraEntries(m, cur)
			body := renderTechEntryLines(m, curEntries, inner, sel == cur)
			if body == "" {
				body = styleMuted.Render("（無條目）")
			}
			rows = append(rows, CardIn(CardAccent, cw, fmt.Sprintf("%s %s  ·  當前", marker, title), body))
		case era == cur+1:
			// One-level next: interactive list when selected, muted preview otherwise.
			preview := techEraEntries(m, era)
			var body string
			if sel == cur+1 {
				body = renderTechEntryLines(m, preview, inner, true)
			} else {
				body = renderTechPreviewLines(m, preview, inner)
			}
			if body == "" {
				body = styleMuted.Render("（尚無預覽）")
			}
			cond := nextEraUnlockHint(m, era)
			hdr := fmt.Sprintf("%s %s  ·  預覽", marker, title)
			rows = append(rows, CardIn(CardDefault, cw, hdr, VStack(styleMuted.Render(cond), body)))
		}
	}

	help := styleMuted.Render("[↑↓]條目  [ ]時代  [Enter]執行  [+]/[-]前沿分配±10%  [P]傳承")
	rows = append(rows, TruncateWidth(help, cw))
	return VStack(rows...)
}

func eraFixedUnlockCounts(m Model, era int) (unlocked, total int) {
	for _, node := range m.cfg.TechNodes {
		if fixedNodeEra(node) != era {
			continue
		}
		total++
		if techUnlocked(m.state, node.ID) {
			unlocked++
		}
	}
	return unlocked, total
}

// eraProgressSummary formats the collapsed-past line. Eras I–II use fixed
// catalog nodes; Era III+ uses generation unlock counts plus breakthrough bits.
func eraProgressSummary(m Model, era int) string {
	if era < 3 {
		unlocked, total := eraFixedUnlockCounts(m, era)
		return fmt.Sprintf("%d/%d 已解鎖", unlocked, total)
	}
	genU, genT := 0, 0
	if start, err1 := balance.EraStartGen(era); err1 == nil {
		if end, err2 := balance.EraEndGen(era); err2 == nil {
			max := sim.MaxUnlockedGen(m.state, m.cfg)
			for g := start; g <= end; g++ {
				genT++
				if g <= max {
					genU++
				}
			}
		}
	}
	btU := 0
	for _, ep := range m.state.Progression.Eras {
		if ep.Era != era {
			continue
		}
		for b := 0; b < model.NumBranches; b++ {
			if ep.UnlockedMask&(1<<b) != 0 {
				btU++
			}
		}
		break
	}
	return fmt.Sprintf("世代 %d/%d · 突破 %d/%d", genU, genT, btU, model.NumBranches)
}

func nextEraUnlockHint(m Model, era int) string {
	if sim.EraOpen(m.state, era) {
		return "已可進入"
	}
	if era <= 2 {
		return "推進世代解鎖"
	}
	prev := era - 1
	end, err := balance.EraEndGen(prev)
	if err != nil {
		return "條件未明"
	}
	return fmt.Sprintf("需 Gen%d + 上時代至少 2 項突破", end)
}

func renderTechEntryLines(m Model, entries []techEntry, inner int, showCursor bool) string {
	if len(entries) == 0 {
		return ""
	}
	var lines []string
	for i, e := range entries {
		cursor := " "
		if showCursor && i == m.techCursor {
			cursor = "▸"
		}
		state, muted := techEntryState(m, e)
		line := fmt.Sprintf("%s %-22s %-18s | %s", cursor, e.label, state, e.detail)
		line = TruncateWidth(line, inner)
		if muted {
			line = styleMuted.Render(line)
		}
		lines = append(lines, line)
	}
	return VStack(lines...)
}

func renderTechPreviewLines(m Model, entries []techEntry, inner int) string {
	var lines []string
	// Preview: first few labels only, muted.
	limit := 4
	for i, e := range entries {
		if i >= limit {
			lines = append(lines, styleMuted.Render(TruncateWidth("…", inner)))
			break
		}
		line := TruncateWidth("  · "+e.label, inner)
		lines = append(lines, styleMuted.Render(line))
	}
	if len(lines) == 0 {
		return styleMuted.Render("（尚無預覽）")
	}
	return VStack(lines...)
}

func techEntryState(m Model, e techEntry) (state string, muted bool) {
	switch e.kind {
	case techEntryFixed:
		if e.fixedIndex < 0 || e.fixedIndex >= len(m.cfg.TechNodes) {
			return "?", true
		}
		node := m.cfg.TechNodes[e.fixedIndex]
		if techUnlocked(m.state, node.ID) {
			return styleGain.Render("✓") + " 已解鎖", false
		}
		if !prereqsMet(m.state, node.Prereqs) {
			return "🔒 前置未滿", true
		}
		return human(node.Cost) + " R&D", false
	case techEntryGeneration:
		max := sim.MaxUnlockedGen(m.state, m.cfg)
		if e.targetGen <= max {
			return styleGain.Render("✓") + " 已解鎖", false
		}
		if m.state.Progression.Frontier.Active && m.state.Progression.Frontier.TargetGen == e.targetGen {
			fp := m.state.Progression.Frontier
			pct := 0.0
			if fp.WorkTotal > 0 {
				pct = (1 - fp.WorkRemaining/fp.WorkTotal) * 100
			}
			return fmt.Sprintf("進行中 %.0f%%", pct), false
		}
		if e.targetGen == max+1 {
			return "可啟動", false
		}
		return "🔒 需前序世代", true
	case techEntryBreakthrough:
		if eraBreakthroughOwned(m.state, e.era, e.branch) {
			return styleGain.Render("✓") + " 已解鎖", false
		}
		if !sim.EraOpen(m.state, e.era) {
			return "🔒 時代未開", true
		}
		cost, err := sim.EraBreakthroughCost(m.state, e.era, e.branch)
		if err != nil {
			return "—", true
		}
		return human(cost) + " R&D", false
	}
	return "", false
}

func techUnlocked(s model.GameState, id string) bool {
	for _, u := range s.UnlockedTech {
		if u == id {
			return true
		}
	}
	return false
}

func prereqsMet(s model.GameState, prereqs []string) bool {
	for _, p := range prereqs {
		if !techUnlocked(s, p) {
			return false
		}
	}
	return true
}

// clampTechCursor keeps techCursor in range of visible entries.
func clampTechCursor(m *Model) {
	vis := techVisibleEntries(*m)
	if len(vis) == 0 {
		m.techCursor = 0
		return
	}
	if m.techCursor < 0 {
		m.techCursor = 0
	}
	if m.techCursor >= len(vis) {
		m.techCursor = len(vis) - 1
	}
}

// techMoveCursor steps the visible-entry cursor by delta (−1/√1).
func techMoveCursor(m *Model, delta int) {
	clampTechCursor(m)
	vis := techVisibleEntries(*m)
	if len(vis) == 0 {
		return
	}
	m.techCursor += delta
	if m.techCursor < 0 {
		m.techCursor = 0
	}
	if m.techCursor >= len(vis) {
		m.techCursor = len(vis) - 1
	}
}

// techShiftEra moves selected era by delta within [1, current+1].
func techShiftEra(m *Model, delta int) {
	cur := techCurrentEra(*m)
	sel := techSelectedEra(*m)
	sel += delta
	if sel < 1 {
		sel = 1
	}
	max := cur + 1
	if max < 2 {
		max = 2
	}
	if sel > max {
		sel = max
	}
	m.techEra = sel
	m.techCursor = 0
}

// techAdjustAllocation changes frontier AllocationPct by delta (±10), if active.
func techAdjustAllocation(m *Model, delta int) {
	if !m.state.Progression.Frontier.Active {
		m.setNotice("無進行中的前沿研究")
		return
	}
	ns, err := sim.Apply(m.state, model.SetFrontierAllocation{
		Percent: m.state.Progression.Frontier.AllocationPct + delta,
	}, m.cfg)
	if err != nil {
		m.setNotice("分配需在 0–100%")
		return
	}
	m.state = ns
	m.setNotice(fmt.Sprintf("前沿分配 %d%%", m.state.Progression.Frontier.AllocationPct))
}

// techActivate runs Enter on the selected visible entry.
func techActivate(m *Model) {
	clampTechCursor(m)
	vis := techVisibleEntries(*m)
	if len(vis) == 0 || m.techCursor < 0 || m.techCursor >= len(vis) {
		return
	}
	e := vis[m.techCursor]
	switch e.kind {
	case techEntryFixed:
		if e.fixedIndex < 0 || e.fixedIndex >= len(m.cfg.TechNodes) {
			return
		}
		node := m.cfg.TechNodes[e.fixedIndex]
		ns, err := sim.Apply(m.state, model.UnlockTech{NodeID: node.ID}, m.cfg)
		switch {
		case err == nil:
			m.state = ns
			m.setNotice("🔬 已解鎖：" + techLabel(node.ID).Name)
		case errors.Is(err, sim.ErrInsufficientRnD):
			m.setNotice("R&D 不足")
		case errors.Is(err, sim.ErrAlreadyUnlocked):
			m.setNotice("已解鎖")
		case errors.Is(err, sim.ErrPrereqNotMet):
			m.setNotice("前置科技未滿足")
		default:
			m.setNotice("無法解鎖")
		}
	case techEntryGeneration:
		ns, err := sim.Apply(m.state, model.StartFrontierProject{TargetGen: e.targetGen}, m.cfg)
		switch {
		case err == nil:
			m.state = ns
			m.setNotice(fmt.Sprintf("🚀 開始前沿研究 Gen%d", e.targetGen))
		case errors.Is(err, sim.ErrFrontierActive):
			m.setNotice("已有進行中的前沿研究")
		case errors.Is(err, sim.ErrInvalidFrontierTarget):
			m.setNotice("目標世代不可用")
		case errors.Is(err, sim.ErrEraNotOpen):
			m.setNotice("時代未開放")
		default:
			m.setNotice("無法啟動前沿研究")
		}
	case techEntryBreakthrough:
		ns, err := sim.Apply(m.state, model.UnlockEraBreakthrough{Era: e.era, Branch: e.branch}, m.cfg)
		switch {
		case err == nil:
			m.state = ns
			m.setNotice("✦ 突破：" + breakthroughEntryLabel(e.branch))
		case errors.Is(err, sim.ErrInsufficientRnD):
			m.setNotice("R&D 不足")
		case errors.Is(err, sim.ErrEraNotOpen):
			m.setNotice("時代未開放")
		case errors.Is(err, sim.ErrEraBreakthroughOwned):
			m.setNotice("已解鎖")
		default:
			m.setNotice("無法購買突破")
		}
	}
}

// techVisualOrder kept for any legacy callers (branch-major catalog order).
func techVisualOrder(nodes []model.TechNode) []int {
	order := make([]int, 0, len(nodes))
	for b := 0; b < model.NumBranches; b++ {
		for i, n := range nodes {
			if n.Branch == model.TechBranch(b) {
				order = append(order, i)
			}
		}
	}
	return order
}
