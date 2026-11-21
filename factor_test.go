package main

import (
	"reflect"
	"testing"
)

func TestFactorsToAudit(t *testing.T) {
	resp := &CoreAgentResponse{MedicalFactors: map[string]string{
		"I10":   "hypertension",
		"E11.9": "type 2 diabetes",
		"F17":   "BMI",
	}}

	got := factorsToAudit(resp)
	want := []string{
		"E11.9: type 2 diabetes",
		"F17: BMI",
		"I10: hypertension",
		"age",
		"tobacco_use",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("factorsToAudit() = %v, want %v", got, want)
	}

	if got := factorsToAudit(nil); !reflect.DeepEqual(got, defaultFactors) {
		t.Errorf("factorsToAudit(nil) = %v, want %v", got, defaultFactors)
	}
}
