package main

import (
	"errors"
	"iter"
	"reflect"
	"strings"
	"testing"

	"google.golang.org/adk/v2/session"
	"google.golang.org/adk/v2/workflow"
)

// fakeWritableState is a session.State backed by a map, for tests that need to
// observe Set in addition to Get (unlike audit_test.go's read-only fakeState).
type fakeWritableState map[string]any

func (s fakeWritableState) Get(key string) (any, error) {
	v, ok := s[key]
	if !ok {
		return nil, session.ErrStateKeyNotExist
	}

	return v, nil
}

func (s fakeWritableState) Set(key string, value any) error {
	s[key] = value

	return nil
}

func (s fakeWritableState) All() iter.Seq2[string, any] {
	return func(yield func(string, any) bool) {
		for k, v := range s {
			if !yield(k, v) {
				return
			}
		}
	}
}

func TestHistoryLength(t *testing.T) {
	history := []Turn{{Role: "user", Message: "hi"}, {Role: "user", Message: "there"}}
	if got, want := historyLength(history), len("hi")+len("there"); got != want {
		t.Errorf("historyLength() = %d, want %d", got, want)
	}
	if got := historyLength(nil); got != 0 {
		t.Errorf("historyLength(nil) = %d, want 0", got)
	}
}

func TestAppendMainHistory(t *testing.T) {
	state := fakeWritableState{}

	// First turn: no prior key, starts from an empty history.
	history, err := appendMainHistory(state, "first message")
	if err != nil {
		t.Fatal(err)
	}
	want := []Turn{{Role: "user", Message: "first message"}}
	if !reflect.DeepEqual(history, want) {
		t.Errorf("first append = %+v, want %+v", history, want)
	}

	// Second turn: appends to, rather than replaces, the stored history.
	history, err = appendMainHistory(state, "second message")
	if err != nil {
		t.Fatal(err)
	}
	want = append(want, Turn{Role: "user", Message: "second message"})
	if !reflect.DeepEqual(history, want) {
		t.Errorf("second append = %+v, want %+v", history, want)
	}

	stored, err := state.Get(mainHistoryKey)
	if err != nil || !reflect.DeepEqual(stored, want) {
		t.Errorf("stored state = %+v, %v; want %+v", stored, err, want)
	}
}

func TestAppendMainHistoryPropagatesOtherErrors(t *testing.T) {
	// A key present but of the wrong shape is a real error, not an empty history.
	state := fakeWritableState{mainHistoryKey: 42}
	if _, err := appendMainHistory(state, "x"); err == nil || errors.Is(err, session.ErrStateKeyNotExist) {
		t.Errorf("malformed history error = %v, want a non-ErrStateKeyNotExist error", err)
	}
}

func TestSafetyRoute(t *testing.T) {
	short := []Turn{{Role: "user", Message: strings.Repeat("x", safetyHistoryLengthThreshold-1)}}
	route, output := safetyRoute(short, "current")
	if route != routeContinue || output != "current" {
		t.Errorf("below threshold: route=%q output=%v, want %q, \"current\"", route, output, routeContinue)
	}

	long := []Turn{{Role: "user", Message: strings.Repeat("x", safetyHistoryLengthThreshold)}}
	route, output = safetyRoute(long, "current")
	want := MedicalSafetyInput{History: long}
	if route != routeSafetyCheck || !reflect.DeepEqual(output, want) {
		t.Errorf("at threshold: route=%q output=%+v, want %q, %+v", route, output, routeSafetyCheck, want)
	}
}

func TestNewMedicalSafetyAgentWiring(t *testing.T) {
	a, err := newMedicalSafetyAgent(nil)
	if err != nil {
		t.Fatal(err)
	}
	if a.Name() != medicalSafetyAgentName {
		t.Errorf("agent name = %q, want %q", a.Name(), medicalSafetyAgentName)
	}

	if _, err := workflow.NewAgentNodeTyped[MedicalSafetyInput, MedicalAuditResult](a, workflow.NodeConfig{}); err != nil {
		t.Fatal(err)
	}
}

func TestMedicalSafetyFindingsArtifact(t *testing.T) {
	findings, err := readArtifact(findingsForms, "medical-safety", "audit-findings")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"overwhelming_medical_info", "medical_research"} {
		if !strings.Contains(findings, want) {
			t.Errorf("medical safety findings missing %q", want)
		}
	}
	if strings.Contains(findings, "trivial_condition") {
		t.Error("medical safety findings should not cover \"trivial_condition\" (medical_audit_agent's job)")
	}
}
