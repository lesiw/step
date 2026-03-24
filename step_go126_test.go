//go:build go1.26

package step_test

import (
	"errors"
	"testing"

	"lesiw.io/step"
)

func TestStepErrorAsType(t *testing.T) {
	var f failer
	err := step.Do(t.Context(), f.step1)
	if err == nil {
		t.Fatal("expected error")
	}
	stepErr, ok := errors.AsType[*step.Error](err)
	if !ok {
		t.Fatalf("expected *step.Error, got %T", err)
	}
	if stepErr.Name != "step2" {
		t.Errorf("got name %q, want %q", stepErr.Name, "step2")
	}
}
