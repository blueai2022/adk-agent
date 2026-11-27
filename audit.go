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

// Session state keys under which the core sub-agent stores the ICD-10 -> factor
// ID mapping, the per-factor states and and the detected ICD-10 -> medical factors.
const (
	factorIDsKey      = "core:factor_ids"
	factorStatesKey   = "core:factor_states"
	medicalFactorsKey = "core:medical_factors"
)

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
func inputRules(ctx agent.Context, args FactorArgs) (InputRules, error) {
	factorIDs, err := readFactorIDs(ctx.State())
	if err != nil {
		return InputRules{}, err
	}

	return inputRulesFor(args.Factor, factorIDs)
}

// inputRulesFor returns the input rules of a factor, resolved against the
// ICD-10 -> factor ID mapping from the core agent.
func inputRulesFor(factorICD10 string, factorIDs map[string]string) (InputRules, error) {
	rules := InputRules{
		FactorICD10: factorICD10,
		Description: "Input interpretation rules for the medical factor " + factorICD10,
	}

	key, ok := resolveFactorKey(factorICD10, factorIDs)
	if !ok {
		return InputRules{}, fmt.Errorf("unknown factor %q", factorICD10)
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
	factorIDs, err := readFactorIDs(ctx.State())
	if err != nil {
		return nil, err
	}

	// The core sub-agent always stores factor states, so a missing key is a wiring bug.
	states, err := readState[map[string]map[string]any](ctx.State(), factorStatesKey)
	if err != nil {
		return nil, err
	}

	return pickFactorState(states, factorID, factorIDs)
}

// readFactorIDs reads the core agent's ICD-10 -> factor ID mapping from session state.
func readFactorIDs(state session.ReadonlyState) (map[string]string, error) {
	return readState[map[string]string](state, factorIDsKey)
}

// readState reads key from session state as a T. The stored value may be typed or,
// after a state round trip, generic JSON.
func readState[T any](state session.ReadonlyState, key string) (T, error) {
	raw, err := state.Get(key)
	if err != nil {
		var out T

		return out, fmt.Errorf("read %s: %w", key, err)
	}

	out, err := decodeAny[T](raw)
	if err != nil {
		return out, fmt.Errorf("%s has an unexpected shape: %w", key, err)
	}

	return out, nil
}

// decodeAny converts v - which may already be a T, or the generic map/slice shape
// JSON produces after a state or workflow JoinNode round trip - into a T, via a
// JSON marshal/unmarshal round trip.
func decodeAny[T any](v any) (T, error) {
	var out T

	data, err := json.Marshal(v)
	if err != nil {
		return out, err
	}

	if err := json.Unmarshal(data, &out); err != nil {
		return out, err
	}

	return out, nil
}

// pickFactorState finds the factor's entry in the core agent's factor states.
func pickFactorState(states map[string]map[string]any, factorID string, factorIDs map[string]string) (map[string]any, error) {
	factorKey, ok := resolveFactorKey(factorID, factorIDs)
	if !ok {
		return nil, fmt.Errorf("unknown factor %q", factorID)
	}

	for key, state := range states {
		if strings.EqualFold(key, factorKey) {
			return state, nil
		}
	}

	return nil, fmt.Errorf("core agent has no state for factor %q", factorID)
}

// auditInstruction is the audit agent's InstructionProvider: the base instruction
// followed by the findings (Output section) of the factor being audited.
func auditInstruction(ctx agent.ReadonlyContext) (string, error) {
	factorIDs, err := readFactorIDs(ctx.ReadonlyState())
	if err != nil {
		return "", err
	}

	return auditInstructionFor(auditFactor(contentText(ctx.UserContent())), factorIDs)
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

// auditInstructionFor builds the audit instruction for a factor. A factor that does not
// resolve, or has no findings file, gets the default findings.
func auditInstructionFor(factorID string, factorIDs map[string]string) (string, error) {
	key, ok := resolveFactorKey(factorID, factorIDs)
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

// resolveFactorKey resolves a factor identifier to its factor ID (e.g. "dm2").
// It handles a bare factor ID or ICD-10 code ("age", "E11.9") and the form
// "<code>: <name>" (e.g. "E11.9: type 2 diabetes"), as produced by factorsToAudit.
// Only the part before the colon is used. It is tried as a known factor ID first
// (a default factor or a value of factorIDs), then as an ICD-10 code in factorIDs,
// the mapping from the core agent. It reports false for an unknown factor.
func resolveFactorKey(factorID string, factorIDs map[string]string) (string, bool) {
	code, _, _ := strings.Cut(factorID, ":")
	code = strings.ToLower(strings.TrimSpace(code))
	if code == "" {
		return "", false
	}

	for _, id := range defaultFactors {
		if code == id {
			return id, true
		}
	}
	for _, id := range factorIDs {
		if id = strings.ToLower(strings.TrimSpace(id)); code == id {
			return id, true
		}
	}

	for icd10, id := range factorIDs {
		if strings.EqualFold(strings.TrimSpace(icd10), code) {
			return strings.ToLower(strings.TrimSpace(id)), true
		}
	}

	return "", false
}
