package main

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/functiontool"
	"google.golang.org/adk/v2/workflow"
)

func TestAuditInstructionFor(t *testing.T) {
	dm2, err := auditInstructionFor("E11.9: type 2 diabetes")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(dm2, auditBaseInstruction) || !strings.Contains(dm2, "`a1c` and `duration`") {
		t.Errorf("E11.9 instruction lacks the DM2 findings: %q", dm2)
	}

	other, err := auditInstructionFor("tobacco_use")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(other, auditBaseInstruction) || strings.Contains(other, "`a1c`") {
		t.Errorf("tobacco_use instruction should use the default findings: %q", other)
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
	typed := map[string]map[string]any{
		"dm2": {"a1c": 7.2},
		"age": {"age": 41},
	}

	got, err := pickFactorState(typed, "E11.9: type 2 diabetes")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, map[string]any{"a1c": 7.2}) {
		t.Errorf("typed state = %v", got)
	}

	generic := map[string]any{"AGE": map[string]any{"age": 41.0}}
	got, err = pickFactorState(generic, "age")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, map[string]any{"age": 41.0}) {
		t.Errorf("generic state = %v", got)
	}

	if got, _ := pickFactorState(typed, "bmi"); got != nil {
		t.Errorf("unknown factor state = %v, want nil", got)
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
