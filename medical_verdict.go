package main

import (
	"slices"
	"strings"

	"github.com/rs/zerolog/log"
)

// Verdict is LLM-friendly representation of the model's answer to one yes/no check.
type Verdict struct {
	// Detected: a LLM-friendly term in the JSON output for the model to understand
	// whether the issue was detected, see ...-findings.md.
	Detected bool   `json:"detected"` // audit-specific issue detected
	Reason   string `json:"reason"`   // required either way: the evidence, or why not
}

// MedicalSafetyVerdict is the output of the medical safety agent: an explicit
// answer to every check, so "no" is a stated answer rather than an omission.
type MedicalSafetyVerdict struct {
	OverwhelmingMedicalInfo Verdict `json:"overwhelming_medical_info"`
	MedicalResearch         Verdict `json:"medical_research"`
}

// Result derives the MedicalAuditResult: one global finding per detected check.
func (v MedicalSafetyVerdict) Result() MedicalAuditResult {
	var findings []Finding
	if v.OverwhelmingMedicalInfo.Detected {
		findings = append(findings, Finding{
			ReasonType: ReasonOverwhelmingInfo,
			Reason:     v.OverwhelmingMedicalInfo.Reason,
		})
	}
	if v.MedicalResearch.Detected {
		findings = append(findings, Finding{
			ReasonType: ReasonMedicalResearch,
			Reason:     v.MedicalResearch.Reason,
		})
	}

	return newMedicalAuditResult(findings)
}

// FactorVerdict is the model's answer for one detected medical factor.
type FactorVerdict struct {
	Factor  string `json:"factor"` // the ICD-10 code, as given in the input
	Trivial bool   `json:"trivial"`
	Reason  string `json:"reason"`
}

type MedicalFactorVerdict interface {
	AllFactors() []FactorVerdict
}

// TrivialConditionVerdict is the output of the medical audit agent: one entry per
// input factor, so none can be silently skipped.
type TrivialConditionVerdict struct {
	Factors []FactorVerdict `json:"factors"`
}

func (v TrivialConditionVerdict) AllFactors() []FactorVerdict {
	return v.Factors
}

// newMedicalAuditResult derives Passed and SecurityRisk from the findings.
func newMedicalAuditResult(findings []Finding) MedicalAuditResult {
	result := MedicalAuditResult{Passed: len(findings) == 0, Findings: findings}
	for _, f := range findings {
		if f.ReasonType.IsSecurityRisk() {
			result.SecurityRisk = true
		}
	}

	return result
}

// trivialConditionResult derives the MedicalAuditResult from the model's per-factor
// answers.
//
// Prompt asks the model to answer every factor in medicalFactors (ICD-10 -> description);
// one that is not requires a warning, not a refer to underwriter reason.
//
// An answer the model could not attribute to a factor (e.g. "unspecified") yields a
// log warning for the factor it left unanswered.
func trivialConditionResult(
	factorVerdicts []FactorVerdict,
	medicalFactors map[string]string,
) MedicalAuditResult {
	type answer struct {
		trivial bool
		reason  string
	}

	knownFactors := make(map[string]bool, len(medicalFactors))
	for icd10 := range medicalFactors {
		knownFactors[factorCode(icd10)] = true
	}

	answers := make(map[string]answer, len(factorVerdicts))

	for _, verdict := range factorVerdicts {
		key := factorCode(verdict.Factor)
		if !knownFactors[key] {
			// The model could not attribute this answer to any factor.
			log.Warn().
				Str("factor", key).
				Msgf("model output an un-attributable factor key %q", key)

			continue // unmatchable answer, not include
		}

		// On duplicate answers for a factor, a trivial verdict wins.
		if prev, seen := answers[key]; !seen || (verdict.Trivial && !prev.trivial) {
			answers[key] = answer{
				trivial: verdict.Trivial,
				reason:  verdict.Reason,
			}
		}
	}

	icd10s := make([]string, 0, len(medicalFactors))
	for icd10 := range medicalFactors {
		icd10s = append(icd10s, icd10)
	}
	slices.Sort(icd10s)

	var findings []Finding
	for _, icd10 := range icd10s {
		ans, exists := answers[factorCode(icd10)]
		switch {
		case !exists:
			log.Warn().
				Str("factor", icd10).
				Msg("trivial condition audit left a factor unanswered")

		case ans.trivial:
			findings = append(findings, Finding{
				Factor:     icd10,
				ReasonType: ReasonTrivialCondition,
				Reason:     ans.reason,
			})
		}
	}

	return newMedicalAuditResult(findings)
}

// factorCode normalizes a factor as echoed by the model: the part before any colon
// ("E11.9: type 2 diabetes" -> "e11.9"), trimmed and lowercased.
func factorCode(factor string) string {
	code, _, _ := strings.Cut(factor, ":")

	return strings.ToLower(strings.TrimSpace(code))
}
