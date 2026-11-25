package main

import (
	"reflect"
	"strings"
	"testing"

	"google.golang.org/adk/v2/workflow"
)

func TestMedicalAuditInput(t *testing.T) {
	resp := &CoreAgentResponse{
		MedicalFactors: map[string]string{"J00": "common cold"},
		History:        []Turn{{Role: "user", Message: "I have a cold"}},
	}

	got := medicalAuditInput(resp)
	want := MedicalAuditInput{MedicalFactors: resp.MedicalFactors, History: resp.History}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("medicalAuditInput() = %+v, want %+v", got, want)
	}

	if got := medicalAuditInput(nil); !reflect.DeepEqual(got, MedicalAuditInput{}) {
		t.Errorf("medicalAuditInput(nil) = %+v, want zero value", got)
	}
}

func TestNewMedicalAuditAgentWiring(t *testing.T) {
	a, err := newMedicalAuditAgent(nil)
	if err != nil {
		t.Fatal(err)
	}
	if a.Name() != medicalAuditAgentName {
		t.Errorf("agent name = %q, want %q", a.Name(), medicalAuditAgentName)
	}

	if _, err := workflow.NewAgentNodeTyped[MedicalAuditInput, MedicalAuditResult](a, workflow.NodeConfig{}); err != nil {
		t.Fatal(err)
	}
}

func TestMedicalAuditFindingsArtifact(t *testing.T) {
	// readArtifact resolves "medical" against the same *-audit-findings.md glob
	// the per-factor findings use, so the file must exist under that name.
	findings, err := readArtifact(findingsForms, "medical", "audit-findings")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"overwhelming_medical_info", "medical_research", "trivial_condition"} {
		if !strings.Contains(findings, want) {
			t.Errorf("medical audit findings missing %q", want)
		}
	}
}
