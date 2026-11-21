// Package main implements an LLM agent that audits ICD-10 medical factors and factor states.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"log"
	"os"
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

type MedicalFactor struct {
	ICD10  string `json:"icd10"`
	Factor string `json:"factor"`
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

					for icd10, factor := range resp.MedicalFactors {
						factor := MedicalFactor{
							ICD10:  icd10,
							Factor: factor,
						}

						jsonData, err := json.Marshal(factor)
						if err != nil {
							yield(nil, err)

							return
						}

						event := session.NewEvent(ic, ic.InvocationID())
						event.Author = "core_underwriting_agent"
						event.Content = genai.NewContentFromText(
							string(jsonData),
							genai.RoleModel,
						)

						ok := yield(event, nil)
						if !ok {
							return
						}
					}

					return
				}

				// For now, always pass through the user response as the agent's reply.
				// TODO: add Language Assistance logic here.

				// Determine the reply message to emit as the agent's conversational output.
				replyText := resp.UserResponse

				event := session.NewEvent(ic, ic.InvocationID())
				event.Author = "core_underwriting_agent"
				event.Content = genai.NewContentFromText(replyText, genai.RoleModel)
				event.Output = resp // Populates ev.Output for downstream workflow nodes

				yield(event, nil)
			}
		},
	})
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
			"needed for auditing, load the audit findings form for the medical factors using ``audit_findings_form`` tool," +
			"output the findings accordingly as JSON",
		Tools: []tool.Tool{
			inputRulesTool,
			AuditFindingsFormTool,
		},
	})
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	// Audit LlmAgent wrapped in NewAgentNode -> MUST be listed in SubAgents
	auditNode, err := workflow.NewAgentNode(auditAgent, cfg)
	if err != nil {
		log.Fatalf("Failed to create audit node: %v", err)
	}

	// Producer node: generates the []string slice to fan out across LLM calls.
	splitTopics := workflow.NewFunctionNode(
		"split_topics",
		func(_ agent.Context, _ string) ([]string, error) {
			return []string{"goroutines", "channels", "sync.WaitGroup"}, nil
		},
		cfg,
	)

	// ParallelWorker: runs auditNodes concurrently across []string -> []string.
	// Cap maxConcurrency at 2 to stay within API rate limits.
	parallelAuditors, err := workflow.NewParallelWorker(
		"parallel_auditors",
		auditNode,
		2, // maxConcurrency
		cfg,
	)
	if err != nil {
		log.Fatalf("Failed to create ParallelWorker: %v", err)
	}

	// Aggregator node: receives []string in the exact order of the input slice.
	aggregate := workflow.NewFunctionNode(
		"aggregate",
		func(ctx agent.Context, summaries []string) (string, error) {
			return "=== Parallel Audit Results ===\n" + strings.Join(summaries, "\n"), nil
		},
		cfg,
	)

	// Root workflowagent
	rootAgent, err := workflowagent.New(workflowagent.Config{
		Name:        "root_underwriting_workflow",
		Description: "Runs external core HTTP agent followed by audit agents.",
		SubAgents:   []agent.Agent{coreSubAgent, auditAgent},
		Edges:       workflow.Chain(workflow.Start, coreAgentNode, splitTopics, parallelAuditors, aggregate),
	})

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
