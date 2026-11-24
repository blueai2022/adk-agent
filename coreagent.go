package main

import "context"

type Stage string

const (
	StageInquiry   Stage = "inquiry"
	StageRating    Stage = "rating"
	StageRationale Stage = "rationale"
)

type Turn struct {
	Role    string `json:"role"`
	Message string `json:"message"`
}

type CoreAgentResponse struct {
	SessionID      string                    `json:"session_id"`
	MedicalFactors map[string]string         `json:"medical_factors"` // ICD10 -> Description
	FactorIDs      map[string]string         `json:"factor_ids"`      // ICD10 -> factor ID (e.g. E11.9 -> dm2), the key of FactorStates
	FactorStates   map[string]map[string]any `json:"factor_states"`
	History        []Turn                    `json:"history"`
	UserResponse   string                    `json:"user_response"`
	AgentStage     Stage                     `json:"agent_stage"`
}

func (c *CoreAgentResponse) IsRatingStage() bool {
	return c.AgentStage == StageRating
}

func (c *CoreAgentResponse) IsInquiryStage() bool {
	return c.AgentStage == StageInquiry
}

type coreAgentClient struct {
}

func (c *coreAgentClient) SendTurn(ctx context.Context, sessionID, userInput string) (*CoreAgentResponse, error) {
	return nil, nil
}

func (c *coreAgentClient) ReportAuditFailure(ctx context.Context, sessionID string, failed []AuditResult) error {
	return nil
}

type CoreAgentClient interface {
	SendTurn(ctx context.Context, sessionID, userInput string) (*CoreAgentResponse, error)
	// ReportAuditFailure tells the core agent which factors failed the audit.
	ReportAuditFailure(ctx context.Context, sessionID string, failed []AuditResult) error
}

func NewCoreAgentClient() (CoreAgentClient, error) {
	return nil, nil
}
