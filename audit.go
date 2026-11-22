package main

import (
	"embed"
	"fmt"
	"strings"

	"google.golang.org/adk/v2/agent"
)

//go:embed artifacts/*-audit-findings.md
var findingsForms embed.FS

// findingsFormFiles maps known factor identifiers (ICD-10 code, factor id),
// to the embedded file holding that factor's findings form.
var findingsFormFiles = map[string]string{
	"e11.9": "artifacts/dm2-audit-findings.md",
	"dm2":   "artifacts/dm2-audit-findings.md",
}

type InputRules struct {
	FactorICD10 string            `json:"factor_icd10"`
	Description string            `json:"description"`
	Fields      map[string]string `json:"fields"` // name -> rule or description of the field
}

// FindingsItem is currently unused by auditFindingsForm (see FindingsForm.Content).
type FindingsItem struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Answer      any    `json:"answer"`
}

type FindingsForm struct {
	FactorICD10 string         `json:"factor_icd10"`
	Description string         `json:"description"`
	Content     string         `json:"content"` // raw markdown findings form
	Items       []FindingsItem `json:"items"`
}

// AuditResult is the outcome of auditing one factor.
type AuditResult struct {
	Factor   string `json:"factor"`
	Passed   bool   `json:"passed"`
	Reason   string `json:"reason"`
	Findings string `json:"findings"` // findings as a raw string
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
	form := FindingsForm{
		FactorICD10: factorICD10,
		Description: "Input audit findings form for the medical factor " + factorICD10,
	}

	filename, ok := lookupFindingsFormFile(factorICD10)
	if !ok {
		form.Description += " (no findings form authored yet for this factor)"

		return form, nil
	}

	content, err := findingsForms.ReadFile(filename)
	if err != nil {
		return FindingsForm{}, fmt.Errorf("failed to read findings form %q: %w", filename, err)
	}
	form.Content = string(content)

	return form, nil
}

// lookupFindingsFormFile resolves a factor identifier to its embedded
// findings form file. It handles factor identifiers of the form "<code>: <name>"
// (e.g. "E11.9: type 2 diabetes"), as produced by factorsToAudit.
func lookupFindingsFormFile(factorID string) (string, bool) {
	for _, part := range strings.SplitN(factorID, ":", 2) {
		key := strings.ToLower(strings.TrimSpace(part))
		if key == "" {
			continue
		}
		if filename, ok := findingsFormFiles[key]; ok {
			return filename, true
		}
	}

	return "", false
}
