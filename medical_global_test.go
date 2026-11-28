package main

import (
	"context"
	"reflect"
	"testing"

	"google.golang.org/genai"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/workflowagent"
	"google.golang.org/adk/v2/runner"
	"google.golang.org/adk/v2/workflow"
)

func TestMergeMedicalAuditResults(t *testing.T) {
	trivial := MedicalAuditResult{
		Passed:   false,
		Findings: []Finding{{Factor: "J00", ReasonType: ReasonTrivialCondition, Reason: "cold"}},
	}
	risky := MedicalAuditResult{
		Passed:       false,
		SecurityRisk: true,
		Findings:     []Finding{{ReasonType: ReasonOverwhelmingInfo, Reason: "lab dump"}},
	}
	passed := MedicalAuditResult{Passed: true}

	if got := mergeMedicalAuditResults(nil); !got.Passed || got.SecurityRisk || len(got.Findings) != 0 {
		t.Errorf("no results: %+v, want passed", got)
	}

	if got := mergeMedicalAuditResults([]MedicalAuditResult{passed, passed}); !got.Passed {
		t.Errorf("all passed: %+v, want passed", got)
	}

	// A non-security failure alone does not raise SecurityRisk.
	if got := mergeMedicalAuditResults([]MedicalAuditResult{passed, trivial}); got.Passed || got.SecurityRisk ||
		!reflect.DeepEqual(got.Findings, trivial.Findings) {
		t.Errorf("trivial only: %+v", got)
	}

	// Findings concatenate in the order given; SecurityRisk is re-derived from them.
	got := mergeMedicalAuditResults([]MedicalAuditResult{trivial, passed, risky})
	want := append(append([]Finding{}, trivial.Findings...), risky.Findings...)
	if got.Passed || !got.SecurityRisk || !reflect.DeepEqual(got.Findings, want) {
		t.Errorf("merged: %+v, want security risk with findings %+v", got, want)
	}
}

func TestMergeGlobalResults(t *testing.T) {
	trivial := MedicalAuditResult{Findings: []Finding{{Factor: "J00", ReasonType: ReasonTrivialCondition}}}
	risky := MedicalAuditResult{Findings: []Finding{{ReasonType: ReasonMedicalResearch}}}

	// Findings follow the check order, not the (unordered) map.
	got, err := mergeGlobalResults([]string{"b", "a"}, map[string]any{"a": trivial, "b": risky})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Findings) != 2 || got.Findings[0].ReasonType != ReasonMedicalResearch || !got.SecurityRisk {
		t.Errorf("merged = %+v, want research finding first and a security risk", got)
	}

	// A check that returned nothing is an error, not a silent pass.
	if _, err := mergeGlobalResults([]string{"a", "b"}, map[string]any{"a": trivial}); err == nil {
		t.Error("missing check result: want error")
	}

	if _, err := mergeGlobalResults([]string{"a"}, map[string]any{"a": "not a result"}); err == nil {
		t.Error("undecodable check result: want error")
	}
}

// globalSubgraphResult runs the global check subgraph as wired in run() - input
// builder, checks, join, merge - with stubbed medical audit and extra checks,
// and returns the merged result.
func globalSubgraphResult(
	t *testing.T,
	input MedicalAuditInput,
	verdict TrivialConditionVerdict,
	extra ...workflow.Node,
) MedicalAuditResult {
	t.Helper()

	cfg := workflow.NodeConfig{}

	buildInput := workflow.NewFunctionNode("medical_audit_input",
		func(_ agent.Context, _ string) (MedicalAuditInput, error) { return input, nil }, cfg)

	// Stands in for the LLM-backed medical audit agent node.
	stubAudit := workflow.NewFunctionNode(medicalAuditAgentName,
		func(_ agent.Context, _ MedicalAuditInput) (TrivialConditionVerdict, error) { return verdict, nil }, cfg)

	checks := append([]workflow.Node{newTrivialCheckNode(stubAudit, cfg)}, extra...)
	join := workflow.NewJoinNode("global_join")
	merge := newMergeGlobalNode(checks, cfg)

	edges := workflow.NewEdgeBuilder()
	edges.Add(workflow.Start, buildInput)
	edges.AddFanOut(buildInput, checks...)
	edges.AddFanIn(join, checks...)
	edges.Add(join, merge)

	wa, err := workflowagent.New(workflowagent.Config{
		Name:  "global_subgraph",
		Edges: edges.Build(),
	})
	if err != nil {
		t.Fatal(err)
	}

	r, err := runner.NewInMemory("test", wa)
	if err != nil {
		t.Fatal(err)
	}

	var result *MedicalAuditResult
	for event, err := range r.Run(
		context.Background(), "user", "session",
		genai.NewContentFromText("hi", genai.RoleUser), agent.RunConfig{},
	) {
		if err != nil {
			t.Fatal(err)
		}

		if event.Output == nil {
			continue
		}
		// The last event with an output is the merge node's.
		out, err := decodeAny[MedicalAuditResult](event.Output)
		if err != nil {
			t.Fatalf("decode output %v: %v", event.Output, err)
		}
		result = &out
	}

	if result == nil {
		t.Fatal("subgraph emitted no output")
	}

	return *result
}

func TestGlobalSubgraphTrivialCheck(t *testing.T) {
	input := MedicalAuditInput{
		MedicalFactors: map[string]string{"J00": "common cold", "E11.9": "type 2 diabetes"},
		History:        []Turn{{Role: "user", Message: "I have a cold and diabetes"}},
	}

	got := globalSubgraphResult(t, input, TrivialConditionVerdict{Factors: []FactorVerdict{
		{Factor: "J00", Trivial: true, Reason: "cold"},
		{Factor: "E11.9", Trivial: false, Reason: "diabetes"},
	}})
	want := []Finding{{Factor: "J00", ReasonType: ReasonTrivialCondition, Reason: "cold"}}
	if got.Passed || got.SecurityRisk || !reflect.DeepEqual(got.Findings, want) {
		t.Errorf("trivial: %+v, want failed with %+v", got, want)
	}

	got = globalSubgraphResult(t, input, TrivialConditionVerdict{Factors: []FactorVerdict{
		{Factor: "J00", Trivial: false},
		{Factor: "E11.9", Trivial: false},
	}})
	if !got.Passed {
		t.Errorf("none trivial: %+v, want passed", got)
	}
}

// A further global check, added by appending to the check list, is joined and
// merged with no change to the join or merge.
func TestGlobalSubgraphExtraCheck(t *testing.T) {
	extra := workflow.NewFunctionNode("check_extra",
		func(_ agent.Context, _ MedicalAuditInput) (MedicalAuditResult, error) {
			return newMedicalAuditResult([]Finding{{ReasonType: ReasonOverwhelmingInfo, Reason: "lab dump"}}), nil
		}, workflow.NodeConfig{})

	got := globalSubgraphResult(t,
		MedicalAuditInput{MedicalFactors: map[string]string{"J00": "common cold"}},
		TrivialConditionVerdict{Factors: []FactorVerdict{{Factor: "J00", Trivial: true, Reason: "cold"}}},
		extra,
	)

	if got.Passed || !got.SecurityRisk || len(got.Findings) != 2 ||
		got.Findings[0].ReasonType != ReasonTrivialCondition || got.Findings[1].ReasonType != ReasonOverwhelmingInfo {
		t.Errorf("merged = %+v, want trivial then overwhelming findings and a security risk", got)
	}
}
