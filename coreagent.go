package main

import "context"

type Stage string

const (
	StageInquiry   Stage = "inquiry"
	StageRating    Stage = "rating"
	StageRationale Stage = "rationale"
)

type Turn struct {
	Role    string
	Message string
}

type CoreAgentResponse struct {
	MedicalFactors map[string]string // ICD10 -> Description
	FactorIDs      map[string]string // ICD10 -> factor ID (e.g. E11.9 -> dm2), the key of FactorStates
	FactorStates   map[string]map[string]any
	History        []Turn
	UserResponse   string
	AgentStage     Stage
}

func (c *CoreAgentResponse) IsRatingStage() bool {
	return c.AgentStage == StageRating
}

func (c *CoreAgentResponse) IsInquiryStage() bool {
	return c.AgentStage == StageInquiry
}

type coreAgentClient struct {
}

func (c *coreAgentClient) SendTurn(ctx context.Context, userInput string) (*CoreAgentResponse, error) {
	return nil, nil
}

func (c *coreAgentClient) ReportAuditFailure(ctx context.Context, failed []AuditResult) error {
	return nil
}

type CoreAgentClient interface {
	SendTurn(ctx context.Context, userInput string) (*CoreAgentResponse, error)
	// ReportAuditFailure tells the core agent which factors failed the audit.
	ReportAuditFailure(ctx context.Context, failed []AuditResult) error
}

func NewCoreAgentClient() (CoreAgentClient, error) {
	return nil, nil
}
