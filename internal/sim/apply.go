package sim

import (
	"errors"
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
	case model.HireStaff:
		return applyHireStaff(s, c, b)
	case model.FireStaff:
		return applyFireStaff(s, c)
	case model.UnlockTech:
		return applyUnlockTech(s, c, b)
	case model.BuyPrestigeNode:
		return applyBuyPrestigeNode(s, c, b)
	case model.PrestigeReset:
		return applyPrestigeReset(s, b)
	case model.SignStar:
		return applySignStar(s, c, b)
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
	default:
		return s, ErrUnknownCommand
	}
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
	if c.Gen < 1 || c.Gen > balance.MaxGen {
		return s, ErrInvalidGen
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
	cost := b.GenRnDCost[c.Gen] * te.TrainRnDMult
	if s.Resources.RnD < cost {
		return s, ErrInsufficientRnD
	}
	ns := s
	ns.Resources.RnD -= cost
	ns.HasTraining = true
	ns.Training = model.TrainingJob{
		Gen:           c.Gen,
		Segment:       c.Segment,
		Alloc:         c.Alloc,
		Price:         c.Price,
		WorkRemaining: b.GenTrainWorkGPUSec[c.Gen] * te.TrainWorkMult,
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

func applyHireStaff(s model.GameState, c model.HireStaff, b balance.Config) (model.GameState, error) {
	if c.Count <= 0 {
		return s, ErrInvalidCount
	}
	n := float64(c.Count)
	ns := s
	switch c.Role {
	case model.RoleResearcher:
		if c.Tier < model.Tier1 || c.Tier > model.Tier3 {
			return s, ErrInvalidTier
		}
		cost := n * b.ResearcherHireCost[c.Tier]
		if s.Resources.Cash < cost {
			return s, ErrInsufficientCash
		}
		ns.Resources.Cash -= cost
		ns.Research.Researchers[c.Tier] += c.Count
	case model.RoleEngineer:
		if s.Resources.Cash < n*b.EngineerHireCost {
			return s, ErrInsufficientCash
		}
		ns.Resources.Cash -= n * b.EngineerHireCost
		ns.Engineers += c.Count
	case model.RoleOps:
		if s.Resources.Cash < n*b.OpsHireCost {
			return s, ErrInsufficientCash
		}
		ns.Resources.Cash -= n * b.OpsHireCost
		ns.Ops += c.Count
	case model.RoleMarketing:
		if s.Resources.Cash < n*b.MarketingHireCost {
			return s, ErrInsufficientCash
		}
		ns.Resources.Cash -= n * b.MarketingHireCost
		ns.Marketing += c.Count
	default:
		return s, ErrInvalidRole
	}
	return ns, nil
}

func applyFireStaff(s model.GameState, c model.FireStaff) (model.GameState, error) {
	if c.Count <= 0 {
		return s, ErrInvalidCount
	}
	ns := s
	switch c.Role {
	case model.RoleResearcher:
		if c.Tier < model.Tier1 || c.Tier > model.Tier3 {
			return s, ErrInvalidTier
		}
		ns.Research.Researchers[c.Tier] = max0(ns.Research.Researchers[c.Tier] - c.Count)
	case model.RoleEngineer:
		ns.Engineers = max0(ns.Engineers - c.Count)
	case model.RoleOps:
		ns.Ops = max0(ns.Ops - c.Count)
	case model.RoleMarketing:
		ns.Marketing = max0(ns.Marketing - c.Count)
	default:
		return s, ErrInvalidRole
	}
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
	return ns, nil
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

func findStar(stars []model.Star, id string) (model.Star, bool) {
	for _, st := range stars {
		if st.ID == id {
			return st, true
		}
	}
	return model.Star{}, false
}

func applySignStar(s model.GameState, c model.SignStar, b balance.Config) (model.GameState, error) {
	st, ok := findStar(b.Stars, c.StarID)
	if !ok {
		return s, ErrInvalidStar
	}
	if isStarHired(s, st.ID) {
		return s, ErrAlreadyHired
	}
	if s.Resources.Cash < st.SigningCost {
		return s, ErrInsufficientCash
	}
	ns := s
	ns.Resources.Cash -= st.SigningCost
	ns.HiredStars = append(append([]string(nil), s.HiredStars...), st.ID)
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
