package main

import (
	"reflect"
	"testing"
)

func TestFactorsToAudit(t *testing.T) {
	resp := &CoreAgentResponse{MedicalFactors: map[string]string{
		"I10":    "hypertension",
		"E11.9":  "type 2 diabetes",
		"E11.65": "type 2 diabetes with hyperglycemia",
		"F17":    "nicotine dependence",
	}}

	// E11.9 collapses into E11.65 (both dm2, first in code order); unresolved codes are kept.
	got := factorsToAudit(resp)
	want := []string{
		"E11.65: type 2 diabetes with hyperglycemia",
		"F17: nicotine dependence",
		"I10: hypertension",
		"age",
		"bmi",
		"tobacco",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("factorsToAudit() = %v, want %v", got, want)
	}

	if got := factorsToAudit(nil); !reflect.DeepEqual(got, defaultFactors) {
		t.Errorf("factorsToAudit(nil) = %v, want %v", got, defaultFactors)
	}
}

func TestFactorsToAuditSkipsCoveredDefault(t *testing.T) {
	icd10Aliases["z68"] = "bmi"
	t.Cleanup(func() { delete(icd10Aliases, "z68") })

	resp := &CoreAgentResponse{MedicalFactors: map[string]string{"Z68.1": "BMI 19.9 or less"}}

	got := factorsToAudit(resp)
	want := []string{"Z68.1: BMI 19.9 or less", "age", "tobacco"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("factorsToAudit() = %v, want %v", got, want)
	}
}
