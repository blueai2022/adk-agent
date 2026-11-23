package main

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/session"
)

//go:embed artifacts/*-audit-findings.md
var findingsForms embed.FS

//go:embed artifacts/*-input-rules.md
var inputRulesDocs embed.FS

// defaultFindingsKey names the findings file used for factors without their own.
const defaultFindingsKey = "default"

// factorKeys maps known factor identifiers (ICD-10 code, factor id) to the factor
// key that names its artifact files (artifacts/<key>-input-rules.md,
// artifacts/<key>-audit-findings.md) and its entry in the core agent's factor states.
var factorKeys = map[string]string{
	"e11.9": "dm2",
	"dm2":   "dm2",
}

// auditBaseInstruction is the factor-independent part of the audit agent's instruction.
// The factor's findings file is appended to it as the Output section.
const auditBaseInstruction = `You are an independent second reader of an underwriting core agent. ` +
	`Your input is JSON: one "factor" and the "history" of the chat with the applicant. ` +
	`Read the chat yourself, then decide whether you agree with the state the core agent recorded ` +
	`for that factor, i.e. whether the extraction is safe. Use judgment; this is a review, not a re-do.

Tools:
- factor_state: the state the core agent recorded for the factor. Call it with the factor exactly as given.
- input_rules: the rules for conflicting values and preference order for the factor. ` +
	`These rules are binding. If none are authored for the factor, apply the same principles with judgment.

Principles:
- Upstream already normalizes values (units, rounding, conversions). Do not re-derive or second-guess that.
- You only care whether the chosen value (or lack of one) is justified by the chat and the preference order.
- When the chat has conflicting or unresolved values, an empty state, or a state that is asking the user, is safe. ` +
	`A state that picked a value the rules would not pick is a disagreement.
- A value in the state that the chat never supports is a disagreement.

`

// AuditInput is the input of one audit: a factor and the chat it is audited against.
type AuditInput struct {
	Factor  string `json:"factor"`
	History []Turn `json:"history"`
}

// FactorArgs is the argument of the input_rules and factor_state tools.
type FactorArgs struct {
	Factor string `json:"factor"` // the factor exactly as given in the audit input
}

// InputRules is the result of the input_rules tool.
type InputRules struct {
	FactorICD10 string `json:"factor_icd10"`
	Description string `json:"description"`
	Content     string `json:"content"` // raw markdown input interpretation rules
}

// FactorState is the result of the factor_state tool.
type FactorState struct {
	Factor      string         `json:"factor"`
	Description string         `json:"description"`
	State       map[string]any `json:"state"` // signal name (e.g. a1c) -> recorded value
}

// FactorStateFunc supplies the core agent's recorded state for one factor.
type FactorStateFunc func(ctx agent.Context, factorID string) (map[string]any, error)

// AuditResult is the outcome of auditing one factor.
type AuditResult struct {
	Factor       string `json:"factor"`
	Passed       bool   `json:"passed"` // true when the auditor agrees with the core agent's state
	Reason       string `json:"reason"`
	ConflictKind string `json:"conflict_kind"`
	Findings     string `json:"findings"` // findings as a raw string
}

// inputRules tool returns a set of input rules for medical factors.
func inputRules(_ agent.Context, args FactorArgs) (InputRules, error) {
	factorICD10 := args.Factor
	rules := InputRules{
		FactorICD10: factorICD10,
		Description: "Input interpretation rules for the medical factor " + factorICD10,
	}

	key, ok := resolveFactorKey(factorICD10)
	if !ok {
		rules.Description += " (no input rules authored yet for this factor)"

		return rules, nil
	}

	content, err := readArtifact(inputRulesDocs, key, "input-rules")
	if errors.Is(err, fs.ErrNotExist) {
		rules.Description += " (no input rules authored yet for this factor)"

		return rules, nil
	}
	if err != nil {
		return InputRules{}, err
	}
	rules.Content = content

	return rules, nil
}

