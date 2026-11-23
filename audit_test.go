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
