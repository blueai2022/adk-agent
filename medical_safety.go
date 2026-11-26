package main

import (
	"errors"
	"fmt"

	"github.com/rs/zerolog/log"
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/session"
)

// medicalSafetyAgentName is the AgentNode name for the medical safety agent.
const medicalSafetyAgentName = "medical_safety_agent"

// mainHistoryKey is the session state key for the raw user messages
// accumulated across turns by the root agent. It is distinct from core:history;
// this is for checks in front of the core agent at turn #1.
const mainHistoryKey = "main:history"

// safetyHistoryLengthThreshold is the cumulative character length of
// main:history above which safetyGateCheck routes to the medical safety sub-agent.
const safetyHistoryLengthThreshold = 700

// routeSafetyCheck and routeContinue label safetyGateCheck's and
// medicalSafetyGate's two outgoing edges.
const (
	routeSafetyCheck = "safety_check"
	routeContinue    = "continue"
)

// medicalSafetyBaseInstruction is the factor-independent part of the medical
// safety agent's instruction. The findings file is appended as the Output section.
const medicalSafetyBaseInstruction = `You are a safety reviewer standing in front of an underwriting core agent, ` +
	`checking the applicant's conversation before the core agent ever processes it. ` +
	`Your input is JSON: the "history" of the chat so far, including the newest message. ` +
	`Read it yourself and judge whether it shows either of the blind spots described below.

`

// MedicalSafetyInput is the input of the medical safety check: the raw user
// messages seen so far, accumulated turn by turn in session state.
type MedicalSafetyInput struct {
	History []Turn `json:"history"`
}

// newMedicalSafetyAgent builds the medical safety LLM agent.
func newMedicalSafetyAgent(llm model.LLM) (agent.Agent, error) {
	findings, err := readArtifact(findingsForms, "medical-safety", "audit-findings")
	if err != nil {
		return nil, fmt.Errorf("failed to read medical safety findings: %w", err)
	}

	return llmagent.New(llmagent.Config{
		Name:        medicalSafetyAgentName,
		Model:       llm,
		Description: "Checks the conversation so far for overwhelming medical information or medical research content, before the core agent ever sees it.",
		Instruction: medicalSafetyBaseInstruction + findings,
	})
}

// appendMainHistory appends to the user messages accumulated so far from session
// state, and returns it.
func appendMainHistory(state session.State, text string) ([]Turn, error) {
	history, err := readState[[]Turn](state, mainHistoryKey)
	if err != nil && !errors.Is(err, session.ErrStateKeyNotExist) {
		return nil, err
	}

	history = append(history, Turn{Role: "user", Message: text})

	if err := state.Set(mainHistoryKey, history); err != nil {
		return nil, err
	}

	return history, nil
}

// historyLength is the cumulative character length of history's messages.
func historyLength(history []Turn) int {
	var length int

	for _, turn := range history {
		length += len(turn.Message)
	}

	return length
}

// safetyRoute decides which route safetyGateCheck takes and what that route's
// successor receives as input.
func safetyRoute(
	history []Turn,
	currentText string,
) (route string, output any) {
	if historyLength(history) < safetyHistoryLengthThreshold {
		return routeContinue, currentText
	}

	return routeSafetyCheck, MedicalSafetyInput{History: history}
}

// safetyGateCheck checks whether the accumulated text is long enough to be worth a
// model call, or straight through to the core agent.
func safetyGateCheck(
	ctx agent.Context,
	text string,
	emit func(*session.Event) error,
) (any, error) {
	history, err := appendMainHistory(ctx.State(), text)
	if err != nil {
		return nil, fmt.Errorf("append safety history: %w", err)
	}

	route, output := safetyRoute(history, text)

	event := session.NewEvent(ctx, ctx.InvocationID())
	event.Routes = []string{route}
	event.Output = output

	if err := emit(event); err != nil {
		return nil, err
	}

	return nil, nil
}

// medicalSafetyGate checks if a security risk was found by previous node;
// ends the turn if detected, or passes the original turn to the core agent.
func medicalSafetyGate(
	ctx agent.Context,
	verdict MedicalSafetyVerdict,
	emit func(*session.Event) error,
) (any, error) {
	if result := verdict.Result(); result.SecurityRisk {
		log.Warn().
			Interface("findings", result.Findings).
			Msg("medical safety check flagged a security risk")

		return securityRiskMessage, nil
	}

	event := session.NewEvent(ctx, ctx.InvocationID())
	event.Routes = []string{routeContinue}
	event.Output = userText(ctx)

	if err := emit(event); err != nil {
		return nil, err
	}

	return nil, nil
}
