package main

type ReasonType string

const (
	// Rule-based for icd10 prefix marked with "complication_distinctive"
	ReasonComplicationUnaccounted ReasonType = "complication_unaccounted"

	// Under detection - marked with "medication_indicative" (e.g. meds pinpoint
	// one well-known condition)
	ReasonMedicationIgnored ReasonType = "indicative_medication_ignored"

	// Under detection - marked with "symptom_indicative" (brief symptoms called out)
	ReasonSymptomIgnored ReasonType = "indicative_symptom_ignored"

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
