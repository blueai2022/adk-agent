package main

import "google.golang.org/adk/v2/agent"

type InputRules struct {
	FactorICD10 string            `json:"factor_icd10"`
	Description string            `json:"description"`
	Fields      map[string]string `json:"fields"` // name -> rule or description of the field
}

type FindingsItem struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Answer      any    `json:"answer"`
}

type FindingsForm struct {
	FactorICD10 string         `json:"factor_icd10"`
	Description string         `json:"description"`
	Items       []FindingsItem `json:"items"`
}

// AuditResult is the outcome of auditing one factor.
type AuditResult struct {
	Factor   string `json:"factor"`
	Passed   bool   `json:"passed"`
	Reason   string `json:"reason"`
	Findings string `json:"findings"` // the filled-out findings form, as a raw string
}

// inputRules tool returns a set of input rules for medical factors.
func inputRules(_ agent.Context, factorICD10 string) (InputRules, error) {
	return InputRules{
		FactorICD10: factorICD10,
		Description: "Input ingestiong rules for the medical factor " + factorICD10,
	}, nil
}

// auditFindingsForm tool loads the audit findings form for medical factors.
func auditFindingsForm(_ agent.Context, factorICD10 string) (FindingsForm, error) {
	return FindingsForm{
		FactorICD10: factorICD10,
		Description: "Input audit findings form for the medical factor " + factorICD10,
	}, nil
}
