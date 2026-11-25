// Package main implements an LLM agent that audits ICD-10 medical factors and factor states.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"

	"os"
	"sort"
	"strings"

	"google.golang.org/genai"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/agent/workflowagent"
	"google.golang.org/adk/v2/cmd/launcher"
	"google.golang.org/adk/v2/cmd/launcher/full"
	"google.golang.org/adk/v2/model/openaimodel"
	"google.golang.org/adk/v2/session"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/functiontool"
	"google.golang.org/adk/v2/workflow"

	"github.com/rs/zerolog/log"
)

func userText(ctx agent.InvocationContext) string {
	return contentText(ctx.UserContent())
}

// contentText concatenates the text parts of content.
func contentText(content *genai.Content) string {
	if content == nil {
		return ""
	}

	var text strings.Builder

	for _, part := range content.Parts {
		if part != nil && part.Text != "" {
			// No separator - follows what ADK does internally
			text.WriteString(part.Text)
		}
	}

	return text.String()
}

// routeRating routes the core agent's output to the audit branch of the workflow.
const routeRating = "rating"

// auditFailureMessage is sent to the user when any factor, or a non-security-risk
// global check, fails its audit.
const auditFailureMessage = "We could not complete the automated review of your application, " +
	"so it is being sent to a human underwriter for review."

// securityRiskMessage is sent to the user instead of auditFailureMessage when the
// global medical audit flags a security risk (overwhelming medical info or
// medical research).
const securityRiskMessage = "The agent has detected an anomaly which triggered a security alert " +
	"to protect itself. The chat session is now under human review. We apologize for the inconvenience."

// parallelAuditorsNodeName is the ParallelWorker's node name, and so the JoinNode
// predecessor key gatherAudits reads the per-factor audit results from.
const parallelAuditorsNodeName = "parallel_auditors"

// responseOutput converts the response into a JSON-like map. Agent nodes validate
// their output against a schema, which cannot be done against a struct.
func responseOutput(resp *CoreAgentResponse) (map[string]any, error) {
	data, err := json.Marshal(resp)
	if err != nil {
		return nil, err
	}

	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}

	return out, nil
}

// NewCoreSubAgent wraps CoreAgentClient as a first-class ADK v2 agent.Agent.
func NewCoreSubAgent(client CoreAgentClient) (agent.Agent, error) {
	return agent.New(agent.Config{
		Name:        "core_underwriting_agent",
		Description: "External underwriting LLM agent that extracts ICD-10 medical factors and factor states.",
		Run: func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
			return func(yield func(*session.Event, error) bool) {
				userInput := userText(ctx)
				if userInput == "" {
					yield(nil, fmt.Errorf("core_underwriting_agent: empty user input"))

					return
				}

				state := ctx.Session().State()

				// The core agent mints its session ID on the first turn, so none is stored yet.
				sessionID, err := coreSessionID(state)
				if err != nil && !errors.Is(err, session.ErrStateKeyNotExist) {
					yield(nil, err)

					return
				}

				// Build an agent.Context from the InvocationContext
				agentCtx := agent.NewCallbackContext(ctx, nil)
				resp, err := client.SendTurn(agentCtx, sessionID, userInput)
				if err != nil {
					yield(nil, fmt.Errorf("core_underwriting_agent SendTurn failed: %w", err))

					return
				}
				if resp == nil || resp.SessionID == "" {
					yield(nil, fmt.Errorf("core_underwriting_agent: core agent returned no session ID"))

					return
				}

				// Persist structured response fields into Session State
				// for downstream audit agents.
				output, err := responseOutput(resp)
				if err != nil {
					yield(nil, err)

					return
				}

				if err := state.Set(coreSessionIDKey, resp.SessionID); err != nil {
					yield(nil, err)

					return
				}
				if err := state.Set("core:medical_factors", resp.MedicalFactors); err != nil {
					yield(nil, err)

					return
				}
				if err := state.Set("core:factor_ids", resp.FactorIDs); err != nil {
					yield(nil, err)

					return
				}
				if err := state.Set("core:factor_states", resp.FactorStates); err != nil {
					yield(nil, err)

					return
				}
				if err := state.Set("core:history", resp.History); err != nil {
					yield(nil, err)

					return
				}

				if resp.IsRatingStage() {
					if err := state.Set("core:agent_stage", resp.AgentStage); err != nil {
						yield(nil, err)

						return
					}

					// Held back until the audit passes; the gather node reads it from state.
					if err := state.Set("core:user_response", resp.UserResponse); err != nil {
						yield(nil, err)

						return
					}

					// No Content: the user must not see the rating before it is audited.
					// The route sends the response on to the audit branch.
					event := session.NewEvent(ctx, ctx.InvocationID())
					event.Author = "core_underwriting_agent"
					event.Routes = []string{routeRating}
					event.Output = output

					yield(event, nil)

					return
				}

				// For now, always pass through the core agent's user response as the
				// root agent's reply.
				// TODO: add Language Assistance logic here.

				// Determine the reply message to emit as the agent's conversational output.
				replyText := resp.UserResponse

				event := session.NewEvent(ctx, ctx.InvocationID())
				event.Author = "core_underwriting_agent"
				event.Content = genai.NewContentFromText(replyText, genai.RoleModel)
				event.Output = output // Populates for downstream workflow nodes

				yield(event, nil)
			}
		},
	})
}

