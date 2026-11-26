package main

import (
	"encoding/json"
	"errors"
	"iter"
	"reflect"
	"strings"
	"testing"

	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/session"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/functiontool"
	"google.golang.org/adk/v2/workflow"
)

var testFactorIDs = map[string]string{"E11.9": "dm2", "E11.65": "dm2"}

func TestAuditInstructionFor(t *testing.T) {
	dm2, err := auditInstructionFor("E11.9: type 2 diabetes", testFactorIDs)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(dm2, auditBaseInstruction) || !strings.Contains(dm2, "`a1c` and `duration`") {
		t.Errorf("E11.9 instruction lacks the DM2 findings: %q", dm2)
	}

	for _, factor := range []string{"tobacco", "I10: hypertension"} {
		other, err := auditInstructionFor(factor, testFactorIDs)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(other, auditBaseInstruction) || strings.Contains(other, "`a1c`") {
			t.Errorf("%s instruction should use the default findings: %q", factor, other)
		}
	}
}

func TestAuditFactor(t *testing.T) {
	input, err := json.Marshal(AuditInput{Factor: "E11.9: type 2 diabetes", History: []Turn{{Role: "user", Message: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}

	if got := auditFactor(string(input)); got != "E11.9: type 2 diabetes" {
		t.Errorf("auditFactor(json) = %q", got)
	}
	if got := auditFactor(" age "); got != "age" {
		t.Errorf("auditFactor(plain) = %q", got)
	}
}

func TestPickFactorState(t *testing.T) {
	states := map[string]map[string]any{
		"dm2": {"a1c": 7.2},
		"AGE": {"age": 41},
	}

	got, err := pickFactorState(states, "E11.9: type 2 diabetes", testFactorIDs)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, map[string]any{"a1c": 7.2}) {
		t.Errorf("dm2 state = %v", got)
	}

	if got, err = pickFactorState(states, "age", testFactorIDs); err != nil || got["age"] != 41 {
		t.Errorf("age state = %v, %v", got, err)
	}

	if _, err := pickFactorState(states, "bmi", testFactorIDs); err == nil {
		t.Error("known factor without a state: want error")
	}
	if _, err := pickFactorState(states, "I10: hypertension", testFactorIDs); err == nil || !strings.Contains(err.Error(), "unknown factor") {
		t.Errorf("unknown factor error = %v", err)
	}
}

// fakeState is a session.ReadonlyState backed by a map.
type fakeState map[string]any

func (s fakeState) Get(key string) (any, error) {
	v, ok := s[key]
	if !ok {
		return nil, session.ErrStateKeyNotExist
	}

	return v, nil
}

func (s fakeState) All() iter.Seq2[string, any] {
	return func(yield func(string, any) bool) {
		for k, v := range s {
			if !yield(k, v) {
				return
			}
		}
	}
}

func TestCoreSessionID(t *testing.T) {
	// Before the core agent has replied the key is absent, which callers can detect.
	if _, err := coreSessionID(fakeState{}); !errors.Is(err, session.ErrStateKeyNotExist) {
		t.Errorf("missing session ID error = %v, want ErrStateKeyNotExist", err)
	}

	if got, err := coreSessionID(fakeState{coreSessionIDKey: "abc"}); err != nil || got != "abc" {
		t.Errorf("coreSessionID() = %q, %v", got, err)
	}

	if _, err := coreSessionID(fakeState{coreSessionIDKey: 42}); err == nil || errors.Is(err, session.ErrStateKeyNotExist) {
		t.Errorf("non-string session ID error = %v, want a shape error", err)
	}
}

func TestReadFactorIDs(t *testing.T) {
	// Typed, and generic JSON as after a state round trip.
	for name, stored := range map[string]any{
		"typed":   map[string]string{"E11.9": "dm2"},
		"generic": map[string]any{"E11.9": "dm2"},
	} {
		got, err := readFactorIDs(fakeState{factorIDsKey: stored})
		if err != nil || !reflect.DeepEqual(got, map[string]string{"E11.9": "dm2"}) {
			t.Errorf("%s: readFactorIDs() = %v, %v", name, got, err)
		}
	}

	if _, err := readFactorIDs(fakeState{}); !errors.Is(err, session.ErrStateKeyNotExist) {
		t.Errorf("missing key error = %v, want ErrStateKeyNotExist", err)
	}
}

func TestResolveFactorKey(t *testing.T) {
	for id, want := range map[string]string{
		"dm2":                    "dm2",
		" Age ":                  "age",
		"tobacco":                "tobacco",
		"E11.9: type 2 diabetes": "dm2",
		"e11.65":                 "dm2",
	} {
		if got, ok := resolveFactorKey(id, testFactorIDs); !ok || got != want {
			t.Errorf("resolveFactorKey(%q) = %q, %v; want %q", id, got, ok, want)
		}
	}

	// Only the part before the colon counts, and the core agent's mapping is exact.
	for _, id := range []string{"I10: hypertension", "I10: dm2", "E11", "E11.2", "", "../x"} {
		if got, ok := resolveFactorKey(id, testFactorIDs); ok {
			t.Errorf("resolveFactorKey(%q) = %q, want unresolved", id, got)
		}
	}

	// Without a mapping from the core agent, only the default factors resolve.
	if _, ok := resolveFactorKey("E11.9", nil); ok {
		t.Error("E11.9 resolved without a mapping")
	}
	if got, ok := resolveFactorKey("bmi", nil); !ok || got != "bmi" {
		t.Errorf("bmi = %q, %v", got, ok)
	}
}

func TestInputRulesFor(t *testing.T) {
	rules, err := inputRulesFor("E11.9: type 2 diabetes", testFactorIDs)
	if err != nil || !strings.Contains(rules.Content, "a1c") {
		t.Errorf("dm2 rules = %+v, %v", rules, err)
	}

	if _, err := inputRulesFor("I10: hypertension", testFactorIDs); err == nil {
		t.Error("unknown factor: want error")
	}

	// A known factor without authored rules is not an error.
	rules, err = inputRulesFor("age", testFactorIDs)
	if err != nil || rules.Content != "" {
		t.Errorf("age rules = %+v, %v", rules, err)
	}
}

func TestAuditToolsAndAgentWiring(t *testing.T) {
	rulesTool, err := functiontool.New(functiontool.Config{Name: "input_rules", Description: "d"}, inputRules)
	if err != nil {
		t.Fatal(err)
	}
	stateTool, err := functiontool.New(functiontool.Config{Name: "factor_state", Description: "d"}, newFactorStateTool(sessionFactorState))
	if err != nil {
		t.Fatal(err)
	}

	a, err := llmagent.New(llmagent.Config{
		Name:                "audit_agent",
		InstructionProvider: auditInstruction,
		Tools:               []tool.Tool{rulesTool, stateTool},
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := workflow.NewAgentNodeTyped[AuditInput, AuditResult](a, workflow.NodeConfig{}); err != nil {
		t.Fatal(err)
	}
}

func TestAuditOutcome(t *testing.T) {
	passingResults := []AuditResult{{Factor: "age", Passed: true}, {Factor: "bmi", Passed: true}}
	failingResults := []AuditResult{{Factor: "age", Passed: true}, {Factor: "dm2", Passed: false, Reason: "no a1c in chat"}}

	joined := func(results []AuditResult, global MedicalAuditResult) map[string]any {
		return map[string]any{
			parallelAuditorsNodeName:   results,
			medicalAuditResultNodeName: global,
		}
	}

	// Everything passes: no failure, caller falls back to the core agent's reply.
	if failure, message, err := auditOutcome(joined(passingResults, MedicalAuditResult{Passed: true})); err != nil || failure != nil || message != "" {
		t.Errorf("all passed: failure=%v message=%q err=%v", failure, message, err)
	}

	// A failed factor alone: reported, generic message, no Global on the failure.
	failure, message, err := auditOutcome(joined(failingResults, MedicalAuditResult{Passed: true}))
	if err != nil || failure == nil || len(failure.FactorResults) != 1 || failure.FactorResults[0].Factor != "dm2" ||
		failure.Global != nil || message != auditFailureMessage {
		t.Errorf("factor failure: failure=%+v message=%q err=%v", failure, message, err)
	}

	// A non-security global failure (e.g. trivial_condition): reported with Global set, generic message.
	trivial := MedicalAuditResult{Passed: false, Findings: []Finding{{Factor: "J00", ReasonType: ReasonTrivialCondition}}}
	failure, message, err = auditOutcome(joined(passingResults, trivial))
	if err != nil || failure == nil || len(failure.FactorResults) != 0 || failure.Global == nil ||
		failure.Global.Passed || message != auditFailureMessage {
		t.Errorf("trivial condition failure: failure=%+v message=%q err=%v", failure, message, err)
	}

	// A security-risk global failure (overwhelming info / medical research): distinct message.
	risky := MedicalAuditResult{Passed: false, SecurityRisk: true, Findings: []Finding{{ReasonType: ReasonOverwhelmingInfo}}}
	if failure, message, err = auditOutcome(joined(passingResults, risky)); err != nil || failure == nil ||
		failure.Global == nil || !failure.Global.SecurityRisk || message != securityRiskMessage {
		t.Errorf("security risk failure: failure=%+v message=%q err=%v", failure, message, err)
	}

	// Both a failed factor and a security risk: both carried, the security message wins.
	if failure, message, err = auditOutcome(joined(failingResults, risky)); err != nil || failure == nil ||
		len(failure.FactorResults) != 1 || failure.Global == nil || message != securityRiskMessage {
		t.Errorf("combined failure: failure=%+v message=%q err=%v", failure, message, err)
	}
}

func TestAuditOutcomeDecodeError(t *testing.T) {
	// An undecodable shape at either key is an error, not a silent pass.
	if _, _, err := auditOutcome(map[string]any{
		parallelAuditorsNodeName:   "not a list",
		medicalAuditResultNodeName: MedicalAuditResult{Passed: true},
	}); err == nil {
		t.Error("undecodable factor results: want error")
	}
	if _, _, err := auditOutcome(map[string]any{
		parallelAuditorsNodeName:   []AuditResult{},
		medicalAuditResultNodeName: "not a result",
	}); err == nil {
		t.Error("undecodable global result: want error")
	}
}

func TestAuditInputsCarryHistory(t *testing.T) {
	resp := &CoreAgentResponse{
		MedicalFactors: map[string]string{"E11.9": "type 2 diabetes"},
		History:        []Turn{{Role: "user", Message: "my A1C is 7"}},
	}

	inputs := auditInputs(resp)
	if len(inputs) != 4 || inputs[0].Factor != "E11.9: type 2 diabetes" {
		t.Fatalf("auditInputs() = %v", inputs)
	}
	for _, in := range inputs {
		if !reflect.DeepEqual(in.History, resp.History) {
			t.Errorf("%s: history = %v", in.Factor, in.History)
		}
	}
}
