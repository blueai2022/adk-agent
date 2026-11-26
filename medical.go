package main

type ReasonType string

const (
	// Rule-based for icd10 prefix marked with "complication_distinctive"
	// Deferred: there is no such use cases in currently supported UW rules.
	ReasonComplicationUnaccounted ReasonType = "complication_unaccounted"

	// Core agent vs. audit agent's overrule for "unmapped signals": core agent has specific
	// rules for whether it should automatically map medications or symptoms to specific
	// conditions and it is quite accurate in doing so.
	// Our audit agent will Not be reapplying the same rules; or override the rating of the
	// core agent. It only provides a system-level override for requiring human review or
	// follow-up questions. It's a system-level override chosen by most insurance underwriters.
	// Note: this framing needs to be clear in the system prompt of the llm agent.
	//
	// Audit agent's process:
	// 1. Google-searches on mentioned medication uses (including handling minor typos)
	// 2. Maps out treatment relationship between medications and disclosed medical conditions
	// 3. Uses the condition list from returned ICD10 codes from core agent as it reflects the
	// handling of unmapped signals by the core agent.
	// 4. Compile unexplained/unmapped medications, i.e. taking medication without a corresponding
	// diagnosis or clear cause by the user. Note: this excludes medications that are
	// known to treat common trivial conditions.
	// 5. Note if any questions about the unexplained medications have been asked.
	// 6. Agent then adds a turn to ask follow-up questions for All the unexplained medications, excluding
	// any medications for which follow-up had already been asked in the full chat history, by the
	// audit agent or the core agent. Marking a flag indicates that follow-up has been asked for those medications.
	// 7. The audit step is happening just before core agent's final decision. So adding a turn, discarding the
	// the core agent's current message is fine. Simply forwarding the user's response in the next turn,
	// is incorporated naturally into the ongoing conversation.
	// 8. If unanswered after follow-up questions marked as asked in session, refer to human underwriter.
	//
	// Scope - global medical audit since it's not per-factor
	// Knowledge help from agent Tool - Browser Search Tool.
	ReasonMedicationWithoutDx ReasonType = "unexplained_medication"

	// Core agent vs. audit agent's overrule for "unmapped signals": core agent has specific
	// rules for whether it should automatically map medications or symptoms to specific
	// conditions and it is quite accurate in doing so.
	// Our audit agent will Not be reapplying the same rules; or override the rating of the
	// core agent. It only provides a system-level override for requiring human review or
	// follow-up questions. It's a system-level override chosen by most insurance underwriters.
	// Note: this framing needs to be clear in the system prompt of the llm agent.
	//
	// For Severe Symptoms Without Diagnosis or Disclosure, only 2 conditions, Cancer or AIDS,
	// apply here, Resolution: refer to human underwriter.
	//
	// Scope: global medical audit since it's not per-factor.
	// Knowledge help: perhaps with brief symptoms in System Prompt.
	// Caveat: Only if the severe symptoms remain unexplained in the full chat history.
	ReasonSevereSymptomsWithoutDx ReasonType = "unmapped_severe_symptom"

	// Over detection - Trivial conditions overdetected by the core agent
	// e.g. common code/flu, seasonal allergy, acne, athelete foot, minor injuries
	// which do not have mortality implications within life insurance underwriting.
	ReasonTrivialCondition ReasonType = "trivial_condition"

	// User provides overwhelming medical information or biomed research to prevent
	// the agent from processing effectively.
	//
	// Scenarios require the case to be safety flagged: Extensive medical
	// records or Scientific literature or papers
	//
	// - Extensive medical records provided by the user, including extensive lab results,
	// medical treatment details by dates, or imaging or EKH results with detailed
	// narratives or descriptions in multiple paragraphs. Normal EKG or imaging summary
	// should not be considered overwhelming.
	//
	// This is abnormal for content from agent or underwriting workbench toool,
	// which tends to be summaries.
	ReasonOverwhelmingInfo ReasonType = "overwhelming_medical_info"

	// - Scientific literature such as Biomedical research articles, or papers;
	ReasonMedicalResearch ReasonType = "medical_research"
)

type FindingLevel string

const (
	FactorLevel FindingLevel = "factor"
	GlobalLevel FindingLevel = "global"
)

type Finding struct {
	Level      FindingLevel `json:"level"`
	Factor     string       `json:"factor"` // expects value if finding is factor level
	ReasonType ReasonType   `json:"reason_type"`
	Reason     string       `json:"reason"`
}

// Medical audit covers blind spots in diagnosis determination by the core agent:
// such as "overwhelming medical information" (including extensive labs, medical
// treatment details), medical research journal entries in the input provided by
// the user, or trivial conditions the core agent over-detected as medical factors.
//
// MedicalAuditResult is the outcome of the global medical audit, run once per
// chat rather than once per factor.
type MedicalAuditResult struct {
	Passed       bool      `json:"passed"`        // true when none of the blind spots below apply
	SecurityRisk bool      `json:"security_risk"` // true for overwhelming medical info or medical research; never for trivial_condition alone
	Findings     []Finding `json:"findings"`      // one entry per blind spot found
}
