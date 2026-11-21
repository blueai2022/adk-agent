// Package main implements an LLM agent that audits ICD-10 medical factors and factor states.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"log"
	"os"
	"sort"
	"strings"

	"google.golang.org/genai"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/agent/workflowagent"
	"google.golang.org/adk/v2/cmd/launcher"
	"google.golang.org/adk/v2/cmd/launcher/full"
	"google.golang.org/adk/v2/model/gemini"
	"google.golang.org/adk/v2/session"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/functiontool"
	"google.golang.org/adk/v2/workflow"
)

func extractUserText(ic agent.InvocationContext) string {
	if ic.UserContent() == nil {
		return ""
	}

	for _, part := range ic.UserContent().Parts {
		if part.Text != "" {
			return part.Text
		}
	}

	return ""
}

// routeRating routes the core agent's output to the audit branch of the workflow.
const routeRating = "rating"

// auditFailureMessage is sent to the user when any factor fails its audit.
const auditFailureMessage = "We could not complete the automated review of your application, " +
	"so it is being sent to a human underwriter for review."

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
		Run: func(ic agent.InvocationContext) iter.Seq2[*session.Event, error] {
			return func(yield func(*session.Event, error) bool) {
				userInput := extractUserText(ic)
				if userInput == "" {
					yield(nil, fmt.Errorf("core_underwriting_agent: empty user input"))

					return
				}

				// Build an agent.Context from the InvocationContext
				agentCtx := agent.NewCallbackContext(ic, nil)
				resp, err := client.SendTurn(agentCtx, userInput)
				if err != nil {
					yield(nil, fmt.Errorf("core_underwriting_agent SendTurn failed: %w", err))

					return
				}

				// Persist structured response fields into Session State
				// for downstream audit agents.
				output, err := responseOutput(resp)
				if err != nil {
					yield(nil, err)

					return
				}

				state := ic.Session().State()
				if err := state.Set("core:medical_factors", resp.MedicalFactors); err != nil {
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
					event := session.NewEvent(ic, ic.InvocationID())
					event.Author = "core_underwriting_agent"
					event.Routes = []string{routeRating}
					event.Output = output

					yield(event, nil)

					return
				}

				// For now, always pass through the user response as the agent's reply.
				// TODO: add Language Assistance logic here.

				// Determine the reply message to emit as the agent's conversational output.
				replyText := resp.UserResponse

				event := session.NewEvent(ic, ic.InvocationID())
				event.Author = "core_underwriting_agent"
				event.Content = genai.NewContentFromText(replyText, genai.RoleModel)
				event.Output = output // Populates ev.Output for downstream workflow nodes

				yield(event, nil)
			}
		},
	})
}

// defaultFactors are always audited, in addition to the medical factors found by the core agent.
var defaultFactors = []string{"age", "bmi", "tobacco_use"}

// factorsToAudit returns the core agent's medical factors (sorted by ICD-10 code)
// followed by the default factors, skipping any default already present.
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
			name := resp.MedicalFactors[icd10]
			seen[strings.ToLower(icd10)] = true
			seen[strings.ToLower(name)] = true
			factors = append(factors, fmt.Sprintf("%s: %s", icd10, name))
		}
	}

	for _, f := range defaultFactors {
		if !seen[f] {
			factors = append(factors, f)
		}
	}

	return factors
}

// gatherAudits decides the final reply once all factors are audited.
func gatherAudits(ctx agent.Context, client CoreAgentClient, results []AuditResult) (string, error) {
	var failed []AuditResult
	for _, r := range results {
		if !r.Passed {
			failed = append(failed, r)
		}
	}

	if len(failed) > 0 {
		if err := client.ReportAuditFailure(ctx, failed); err != nil {
			return "", fmt.Errorf("report audit failure to core agent: %w", err)
		}

		return auditFailureMessage, nil
	}

	reply, err := ctx.State().Get("core:user_response")
	if err != nil {
		return "", fmt.Errorf("read core agent response: %w", err)
	}

	text, _ := reply.(string)

	return text, nil
}

