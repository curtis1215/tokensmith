package sim

import (
	"errors"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

// ErrUnknownCommand is returned by Apply for an unrecognized command type.
var ErrUnknownCommand = errors.New("sim: unknown command")

var (
	ErrTrainingInProgress  = errors.New("sim: training already in progress")
	ErrInsufficientRnD     = errors.New("sim: insufficient R&D")
	ErrInvalidGen          = errors.New("sim: invalid generation")
	ErrGenLocked           = errors.New("sim: generation not unlocked (need tech tree)")
	ErrInvalidAlloc        = errors.New("sim: allocation must sum to 1")
	ErrInvalidModelIndex   = errors.New("sim: invalid model index")
	ErrInvalidPrice        = errors.New("sim: price must be positive")
	ErrInsufficientCash    = errors.New("sim: insufficient cash")
	ErrProcessLocked       = errors.New("sim: process not unlocked")
	ErrInvalidProcess      = errors.New("sim: unknown process")
	ErrInsufficientPower   = errors.New("sim: datacenter power capacity exceeded")
	ErrInsufficientSpace   = errors.New("sim: datacenter rack space exceeded")
	ErrInvalidCount        = errors.New("sim: count must be positive")
	ErrInvalidTier         = errors.New("sim: invalid researcher tier")
	ErrInvalidRole         = errors.New("sim: invalid role")
	ErrInvalidTech         = errors.New("sim: unknown tech node")
	ErrPrereqNotMet        = errors.New("sim: tech prerequisites not met")
	ErrAlreadyUnlocked     = errors.New("sim: tech already unlocked")
	ErrInvalidPrestigeNode = errors.New("sim: unknown prestige node")
	ErrInsufficientPatents = errors.New("sim: insufficient patents")
	ErrPrestigeLocked      = errors.New("sim: prestige not unlocked")
	ErrInvalidStar         = errors.New("sim: unknown star")
	ErrAlreadyHired        = errors.New("sim: star already hired")
	ErrNotDraft            = errors.New("sim: model is not a publishable draft")
	ErrInvalidName         = errors.New("sim: model name must be 1–24 characters")
	ErrInvalidEventIndex   = errors.New("sim: invalid pending-event index")
	ErrInvalidEventChoice  = errors.New("sim: invalid event choice")

	ErrCampaignNeedsModel    = errors.New("sim: campaign needs an online model")
	ErrInvalidDoctrine       = errors.New("sim: invalid campaign doctrine")
	ErrDoctrineAlreadyChosen = errors.New("sim: doctrine already chosen")
	ErrInvalidDoctrinePerk   = errors.New("sim: invalid doctrine perk")
	ErrPerkChoiceNotReady    = errors.New("sim: doctrine perk choice not ready")
	ErrSecondaryNotReady     = errors.New("sim: secondary doctrine not ready")
	ErrPivotAlreadyUsed      = errors.New("sim: doctrine pivot already used")
	ErrPivotLocked           = errors.New("sim: doctrine pivot locked during showdown")

	ErrDirectiveUsed         = errors.New("sim: executive directive already used this cycle")
	ErrInvalidDirective      = errors.New("sim: invalid executive directive")
	ErrInvalidRivalTarget    = errors.New("sim: invalid rival target")
	ErrRivalAlreadyCountered = errors.New("sim: rival action already countered")

	ErrCampaignNotWon     = errors.New("sim: campaign has not been won")
	ErrInvalidLegacy      = errors.New("sim: invalid legacy choice")
	ErrStrategyExitLocked = errors.New("sim: strategy exit not unlocked")

	ErrEraNotOpen           = errors.New("sim: era not open")
	ErrEraBreakthroughOwned = errors.New("sim: era breakthrough already unlocked")
	ErrInvalidEraBranch     = errors.New("sim: invalid era branch")

	ErrFrontierActive            = errors.New("sim: frontier project already active")
	ErrInvalidFrontierTarget     = errors.New("sim: invalid frontier target generation")
	ErrNoFrontierProject         = errors.New("sim: no active frontier project")
	ErrInvalidFrontierAllocation = errors.New("sim: frontier allocation must be 0–100")

	ErrOfficeMaxed      = errors.New("sim: office already max level")
	ErrNoSeats          = errors.New("sim: no free office seats")
	ErrUnknownEmployee  = errors.New("sim: unknown employee id")
	ErrUnknownCandidate = errors.New("sim: unknown market candidate")
)

// Apply validates and applies a single player command, returning the new
// state. Pure: it does not mutate s.
func Apply(s model.GameState, cmd model.Command, b balance.Config) (model.GameState, error) {
	switch c := cmd.(type) {
	case model.RentCompute:
		return applyRentCompute(s, c, b)
	case model.StartTraining:
		return applyStartTraining(s, c, b)
	case model.SetPrice:
		return applySetPrice(s, c)
	case model.ExpandDatacenter:
		return applyExpandDatacenter(s, c, b)
	case model.BuildServer:
		return applyBuildServer(s, c, b)
	case model.UnlockTech:
		return applyUnlockTech(s, c, b)
	case model.UnlockEraBreakthrough:
		return applyUnlockEraBreakthrough(s, c, b)
	case model.StartFrontierProject:
		return applyStartFrontierProject(s, c, b)
	case model.SetFrontierAllocation:
		return applySetFrontierAllocation(s, c)
	case model.BuyPrestigeNode:
		return applyBuyPrestigeNode(s, c, b)
	case model.PrestigeReset:
		return applyPrestigeReset(s, b)
	case model.PublishModel:
		return applyPublishModel(s, c)
	case model.ResolveEvent:
		return resolveChoice(s, c.PendingIndex, c.Choice, false, b)
	case model.ChooseDoctrine:
		return applyChooseDoctrine(s, c, b)
	case model.ChooseDoctrinePerk:
		return applyChooseDoctrinePerk(s, c, b)
	case model.ChooseSecondaryDoctrine:
		return applyChooseSecondaryDoctrine(s, c, b)
	case model.PivotDoctrine:
		return applyPivotDoctrine(s, c, b)
	case model.IssueDirective:
		return applyIssueDirective(s, c, b)
	case model.CampaignPrestige:
		return applyCampaignPrestige(s, c, b)
	case model.CampaignContinue:
		return applyCampaignContinue(s)
	case model.CampaignExit:
		return applyCampaignExit(s, b)
	case model.UpgradeOffice:
		return applyUpgradeOffice(s, b)
	case model.HireEmployee:
		return applyHireEmployee(s, c, b)
	case model.FireEmployee:
		return applyFireEmployee(s, c, b)
	case model.RerollMarket:
		return applyRerollMarket(s, b)
	default:
		return s, ErrUnknownCommand
	}
}

func applyUpgradeOffice(s model.GameState, b balance.Config) (model.GameState, error) {
	level := effectiveOfficeLevel(s)
	cost, ok := balance.OfficeUpgradeCostAt(level, b)
	if !ok {
		return s, ErrOfficeMaxed
	}
	if s.Resources.Cash < cost {
		return s, ErrInsufficientCash
	}
	ns := s
	ns.Resources.Cash -= cost
	ns.Office.Level = level + 1
	return ns, nil
}

func applyHireEmployee(s model.GameState, c model.HireEmployee, b balance.Config) (model.GameState, error) {
	idx := -1
	var cand model.Employee
	for i, e := range s.Market.Candidates {
		if e.ID == c.CandidateID {
			idx = i
			cand = e
			break
		}
	}
	if idx < 0 {
		return s, ErrUnknownCandidate
	}
	if rosterFull(s, b) {
		return s, ErrNoSeats
	}
	cost := HireCostQuote(s, cand, b)
	if s.Resources.Cash < cost {
		return s, ErrInsufficientCash
	}
	ns := s
	ns.Resources.Cash -= cost
	// Remove candidate (clone pool).
	cands := make([]model.Employee, 0, len(s.Market.Candidates)-1)
	cands = append(cands, s.Market.Candidates[:idx]...)
	cands = append(cands, s.Market.Candidates[idx+1:]...)
	ns.Market.Candidates = cands
	// Append to roster (clone).
	ns.Employees = append(append([]model.Employee(nil), s.Employees...), cand)
	return ns, nil
}

func applyFireEmployee(s model.GameState, c model.FireEmployee, b balance.Config) (model.GameState, error) {
	idx := -1
	var emp model.Employee
	for i, e := range s.Employees {
		if e.ID == c.EmployeeID {
			idx = i
			emp = e
			break
		}
	}
	if idx < 0 {
		return s, ErrUnknownEmployee
	}
	sev := SeveranceQuote(emp, s, b)
	if s.Resources.Cash < sev {
		return s, ErrInsufficientCash
	}
	ns := s
	ns.Resources.Cash -= sev
	emps := make([]model.Employee, 0, len(s.Employees)-1)
	emps = append(emps, s.Employees[:idx]...)
	emps = append(emps, s.Employees[idx+1:]...)
	ns.Employees = emps
	return ns, nil
}

func applyRerollMarket(s model.GameState, b balance.Config) (model.GameState, error) {
	cost := RerollCostQuote(s, b)
	if s.Resources.Cash < cost {
		return s, ErrInsufficientCash
	}
	keepRefresh := s.Market.NextRefreshAt
	keepN := s.Market.RerollCount
	ns := s
	ns.Resources.Cash -= cost
	ns = regenerateCandidatesOnly(ns, b)
	ns.Market.NextRefreshAt = keepRefresh
	ns.Market.RerollCount = keepN + 1
	return ns, nil
}

// HireCostQuote is the cash charged to hire cand (base HireCost × company mults).
// Exported so TUI can display the same figure Apply deducts.
func HireCostQuote(s model.GameState, cand model.Employee, b balance.Config) float64 {
	return cand.HireCost * companyHireCostMult(s, b)
}

// RerollCostQuote is the cash charged for the next paid market reroll.
func RerollCostQuote(s model.GameState, b balance.Config) float64 {
	return balance.RerollCost(s.Market.RerollCount, b) * companyRerollBaseMult(s, b)
}

// SeatCap is office seats at the effective level plus capped skill ExtraSeats.
func SeatCap(ns model.GameState, b balance.Config) int {
	return seatCap(ns, b)
}

// EffectiveMonthlySalary is the roster pay amount for one employee after self
// and company salary mults (what TotalSalaryPerSec converts to cash/sec).
func EffectiveMonthlySalary(e model.Employee, ns model.GameState, b balance.Config) float64 {
	return e.MonthlySalary * employeeSelfSalaryMult(e, b) * companySalaryMult(ns, b)
}

// EffectiveMonthlySalaryForHire quotes a candidate's monthly pay if hired now.
// Temporarily includes cand on a cloned roster so their CompanySalaryMult
// (e.g. d-comp-opt) is reflected the same way post-hire payroll will.
func EffectiveMonthlySalaryForHire(cand model.Employee, ns model.GameState, b balance.Config) float64 {
	probe := ns
	probe.Employees = append(append([]model.Employee(nil), ns.Employees...), cand)
	return EffectiveMonthlySalary(cand, probe, b)
}

// TotalMonthlyPayroll is Σ EffectiveMonthlySalary for the roster (UI 月薪合計).
func TotalMonthlyPayroll(ns model.GameState, b balance.Config) float64 {
	co := companySalaryMult(ns, b)
	var sum float64
	for _, e := range ns.Employees {
		sum += e.MonthlySalary * employeeSelfSalaryMult(e, b) * co
	}
	return sum
}

// SeveranceQuote is cash charged to fire emp (same formula as Apply).
func SeveranceQuote(emp model.Employee, ns model.GameState, b balance.Config) float64 {
	return emp.MonthlySalary * b.SeveranceMonths *
		employeeSelfSeveranceMult(emp, b) * companySeveranceMult(ns, b)
}

// companyHireCostMult is the product of HireCostMult hooks across the roster.
func companyHireCostMult(ns model.GameState, b balance.Config) float64 {
	m := 1.0
	for _, e := range ns.Employees {
		for _, id := range e.SkillIDs {
			sk, ok := balance.SkillByID(b, id)
			if !ok || sk.HireCostMult <= 0 {
				continue
			}
			m *= sk.HireCostMult
		}
	}
	return m
}

// companyRerollBaseMult multiplies paid-reroll cost (e.g. gs-war-chest).
func companyRerollBaseMult(ns model.GameState, b balance.Config) float64 {
	m := 1.0
	for _, e := range ns.Employees {
		for _, id := range e.SkillIDs {
			sk, ok := balance.SkillByID(b, id)
			if !ok || sk.RerollBaseMult <= 0 {
				continue
			}
			m *= sk.RerollBaseMult
		}
	}
	return m
}

// employeeSelfSeveranceMult multiplies severance for the fired employee
// (skills with Family != "severance_company").
func employeeSelfSeveranceMult(e model.Employee, b balance.Config) float64 {
	m := 1.0
	for _, id := range e.SkillIDs {
		sk, ok := balance.SkillByID(b, id)
		if !ok || sk.SeveranceMult <= 0 {
			continue
		}
		if sk.Family == "severance_company" {
			continue
		}
		m *= sk.SeveranceMult
	}
	return m
}

// companySeveranceMult multiplies all severance via Family "severance_company".
func companySeveranceMult(ns model.GameState, b balance.Config) float64 {
	m := 1.0
	for _, e := range ns.Employees {
		for _, id := range e.SkillIDs {
			sk, ok := balance.SkillByID(b, id)
			if !ok || sk.SeveranceMult <= 0 || sk.Family != "severance_company" {
				continue
			}
			m *= sk.SeveranceMult
		}
	}
	return m
}

func applyRentCompute(s model.GameState, c model.RentCompute, b balance.Config) (model.GameState, error) {
	if _, ok := balance.ProcessByID(b.Processes, c.Process); !ok {
		return s, ErrInvalidProcess
	}
	if !ProcessUnlocked(s, b, c.Process) {
		return s, ErrProcessLocked
	}
	ns := s
	if c.Pool == model.PoolTraining {
		ns.Compute.RentedTraining = cloneCounts(s.Compute.RentedTraining)
		ns.Compute.RentedTraining[c.Process] = max0(ns.Compute.RentedTraining[c.Process] + c.Delta)
	} else {
		ns.Compute.RentedInference = cloneCounts(s.Compute.RentedInference)
		ns.Compute.RentedInference[c.Process] = max0(ns.Compute.RentedInference[c.Process] + c.Delta)
	}
	return ns, nil
}

func applyStartTraining(s model.GameState, c model.StartTraining, b balance.Config) (model.GameState, error) {
	if s.HasTraining {
		return s, ErrTrainingInProgress
	}
	spec, err := balance.Generation(c.Gen)
	if err != nil {
		return s, err
	}
	if c.Gen > MaxUnlockedGen(s, b) {
		return s, ErrGenLocked
	}
	var sum float64
	for _, a := range c.Alloc {
		if a < 0 {
			return s, ErrInvalidAlloc
		}
		sum += a
	}
	if sum < 0.999 || sum > 1.001 {
		return s, ErrInvalidAlloc
	}
	if c.Price <= 0 {
		return s, ErrInvalidPrice
	}
	te := techEffects(s, b)
	cost := spec.TrainRnD * te.TrainRnDMult
	if s.Resources.RnD < cost {
		return s, ErrInsufficientRnD
	}
	cashCost, err := QuoteTrainBoostCost(s, c.Gen, c.Boosts, b)
	if err != nil {
		return s, err
	}
	// Fail closed: never debit NaN/negative (invalid knobs must not mint cash).
	if math.IsNaN(cashCost) || math.IsInf(cashCost, 0) || cashCost < 0 {
		return s, balance.ErrInvalidTrainBoostConfig
	}
	// Zero-boost starts must not fail when Cash is already negative from rent.
	if cashCost > 0 && s.Resources.Cash < cashCost {
		return s, ErrInsufficientCash
	}
	bonus, err := balance.TrainBoostCashBonus(c.Gen, c.Boosts, b)
	if err != nil {
		return s, err
	}
	ns := s
	ns.Resources.RnD -= cost
	ns.Resources.Cash -= cashCost
	ns.HasTraining = true
	ns.Training = model.TrainingJob{
		Gen:           c.Gen,
		Segment:       c.Segment,
		Alloc:         c.Alloc,
		Price:         c.Price,
		WorkRemaining: spec.TrainWork * te.TrainWorkMult,
		Boosts:        c.Boosts,
		CashBonus:     bonus,
		BoostCashPaid: cashCost,
	}
	return ns, nil
}

func applySetPrice(s model.GameState, c model.SetPrice) (model.GameState, error) {
	if c.ModelIndex < 0 || c.ModelIndex >= len(s.Models) {
		return s, ErrInvalidModelIndex
	}
	if c.Price <= 0 {
		return s, ErrInvalidPrice
	}
	ns := s
	ns.Models = append([]model.Model(nil), s.Models...)
	ns.Models[c.ModelIndex].Price = c.Price
	return ns, nil
}

func applyExpandDatacenter(s model.GameState, c model.ExpandDatacenter, b balance.Config) (model.GameState, error) {
	power := c.PowerDelta
	if power < 0 {
		power = 0
	}
	slots := c.SlotDelta
	if slots < 0 {
		slots = 0
	}
	cost := power*b.PowerCostPerKW + slots*b.SlotCost
	if s.Resources.Cash < cost {
		return s, ErrInsufficientCash
	}
	ns := s
	ns.Resources.Cash -= cost
	ns.Datacenter.PowerCapacity += power
	ns.Datacenter.SlotCapacity += slots
	return ns, nil
}

// applyBuildServer builds one server from the named process into c.Pool.
func applyBuildServer(s model.GameState, c model.BuildServer, b balance.Config) (model.GameState, error) {
	p, ok := balance.ProcessByID(b.Processes, c.Process)
	if !ok {
		return s, ErrInvalidProcess
	}
	if !ProcessUnlocked(s, b, c.Process) {
		return s, ErrProcessLocked
	}
	server := model.Server{
		Pool:    c.Pool,
		Compute: p.Compute,
		PowerKW: p.PowerKW,
		Slots:   1,
	}
	capex := (p.BuyPrice + b.ChassisCost) * eventEffects(s, b).BuildCostMult
	if s.Resources.Cash < capex {
		return s, ErrInsufficientCash
	}
	usedPower, usedSlots := 0.0, 0.0
	for _, sv := range s.Servers {
		usedPower += sv.PowerKW
		usedSlots += sv.Slots
	}
	if usedPower+server.PowerKW > s.Datacenter.PowerCapacity {
		return s, ErrInsufficientPower
	}
	if usedSlots+server.Slots > s.Datacenter.SlotCapacity {
		return s, ErrInsufficientSpace
	}
	ns := s
	ns.Resources.Cash -= capex
	ns.Servers = append(append([]model.Server(nil), s.Servers...), server)
	return ns, nil
}

func max0(n int) int {
	if n < 0 {
		return 0
	}
	return n
}

func findTechNode(nodes []model.TechNode, id string) (model.TechNode, bool) {
	for _, n := range nodes {
		if n.ID == id {
			return n, true
		}
	}
	return model.TechNode{}, false
}

func applyUnlockTech(s model.GameState, c model.UnlockTech, b balance.Config) (model.GameState, error) {
	node, ok := findTechNode(b.TechNodes, c.NodeID)
	if !ok {
		return s, ErrInvalidTech
	}
	if isUnlocked(s, node.ID) {
		return s, ErrAlreadyUnlocked
	}
	for _, p := range node.Prereqs {
		if !isUnlocked(s, p) {
			return s, ErrPrereqNotMet
		}
	}
	cost := node.Cost * eventTechCostMult(s, node.Branch)
	if s.Resources.RnD < cost {
		return s, ErrInsufficientRnD
	}
	ns := s
	ns.Resources.RnD -= cost
	ns.UnlockedTech = append(append([]string(nil), s.UnlockedTech...), node.ID)
	// Contiguous model-gen-N unlocks advance run-scoped progression atomically.
	if _, ok := parseModelGenNodeID(node.ID); ok {
		ns.Progression.MaxUnlockedGen = MaxUnlockedGen(ns, b)
	}
	return ns, nil
}

// parseModelGenNodeID returns N for IDs of the form "model-gen-N".
func parseModelGenNodeID(id string) (int, bool) {
	const prefix = "model-gen-"
	if !strings.HasPrefix(id, prefix) {
		return 0, false
	}
	n, err := strconv.Atoi(id[len(prefix):])
	if err != nil || n < 2 {
		return 0, false
	}
	return n, true
}

func findPrestigeNode(nodes []model.PrestigeNode, id string) (model.PrestigeNode, bool) {
	for _, n := range nodes {
		if n.ID == id {
			return n, true
		}
	}
	return model.PrestigeNode{}, false
}

func applyBuyPrestigeNode(s model.GameState, c model.BuyPrestigeNode, b balance.Config) (model.GameState, error) {
	node, ok := findPrestigeNode(b.PrestigeNodes, c.NodeID)
	if !ok {
		return s, ErrInvalidPrestigeNode
	}
	if isPrestigeUnlocked(s, node.ID) {
		return s, ErrAlreadyUnlocked
	}
	if s.Prestige.Patents < node.Cost {
		return s, ErrInsufficientPatents
	}
	ns := s
	ns.Prestige.Patents -= node.Cost
	ns.Prestige.UnlockedPrestige = append(append([]string(nil), s.Prestige.UnlockedPrestige...), node.ID)
	return ns, nil
}

func applyPrestigeReset(s model.GameState, b balance.Config) (model.GameState, error) {
	// Active campaigns must settle via CampaignPrestige/Exit; old valuation gate
	// remains only for pre-campaign (no-doctrine) saves.
	if s.Campaign.Doctrine != model.DoctrineNone {
		return s, ErrCampaignNotWon
	}
	if s.PeakValuation < b.PrestigeUnlockValuation {
		return s, ErrPrestigeLocked
	}
	p := s.Prestige
	p.Patents += patentsFor(s.PeakValuation, b)
	ns := freshRun(p, b)
	ns.Events.RandState = s.Events.RandState
	return ns, nil
}

func applyPublishModel(s model.GameState, c model.PublishModel) (model.GameState, error) {
	if c.ModelIndex < 0 || c.ModelIndex >= len(s.Models) {
		return s, ErrInvalidModelIndex
	}
	m := s.Models[c.ModelIndex]
	// Draft is defined as Online == false && Users == 0
	if m.Online || m.Users != 0 {
		return s, ErrNotDraft
	}
	name := strings.TrimSpace(c.Name)
	if name == "" || utf8.RuneCountInString(name) > 24 {
		return s, ErrInvalidName
	}
	if c.Price <= 0 {
		return s, ErrInvalidPrice
	}
	ns := s
	ns.Models = append([]model.Model(nil), s.Models...)
	ns.Models[c.ModelIndex].Name = name
	ns.Models[c.ModelIndex].Price = c.Price
	ns.Models[c.ModelIndex].Online = true
	return ns, nil
}