// newFactorStateTool returns the factor_state tool function, which reports the
// core agent's recorded state for a factor as supplied by stateFn.
func newFactorStateTool(stateFn FactorStateFunc) func(agent.Context, FactorArgs) (FactorState, error) {
	return func(ctx agent.Context, args FactorArgs) (FactorState, error) {
		factorID := args.Factor
		state, err := stateFn(ctx, factorID)
		if err != nil {
			return FactorState{}, fmt.Errorf("failed to read state of factor %q: %w", factorID, err)
		}

		result := FactorState{
			Factor:      factorID,
			Description: "State recorded by the core agent for the medical factor " + factorID,
			State:       state,
		}
		if len(state) == 0 {
			result.Description += " (no state recorded: the core agent holds no value for this factor)"
		}

		return result, nil
	}
}

// sessionFactorState is the default FactorStateFunc. It reads the factor's entry
// from the core agent's factor states in session state, which are keyed by factor key.
func sessionFactorState(ctx agent.Context, factorID string) (map[string]any, error) {
	raw, err := ctx.State().Get("core:factor_states")
	if err != nil {
		if errors.Is(err, session.ErrStateKeyNotExist) {
			return nil, nil
		}

		return nil, err
	}

	return pickFactorState(raw, factorID)
}

// pickFactorState finds the factor's entry in the core agent's factor states.
func pickFactorState(raw any, factorID string) (map[string]any, error) {
	// The stored value may be typed or, after a state round trip, generic JSON.
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}

	var states map[string]map[string]any
	if err := json.Unmarshal(data, &states); err != nil {
		return nil, fmt.Errorf("core:factor_states has an unexpected shape: %w", err)
	}

	for _, candidate := range factorKeyCandidates(factorID) {
		for key, state := range states {
			if strings.EqualFold(key, candidate) {
				return state, nil
			}
		}
	}

	return nil, nil
}

// auditInstruction is the audit agent's InstructionProvider: the base instruction
// followed by the findings (Output section) of the factor being audited.
func auditInstruction(ctx agent.ReadonlyContext) (string, error) {
	return auditInstructionFor(auditFactor(contentText(ctx.UserContent())))
}

// auditFactor extracts the factor from the audit node's input, which is an
// AuditInput as JSON. Anything else is taken to be the factor itself.
func auditFactor(input string) string {
	var in AuditInput
	if err := json.Unmarshal([]byte(input), &in); err == nil && in.Factor != "" {
		return in.Factor
	}

	return strings.TrimSpace(input)
}

// auditInstructionFor builds the audit instruction for a factor.
func auditInstructionFor(factorID string) (string, error) {
	key, ok := resolveFactorKey(factorID)
	if !ok {
		key = defaultFindingsKey
	}

	findings, err := readArtifact(findingsForms, key, "audit-findings")
	if errors.Is(err, fs.ErrNotExist) {
		findings, err = readArtifact(findingsForms, defaultFindingsKey, "audit-findings")
	}
	if err != nil {
		return "", err
	}

	return auditBaseInstruction + findings, nil
}

// readArtifact reads artifacts/<key>-<kind>.md from files.
func readArtifact(files embed.FS, key, kind string) (string, error) {
	filename := fmt.Sprintf("artifacts/%s-%s.md", key, kind)

	content, err := files.ReadFile(filename)
	if err != nil {
		return "", fmt.Errorf("failed to read %q: %w", filename, err)
	}

	return string(content), nil
}

// factorKeyCandidates lists the lowercase identifiers a factor may be known by.
// It handles factor identifiers of the form "<code>: <name>"
// (e.g. "E11.9: type 2 diabetes"), as produced by factorsToAudit.
// The mapped factor key, if any, comes first.
func factorKeyCandidates(factorID string) []string {
	var candidates []string
	if key, ok := resolveFactorKey(factorID); ok {
		candidates = append(candidates, key)
	}

	for _, part := range strings.SplitN(factorID, ":", 2) {
		if key := strings.ToLower(strings.TrimSpace(part)); key != "" {
			candidates = append(candidates, key)
		}
	}

	return candidates
}

// resolveFactorKey resolves a factor identifier to its factor key.
// It handles factor identifiers of the form "<code>: <name>"
// (e.g. "E11.9: type 2 diabetes"), as produced by factorsToAudit.
func resolveFactorKey(factorID string) (string, bool) {
	for _, part := range strings.SplitN(factorID, ":", 2) {
		if key, ok := factorKeys[strings.ToLower(strings.TrimSpace(part))]; ok {
			return key, true
		}
	}

	return "", false
}
