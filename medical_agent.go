package main

import (
	"fmt"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/model"
)

// medicalAuditAgentName is both the agent's Name and, since workflow.AgentNode
// names a node after its wrapped agent, the JoinNode predecessor key gatherAudits
// reads the global medical audit result from.
const medicalAuditAgentName = "medical_audit_agent"

// medicalAuditBaseInstruction is the factor-independent part of the medical audit
// agent's instruction. The findings file (artifacts/medical-audit-findings.md) is
// appended to it as the Output section.
const medicalAuditBaseInstruction = `You are an independent second reader of an underwriting core agent, ` +
	`reviewing the chat with the applicant as a whole rather than any single factor. ` +
	`Your input is JSON: the "medical_factors" (ICD-10 -> description) the core agent detected and ` +
	`the "history" of the chat. Read the chat yourself and judge whether it shows any of the blind ` +
	`spots described below.

`

// MedicalAuditInput is the input of the global medical audit: the medical factors
// the core agent detected and the chat they are audited against.
type MedicalAuditInput struct {
	MedicalFactors map[string]string `json:"medical_factors"`
	History        []Turn            `json:"history"`
}

// medicalAuditInput builds the global medical audit agent's input from the core
// agent's response.
func medicalAuditInput(resp *CoreAgentResponse) MedicalAuditInput {
	if resp == nil {
		return MedicalAuditInput{}
	}

	return MedicalAuditInput{MedicalFactors: resp.MedicalFactors, History: resp.History}
}

// newMedicalAuditAgent builds the global medical audit LLM agent. Unlike the
// per-factor audit_agent, it reviews the whole chat once per turn for blind spots: 
// over-detected trivial conditions with no mortality implications for underwriting.
func newMedicalAuditAgent(llm model.LLM) (agent.Agent, error) {
	findings, err := readArtifact(findingsForms, "medical", "audit-findings")
	if err != nil {
		return nil, fmt.Errorf("failed to read medical audit findings: %w", err)
	}

	return llmagent.New(llmagent.Config{
		Name:        medicalAuditAgentName,
		Model:       llm,
		Description: "Audits the detected medical factors for over-detected trivial conditions with no mortality implications for underwriting.",
		Instruction: medicalAuditBaseInstruction + findings,
	})
}