func main() {
	ctx := context.Background()

	llm, err := gemini.NewModel(ctx, "gemini-flash-latest", &genai.ClientConfig{
		APIKey: os.Getenv("GOOGLE_API_KEY"),
	})
	if err != nil {
		log.Fatalf("Failed to create model: %v", err)
	}

	cfg := workflow.NodeConfig{}

	coreClient, err := NewCoreAgentClient()
	if err != nil {
		log.Fatalf("Failed to create core client: %v", err)
	}

	coreSubAgent, err := NewCoreSubAgent(coreClient)
	if err != nil {
		log.Fatalf("Failed to create core sub-agent: %v", err)
	}
	coreAgentNode, err := workflow.NewAgentNode(coreSubAgent, cfg)
	if err != nil {
		log.Fatalf("Failed to create core agent node: %v", err)
	}

	inputRulesTool, err := functiontool.New(functiontool.Config{
		Name:        "input_rules",
		Description: "Extracts medical factor-specific input rules.",
	}, inputRules)
	if err != nil {
		log.Fatalf("Failed to create input_rules tool: %v", err)
	}

	AuditFindingsFormTool, err := functiontool.New(functiontool.Config{
		Name:        "audit_findings_form",
		Description: "Loads the audit findings form for medical factors.",
	}, auditFindingsForm)
	if err != nil {
		log.Fatalf("Failed to create audit_findings_form tool: %v", err)
	}

	auditAgent, err := llmagent.New(llmagent.Config{
		Name:        "audit_agent",
		Model:       llm,
		Description: "Audits input interpretation of user input and ingestion into state.",
		Instruction: "retrieve medical factor-specific input rules from ``input_rules`` tool to prepare information " +
			"needed for auditing, load the audit findings form for the medical factors using ``audit_findings_form`` tool, " +
			"fill out the form, and report whether the factor passed the audit with the reason and the filled-out form",
		Tools: []tool.Tool{
			inputRulesTool,
			AuditFindingsFormTool,
		},
	})
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	// Producer node: combines the medical factors found by the core agent with the
	// default factors, and fans the result out across LLM calls.
	splitFactors := workflow.NewFunctionNode(
		"split_factors",
		func(_ agent.Context, resp *CoreAgentResponse) ([]string, error) {
			return factorsToAudit(resp), nil
		},
		cfg,
	)

	// Audit LlmAgent wrapped in NewAgentNode -> MUST be listed in SubAgents
	auditNode, err := workflow.NewAgentNodeTyped[string, AuditResult](auditAgent, cfg)
	if err != nil {
		log.Fatalf("Failed to create audit node: %v", err)
	}

	const maxConcurrency = 2

	// ParallelWorker: runs auditNodes concurrently across []string -> []string.
	// Cap maxConcurrency at 2 to stay within API rate limits.
	parallelAuditors, err := workflow.NewParallelWorker(
		"parallel_auditors",
		auditNode,
		maxConcurrency,
		cfg,
	)
	if err != nil {
		log.Fatalf("Failed to create ParallelWorker: %v", err)
	}

	// Gather node: passes the core agent's original reply to the user if every factor
	// passed the audit; otherwise reports the failure to the core agent and the user.
	gather := workflow.NewEmittingFunctionNode(
		"gather_audits",
		func(ctx agent.Context, results []AuditResult, _ func(*session.Event) error) (string, error) {
			return gatherAudits(ctx, coreClient, results)
		},
		cfg,
	)

	// Root workflowagent
	rootAgent, err := workflowagent.New(workflowagent.Config{
		Name:        "root_underwriting_workflow",
		Description: "Runs external core HTTP agent followed by audit agents.",
		SubAgents:   []agent.Agent{coreSubAgent, auditAgent},
		// Only the rating stage is audited; other stages are already answered by the core agent.
		Edges: workflow.Concat(
			workflow.Chain(workflow.Start, coreAgentNode),
			[]workflow.Edge{{From: coreAgentNode, To: splitFactors, Route: workflow.StringRoute(routeRating)}},
			workflow.Chain(splitFactors, parallelAuditors, gather),
		),
	})
	if err != nil {
		log.Fatalf("Failed to create root agent: %v", err)
	}

	// Save message content in OpenTelemetry logs
	if err := os.Setenv("OTEL_INSTRUMENTATION_GENAI_CAPTURE_MESSAGE_CONTENT", "true"); err != nil {
		log.Fatalf("Failed to set env var: %v", err)
	}

	config := &launcher.Config{
		AgentLoader: agent.NewSingleLoader(rootAgent),
	}

	launcher := full.NewLauncher()
	if err = launcher.Execute(ctx, config, os.Args[1:]); err != nil {
		log.Fatalf("Run failed: %v\n\n%s", err, launcher.CommandLineSyntax())
	}
}
