package main

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestMedicalSafetyVerdictResult(t *testing.T) {
	clean := MedicalSafetyVerdict{}
	if got := clean.Result(); !got.Passed || got.SecurityRisk || len(got.Findings) != 0 {
		t.Errorf("nothing detected: %+v, want passed with no findings", got)
	}

	overwhelming := MedicalSafetyVerdict{OverwhelmingMedicalInfo: Verdict{Detected: true, Reason: "lab dump"}}
	got := overwhelming.Result()
	want := []Finding{{ReasonType: ReasonOverwhelmingInfo, Reason: "lab dump"}}
	if got.Passed || !got.SecurityRisk || !reflect.DeepEqual(got.Findings, want) {
		t.Errorf("overwhelming: %+v, want security risk with %+v", got, want)
	}

	both := MedicalSafetyVerdict{
		OverwhelmingMedicalInfo: Verdict{Detected: true},
		MedicalResearch:         Verdict{Detected: true},
	}
	if got := both.Result(); got.Passed || !got.SecurityRisk || len(got.Findings) != 2 {
		t.Errorf("both detected: %+v, want a security risk with 2 findings", got)
	}

	for _, f := range both.Result().Findings {
		if f.IsFactorLevel() {
			t.Errorf("safety finding %+v should be global (no factor)", f)
		}
	}
}

func TestTrivialConditionResult(t *testing.T) {
	factors := map[string]string{"J00": "common cold", "E11.9": "type 2 diabetes"}

	// Trivial verdict: a failed, non-security finding carrying the factor.
	got := trivialConditionResult([]FactorVerdict{
		{Factor: "J00", Trivial: true, Reason: "cold"},
		{Factor: "E11.9", Trivial: false, Reason: "diabetes"},
	}, factors)
	want := []Finding{{Factor: "J00", ReasonType: ReasonTrivialCondition, Reason: "cold"}}
	if got.Passed || got.SecurityRisk || !reflect.DeepEqual(got.Findings, want) {
		t.Errorf("trivial: %+v, want failed, no security risk, findings %+v", got, want)
	}

	// All answered, none trivial: passes. Echoed "code: description" and case still match.
	got = trivialConditionResult([]FactorVerdict{
		{Factor: "j00: common cold", Trivial: false},
		{Factor: " E11.9 ", Trivial: false},
	}, factors)
	if !got.Passed || len(got.Findings) != 0 {
		t.Errorf("none trivial: %+v, want passed", got)
	}

	// An answer the model could not attribute to a factor (e.g. "unspecified") is
	// skipped, and the factor it left unanswered is only logged, not a finding.
	got = trivialConditionResult([]FactorVerdict{
		{Factor: "J00", Trivial: false},
		{Factor: "unspecified", Trivial: false},
	}, factors)
	if !got.Passed || got.SecurityRisk || len(got.Findings) != 0 {
		t.Errorf("unspecified: %+v, want passed with no findings", got)
	}

	// A factor simply omitted (no "unspecified" answer) is only logged, not a finding.
	got = trivialConditionResult([]FactorVerdict{
		{Factor: "J00", Trivial: false},
	}, factors)
	if !got.Passed || got.SecurityRisk || len(got.Findings) != 0 {
		t.Errorf("omitted: %+v, want passed with no findings", got)
	}

	// Duplicate answers: a trivial verdict wins.
	got = trivialConditionResult([]FactorVerdict{
		{Factor: "J00", Trivial: false},
		{Factor: "J00", Trivial: true, Reason: "cold"},
		{Factor: "E11.9", Trivial: false},
	}, factors)
	if got.Passed || len(got.Findings) != 1 || got.Findings[0].ReasonType != ReasonTrivialCondition {
		t.Errorf("duplicates: %+v, want one trivial finding", got)
	}

	// No factors detected: nothing to audit.
	if got := trivialConditionResult([]FactorVerdict{}, nil); !got.Passed {
		t.Errorf("no factors: %+v, want passed", got)
	}
}

func TestIsSecurityRisk(t *testing.T) {
	for reason, want := range map[ReasonType]bool{
		ReasonOverwhelmingInfo: true,
		ReasonMedicalResearch:  true,
		ReasonTrivialCondition: false,
		ReasonUnspecified:      false,
		"":                     false,
	} {
		if got := reason.IsSecurityRisk(); got != want {
			t.Errorf("%q.IsSecurityRisk() = %v, want %v", reason, got, want)
		}
	}
}

// The model must only see judgment fields: passed / security_risk are derived in Go,
// so they must appear in neither the verdict schemas nor the instructions.
func TestModelNeverSeesDerivedFields(t *testing.T) {
	for _, v := range []any{MedicalSafetyVerdict{}, TrivialConditionVerdict{}} {
		data, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		for _, field := range []string{"passed", "security_risk", "reason_type", "level"} {
			if strings.Contains(string(data), `"`+field+`"`) {
				t.Errorf("%T exposes %q to the model", v, field)
			}
		}
	}

	for name, findings := range map[string]string{"medical": medicalAuditBaseInstruction, "medical-safety": medicalSafetyBaseInstruction} {
		artifact, err := readArtifact(findingsForms, name, "audit-findings")
		if err != nil {
			t.Fatal(err)
		}
		for _, field := range []string{"passed", "security_risk", "reason_type", "`level`"} {
			if strings.Contains(findings+artifact, field) {
				t.Errorf("%s instruction mentions %q", name, field)
			}
		}
	}
}