// defaultFactors are always audited, in addition to the medical factors found by the core agent.
var defaultFactors = []string{"age", "bmi", "tobacco"}

// factorsToAudit returns the core agent's medical factors (sorted by ICD-10 code)
// followed by the default factors. A factor is audited once: of several medical
// factors resolving to the same factor ID (e.g. E11.9 and E11.65), only the first
// is kept, and a default already covered by a medical factor is skipped.
// Medical factors without a factor ID from the core agent are kept as they are.
func factorsToAudit(resp *CoreAgentResponse) []string {
	var factors []string
	seen := make(map[string]bool)

	if resp != nil {
		icd10s := make([]string, 0, len(resp.MedicalFactors))
		for icd10 := range resp.MedicalFactors {
			icd10s = append(icd10s, icd10)
		}
		sort.Strings(icd10s)

		for _, icd10 := range icd10s {
			if id, ok := resolveFactorKey(icd10, resp.FactorIDs); ok {
				if seen[id] {
					continue
				}
				seen[id] = true
			}

			factors = append(factors, fmt.Sprintf("%s: %s", icd10, resp.MedicalFactors[icd10]))
		}
	}

	for _, f := range defaultFactors {
		if !seen[f] {
			factors = append(factors, f)
		}
	}

	return factors
}

// auditInputs pairs each factor to audit with the chat history it is audited against.
func auditInputs(resp *CoreAgentResponse) []AuditInput {
	var history []Turn
	if resp != nil {
		history = resp.History
	}

	factors := factorsToAudit(resp)
	inputs := make([]AuditInput, 0, len(factors))
	for _, factor := range factors {
		inputs = append(inputs, AuditInput{Factor: factor, History: history})
	}

	return inputs
}

// coreSessionIDKey is the session state key of the core agent's own session ID.
const coreSessionIDKey = "core:session_id"

// coreSessionID reads the core agent's session ID from state. It returns
// session.ErrStateKeyNotExist (wrapped) before the core agent has replied.
func coreSessionID(state session.ReadonlyState) (string, error) {
	id, err := readState[string](state, coreSessionIDKey)
	if err != nil {
		return "", err
	}

	return id, nil
}

// auditOutcome decodes the JoinNode's output - a map keyed by predecessor node
// name, holding the []AuditResult from parallelAuditorsNodeName and the
// MedicalAuditResult from medicalAuditAgentName - into the failure to report and
// the message to send the user. A nil failure means every factor and the global
// medical audit passed, and the caller should reply with the core agent's own
// response instead of message.
func auditOutcome(joined map[string]any) (failure *AuditFailure, message string, err error) {
	results, err := decodeAny[[]AuditResult](joined[parallelAuditorsNodeName])
	if err != nil {
		return nil, "", fmt.Errorf("decode factor audit results: %w", err)
	}

	global, err := decodeAny[MedicalAuditResult](joined[medicalAuditAgentName])
	if err != nil {
		return nil, "", fmt.Errorf("decode medical audit result: %w", err)
	}

	var failedFactors []AuditResult
	for _, r := range results {
		if !r.Passed {
			failedFactors = append(failedFactors, r)
		}
	}

	if len(failedFactors) == 0 && global.Passed {
		return nil, "", nil
	}

	failure = &AuditFailure{FactorResults: failedFactors}
	if !global.Passed {
		failure.Global = &global
	}

	message = auditFailureMessage
	if global.SecurityRisk {
		message = securityRiskMessage
	}

	return failure, message, nil
}

