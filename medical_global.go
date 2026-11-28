package main

import (
	"fmt"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/session"
	"google.golang.org/adk/v2/workflow"
)

const (
	// Global medical check nodes definitions: each check is a workflow.Node of the same shape
	// (MedicalAuditInput -> MedicalAuditResult). See mergeGlobalResults for results merging.

	// trivialCheckNodeName is the node name and also the results key in mergeGlobalResults.
	trivialCheckNodeName = "check_trivial"
)

// newTrivialCheckNode builds the trivial condition check: it runs auditNode and
// derives the MedicalAuditResult from its per-factor verdicts.
func newTrivialCheckNode(auditNode workflow.Node, cfg workflow.NodeConfig) workflow.Node {
	return workflow.NewDynamicNode( // implicit [MedicalAuditInput, MedicalAuditResult]
		trivialCheckNodeName,
		func(ctx agent.Context, in MedicalAuditInput, _ func(*session.Event) error) (MedicalAuditResult, error) {
			// Agent nodes output JSON-like values, not the typed verdict.
			out, err := workflow.RunNode[any](ctx, auditNode, in)
			if err != nil {
				return MedicalAuditResult{}, err
			}

			verdict, err := decodeAny[TrivialConditionVerdict](out)
			if err != nil {
				return MedicalAuditResult{}, fmt.Errorf("decode trivial condition verdict: %w", err)
			}

			return trivialConditionResult(verdict.AllFactors(), in.MedicalFactors), nil
		},
		cfg,
	)
}

// mergeMedicalAuditResults folds the results of several global checks into one.
func mergeMedicalAuditResults(results []MedicalAuditResult) MedicalAuditResult {
	var findings []Finding
	for _, r := range results {
		findings = append(findings, r.Findings...)
	}

	return newMedicalAuditResult(findings)
}

// mergeGlobalResults decodes each check's result from the join's output (keyed by
// check node name) and merges them in checkNames order.
func mergeGlobalResults(checkNames []string, joined map[string]any) (MedicalAuditResult, error) {
	results := make([]MedicalAuditResult, 0, len(checkNames))

	for _, name := range checkNames {
		raw, ok := joined[name]
		if !ok {
			return MedicalAuditResult{}, fmt.Errorf("global check %q returned no result", name)
		}

		result, err := decodeAny[MedicalAuditResult](raw)
		if err != nil {
			return MedicalAuditResult{}, fmt.Errorf("decode global check %q result: %w", name, err)
		}

		results = append(results, result)
	}

	return mergeMedicalAuditResults(results), nil
}

// newMergeGlobalNode builds the merge node that follows the join of checks. It is
// named medicalAuditResultNodeName, the key auditOutcome reads the global result from.
func newMergeGlobalNode(checks []workflow.Node, cfg workflow.NodeConfig) workflow.Node {
	names := make([]string, 0, len(checks))
	for _, c := range checks {
		names = append(names, c.Name())
	}

	return workflow.NewFunctionNode(
		medicalAuditResultNodeName,
		func(_ agent.Context, joined map[string]any) (MedicalAuditResult, error) {
			return mergeGlobalResults(names, joined)
		},
		cfg,
	)
}
