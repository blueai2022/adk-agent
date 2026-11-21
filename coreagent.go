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

type CoreAgentClient interface {
	SendTurn(ctx context.Context, userInput string) (*CoreAgentResponse, error)
}

func NewCoreAgentClient() (CoreAgentClient, error) {
	return nil, nil
}
