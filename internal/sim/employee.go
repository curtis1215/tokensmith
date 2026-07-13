package sim

import (
	"math"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

// effectiveOfficeLevel returns Office.Level, floored at 1 (unset/0 → starter HQ).
func effectiveOfficeLevel(ns model.GameState) int {
	if ns.Office.Level < 1 {
		return 1
	}
	return ns.Office.Level
}

// deskExtraSeats sums ExtraSeats from employee skills, soft-capped at +2
// (design: d-desk-layout stacks at most twice company-wide).
func deskExtraSeats(ns model.GameState, b balance.Config) int {
	extra := 0
	for _, e := range ns.Employees {
		for _, id := range e.SkillIDs {
			if sk, ok := balance.SkillByID(b, id); ok {
				extra += sk.ExtraSeats
			}
		}
	}
	if extra > 2 {
		extra = 2
	}
	return extra
}

// seatCap is office seats at the effective level plus capped skill ExtraSeats.
func seatCap(ns model.GameState, b balance.Config) int {
	return balance.OfficeSeatsAt(effectiveOfficeLevel(ns), b) + deskExtraSeats(ns, b)
}

// rosterFull reports whether the employee roster is at seat capacity.
func rosterFull(ns model.GameState, b balance.Config) bool {
	return len(ns.Employees) >= seatCap(ns, b)
}

// employeeSelfStatMult multiplies all-role power from the holder's skills.
func employeeSelfStatMult(e model.Employee, b balance.Config) float64 {
	m := 1.0
	for _, id := range e.SkillIDs {
		sk, ok := balance.SkillByID(b, id)
		if !ok || sk.SelfStatMult <= 0 {
			continue
		}
		m *= sk.SelfStatMult
	}
	return m
}

// employeeSecondaryWeight is SecondaryWeight, optionally overridden by a skill.
func employeeSecondaryWeight(e model.Employee, b balance.Config) float64 {
	w := b.SecondaryWeight
	for _, id := range e.SkillIDs {
		sk, ok := balance.SkillByID(b, id)
		if !ok || sk.SecondaryWeight <= 0 {
			continue
		}
		w = sk.SecondaryWeight
	}
	return w
}

// employeeSelfRolePowerMult returns the per-role self power mult from skills.
// Skills with SelfRolePowerMult activate when PreferRole matches PrimaryRole
// (or when HasPrefer is false — then applied to PrimaryRole).
func employeeSelfRolePowerMult(e model.Employee, b balance.Config) [model.NumRoles]float64 {
	var m [model.NumRoles]float64
	for i := range m {
		m[i] = 1
	}
	for _, id := range e.SkillIDs {
		sk, ok := balance.SkillByID(b, id)
		if !ok || sk.SelfRolePowerMult <= 0 {
			continue
		}
		var target model.Role
		if sk.HasPrefer {
			if e.PrimaryRole != sk.PreferRole {
				continue
			}
			target = sk.PreferRole
		} else {
			target = e.PrimaryRole
		}
		m[target] *= sk.SelfRolePowerMult
	}
	return m
}

// employeeRolePower computes this employee's contribution to each role's power
// using primary weight 1.0 / secondary weight (default 0.35), times self skill mults.
func employeeRolePower(e model.Employee, b balance.Config) [model.NumRoles]float64 {
	secW := employeeSecondaryWeight(e, b)
	statM := employeeSelfStatMult(e, b)
	roleM := employeeSelfRolePowerMult(e, b)
	var p [model.NumRoles]float64
	for r := model.Role(0); r < model.NumRoles; r++ {
		w := secW
		if r == e.PrimaryRole {
			w = b.PrimaryWeight
		}
		p[r] = float64(e.Stats[r]) * w * statM * roleM[r]
	}
	return p
}

// companyRolePowerAdd sums CompanyRolePower contributions across hired skills.
// Applied as (1+sum) on totalRolePower (additive within role, product across skills).
func companyRolePowerAdd(ns model.GameState, b balance.Config) [model.NumRoles]float64 {
	var add [model.NumRoles]float64
	for _, e := range ns.Employees {
		for _, id := range e.SkillIDs {
			sk, ok := balance.SkillByID(b, id)
			if !ok {
				continue
			}
			for r := range add {
				add[r] += sk.CompanyRolePower[r]
			}
		}
	}
	return add
}

// totalRolePower sums employeeRolePower across the roster, then scales by
// company-wide CompanyRolePower skill hooks: total[r] *= (1 + sum).
func totalRolePower(ns model.GameState, b balance.Config) [model.NumRoles]float64 {
	var total [model.NumRoles]float64
	for _, e := range ns.Employees {
		p := employeeRolePower(e, b)
		for r := range total {
			total[r] += p[r]
		}
	}
	add := companyRolePowerAdd(ns, b)
	for r := range total {
		total[r] *= 1 + add[r]
	}
	return total
}

// skillPassives aggregates hired-employee passive skill multipliers.
// Mult fields use product-of-nonzero convention (0 = unused in catalog).
// Empty roster → all mults 1 (neutral).
type skillPassives struct {
	TokenRnDMult     float64
	InfraMult        float64
	UserGrowthMult   float64
	ChurnMult        float64
	TrainQualityMult float64
	RevenueMult      float64
	EventNegMult     float64
}

// passiveSkillEffects products skill mult hooks from the hired roster.
func passiveSkillEffects(ns model.GameState, b balance.Config) skillPassives {
	p := skillPassives{
		TokenRnDMult:     1,
		InfraMult:        1,
		UserGrowthMult:   1,
		ChurnMult:        1,
		TrainQualityMult: 1,
		RevenueMult:      1,
		EventNegMult:     1,
	}
	for _, e := range ns.Employees {
		for _, id := range e.SkillIDs {
			sk, ok := balance.SkillByID(b, id)
			if !ok {
				continue
			}
			if sk.TokenRnDMult > 0 {
				p.TokenRnDMult *= sk.TokenRnDMult
			}
			if sk.InfraMult > 0 {
				p.InfraMult *= sk.InfraMult
			}
			if sk.UserGrowthMult > 0 {
				p.UserGrowthMult *= sk.UserGrowthMult
			}
			if sk.ChurnMult > 0 {
				p.ChurnMult *= sk.ChurnMult
			}
			if sk.TrainQualityMult > 0 {
				p.TrainQualityMult *= sk.TrainQualityMult
			}
			if sk.RevenueMult > 0 {
				p.RevenueMult *= sk.RevenueMult
			}
			if sk.EventNegMult > 0 {
				p.EventNegMult *= sk.EventNegMult
			}
		}
	}
	return p
}

// staffRnDPerSecFromEmployees is R&D/sec from research RolePower × RnDPerPower,
// scaled by Research.EfficiencyMult (tech / future hooks).
func staffRnDPerSecFromEmployees(ns model.GameState, b balance.Config) float64 {
	p := totalRolePower(ns, b)[model.RoleResearcher]
	eff := ns.Research.EfficiencyMult
	if eff == 0 {
		// Uninitialized Research treats EfficiencyMult as neutral 1.0.
		eff = 1
	}
	return p * b.RnDPerPower * eff
}

// companySalaryMult multiplies every employee's pay (product of skill hooks).
func companySalaryMult(ns model.GameState, b balance.Config) float64 {
	m := 1.0
	for _, e := range ns.Employees {
		for _, id := range e.SkillIDs {
			sk, ok := balance.SkillByID(b, id)
			if !ok || sk.CompanySalaryMult <= 0 {
				continue
			}
			m *= sk.CompanySalaryMult
		}
	}
	return m
}

// employeeSelfSalaryMult is the holder's personal salary mult product.
func employeeSelfSalaryMult(e model.Employee, b balance.Config) float64 {
	m := 1.0
	for _, id := range e.SkillIDs {
		sk, ok := balance.SkillByID(b, id)
		if !ok || sk.SelfSalaryMult <= 0 {
			continue
		}
		m *= sk.SelfSalaryMult
	}
	return m
}

// totalSalaryPerSecFromEmployees converts each MonthlySalary (with skill mults)
// via balance.MonthlyToPerSec and sums.
func totalSalaryPerSecFromEmployees(ns model.GameState, b balance.Config) float64 {
	co := companySalaryMult(ns, b)
	var s float64
	for _, e := range ns.Employees {
		monthly := e.MonthlySalary * employeeSelfSalaryMult(e, b) * co
		s += balance.MonthlyToPerSec(monthly, b)
	}
	return s
}

// roleBonus is the diminishing staff-power curve:
// Cap * (1 - exp(-K * p / Ref)). Zero power → 0 (neutral).
func roleBonus(totalPower float64, b balance.Config) float64 {
	if totalPower <= 0 || b.StaffPowerRef <= 0 {
		return 0
	}
	return b.StaffPowerCap * (1 - math.Exp(-b.StaffPowerK*totalPower/b.StaffPowerRef))
}

// employeeInfraMult is 1 + diminishing engineer roleBonus (used by infraEfficiency).
func employeeInfraMult(ns model.GameState, b balance.Config) float64 {
	return 1 + roleBonus(totalRolePower(ns, b)[model.RoleEngineer], b)
}

// employeeMarketingMult is 1 + diminishing marketing roleBonus.
func employeeMarketingMult(ns model.GameState, b balance.Config) float64 {
	return 1 + roleBonus(totalRolePower(ns, b)[model.RoleMarketing], b)
}

// employeeOpsChurnFactor scales service-churn severity: lower is better.
// Shape mirrors legacy 1/(1+ops*reduction) with roleBonus as the ops strength.
func employeeOpsChurnFactor(ns model.GameState, b balance.Config) float64 {
	return 1 / (1 + roleBonus(totalRolePower(ns, b)[model.RoleOps], b))
}