// gatherAudits decides the final reply once both the per-factor audits and the
// global medical audit have completed. See auditOutcome for how joined is read.
func gatherAudits(ctx agent.Context, client CoreAgentClient, joined map[string]any) (string, error) {
	failure, message, err := auditOutcome(joined)
	if err != nil {
		return "", err
	}

	if failure == nil {
		reply, err := ctx.State().Get("core:user_response")
		if err != nil {
			return "", fmt.Errorf("read core agent response: %w", err)
		}

		if text, ok := reply.(string); ok {
			return text, nil
		}

		return "", fmt.Errorf("core agent response is not a string")
	}

	// Audits only run after the core agent has replied, so its session ID must be stored.
	sessionID, err := coreSessionID(ctx.State())
	if err != nil {
		return "", fmt.Errorf("report audit failure to core agent: %w", err)
	}

	if err := client.ReportAuditFailure(ctx, sessionID, *failure); err != nil {
		return "", fmt.Errorf("report audit failure to core agent: %w", err)
	}

	return message, nil
}

func main() {
	if err := run(); err != nil {
		log.Fatal().
			Err(err).
			Msg("fatal error")
	}
}

func run() error {
	ctx := context.Background()

	flags, err := NewFlags(os.Args)
	if err != nil {
		return fmt.Errorf("failed to initialize flags: %v", err)
	}

	llm, err := openaimodel.NewModel(ctx, flags.Model, &openaimodel.ClientConfig{
		APIKey:  flags.APIKey,
		BaseURL: flags.BaseURL,
	})
	if err != nil {
		return fmt.Errorf("failed to create model: %w", err)
	}

	cfg := workflow.NodeConfig{}

	coreClient, err := NewCoreAgentClient()
	if err != nil {
		return fmt.Errorf("failed to create core client: %v", err)
	}

	coreSubAgent, err := NewCoreSubAgent(coreClient)
	if err != nil {
		return fmt.Errorf("failed to create core sub-agent: %v", err)
	}
	coreAgentNode, err := workflow.NewAgentNode(coreSubAgent, cfg)
	if err != nil {
		return fmt.Errorf("failed to create core agent node: %v", err)
	}

	inputRulesTool, err := functiontool.New(functiontool.Config{
		Name:        "input_rules",
		Description: "Extracts medical factor-specific input rules.",
	}, inputRules)
	if err != nil {
		return fmt.Errorf("failed to create input_rules tool: %v", err)
	}

	factorStateTool, err := functiontool.New(functiontool.Config{
		Name:        "factor_state",
		Description: "Returns the state the core agent recorded for a medical factor.",
	}, newFactorStateTool(sessionFactorState))
	if err != nil {
		return fmt.Errorf("failed to create factor_state tool: %v", err)
	}

	auditAgent, err := llmagent.New(llmagent.Config{
		Name:        "audit_agent",
		Model:       llm,
		Description: "Audits input interpretation of user input and ingestion into state.",
		// Base instruction plus the factor's findings file as the Output section.
		InstructionProvider: auditInstruction,
		Tools: []tool.Tool{
			inputRulesTool,
			factorStateTool,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create agent: %v", err)
	}

	// Producer node: combines the medical factors found by the core agent with the
	// default factors, and fans the result out across LLM calls.
	splitFactors := workflow.NewFunctionNode(
		"split_factors",
		func(_ agent.Context, resp *CoreAgentResponse) ([]AuditInput, error) {
			return auditInputs(resp), nil
		},
		cfg,
	)

	// Audit LlmAgent wrapped in NewAgentNode -> MUST be listed in SubAgents
	auditNode, err := workflow.NewAgentNodeTyped[AuditInput, AuditResult](auditAgent, cfg)
	if err != nil {
		return fmt.Errorf("failed to create audit node: %v", err)
	}

	const maxConcurrency = 2

	// ParallelWorker: runs auditNodes concurrently across []AuditInput -> []AuditResult.
	// Cap maxConcurrency at 2 to stay within API rate limits.
	parallelAuditors, err := workflow.NewParallelWorker(
		parallelAuditorsNodeName,
		auditNode,
		maxConcurrency,
		cfg,
	)
	if err != nil {
		return fmt.Errorf("failed to create ParallelWorker: %v", err)
	}

	medicalAuditAgent, err := newMedicalAuditAgent(llm)
	if err != nil {
		return fmt.Errorf("failed to create medical audit agent: %v", err)
	}

	// Producer node: extracts the global medical audit agent's input (the detected
	// medical factors and the chat) from the core agent's response.
	buildMedicalAuditInput := workflow.NewFunctionNode(
		"medical_audit_input",
		func(_ agent.Context, resp *CoreAgentResponse) (MedicalAuditInput, error) {
			return medicalAuditInput(resp), nil
		},
		cfg,
	)

	medicalAuditNode, err := workflow.NewAgentNodeTyped[MedicalAuditInput, MedicalAuditResult](medicalAuditAgent, cfg)
	if err != nil {
		return fmt.Errorf("failed to create medical audit node: %v", err)
	}

	// Fan-in barrier: waits for both the per-factor audits and the global medical
	// audit before gatherAudits decides the final reply.
	auditJoin := workflow.NewJoinNode("audit_join")

	// Gather node: passes the core agent's original reply to the user if every factor
	// and the global medical audit passed; otherwise reports the failure to the core
	// agent and the user.
	gatherAuditsNode := workflow.NewEmittingFunctionNode(
		"gather_audits",
		func(ctx agent.Context, joined map[string]any, _ func(*session.Event) error) (string, error) {
			return gatherAudits(ctx, coreClient, joined)
		},
		cfg,
	)

	// Root workflowagent. Graph wiring (only the rating stage is audited; other
	// stages are already answered by coreAgentNode and never reach this fan-out):
	//
	//   START ─> coreAgentNode ──rating─┬─> splitFactors ──────────> parallelAuditors ──┐
	//                                   └─> buildMedicalAuditInput ─> medicalAuditNode ──┼─> auditJoin ─> gatherAuditsNode
	edgeBuilder := workflow.NewEdgeBuilder()
	edgeBuilder.Add(workflow.Start, coreAgentNode)
	edgeBuilder.AddRoute(
		coreAgentNode,
		splitFactors,
		workflow.StringRoute(routeRating),
	)
	edgeBuilder.AddRoute(
		coreAgentNode,
		buildMedicalAuditInput,
		workflow.StringRoute(routeRating),
	)
	edgeBuilder.Add(splitFactors, parallelAuditors)
	edgeBuilder.Add(buildMedicalAuditInput, medicalAuditNode)
	edgeBuilder.AddFanIn(auditJoin, parallelAuditors, medicalAuditNode)
	edgeBuilder.Add(auditJoin, gatherAuditsNode)

	rootAgent, err := workflowagent.New(workflowagent.Config{
		Name:        "root_underwriting_workflow",
		Description: "Runs external core HTTP agent followed by audit agents.",
		SubAgents:   []agent.Agent{coreSubAgent, auditAgent, medicalAuditAgent},
		Edges:       edgeBuilder.Build(),
	})
	if err != nil {
		return fmt.Errorf("failed to create root agent: %v", err)
	}

	// Save message content in OpenTelemetry logs
	if err := os.Setenv("OTEL_INSTRUMENTATION_GENAI_CAPTURE_MESSAGE_CONTENT", "true"); err != nil {
		return fmt.Errorf("failed to set env var: %v", err)
	}

	config := &launcher.Config{
		AgentLoader: agent.NewSingleLoader(rootAgent),
	}

	agtLauncher := full.NewLauncher()
	if err = agtLauncher.Execute(ctx, config, os.Args[1:]); err != nil {
		return fmt.Errorf("run failed: %v\n\n%s", err, agtLauncher.CommandLineSyntax())
	}

	return nil
}
