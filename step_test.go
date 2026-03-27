package step_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"lesiw.io/step"
)

type seq struct{ path string }

func (s *seq) start(context.Context) (step.Func[seq], error) {
	switch s.path {
	case "a":
		return s.stepA, nil
	case "b":
		return s.stepB, nil
	}
	return nil, nil
}

func (s *seq) stepA(context.Context) (step.Func[seq], error) {
	return s.end, nil
}

func (s *seq) stepB(context.Context) (step.Func[seq], error) {
	return s.end, nil
}

func (s *seq) end(context.Context) (step.Func[seq], error) {
	return nil, nil
}

func TestTransition(t *testing.T) {
	s := &seq{path: "a"}
	got, err := s.start(t.Context())
	if err != nil {
		t.Fatalf("start err: %v", err)
	}
	if want := s.stepA; !step.Equal(got, want) {
		t.Errorf(
			"got %s, want %s",
			step.Name(got), step.Name(want),
		)
	}
}

func TestTransitionNil(t *testing.T) {
	s := &seq{path: "x"}
	got, err := s.start(t.Context())
	if err != nil {
		t.Fatalf("start err: %v", err)
	}
	if got != nil {
		t.Errorf("got %s, want nil", step.Name(got))
	}
}

func TestNameMethod(t *testing.T) {
	var s seq
	tests := map[string]step.Func[seq]{
		"start": s.start,
		"stepA": s.stepA,
		"stepB": s.stepB,
		"end":   s.end,
	}
	for want, fn := range tests {
		if got := step.Name(fn); got != want {
			t.Errorf("Name: got %q, want %q", got, want)
		}
	}
}

func TestNamePlain(t *testing.T) {
	tests := map[string]step.Func[any]{
		"fetch":   fetch,
		"process": process,
		"store":   store,
		"":        nil,
	}
	for want, fn := range tests {
		if got := step.Name(fn); got != want {
			t.Errorf("Name: got %q, want %q", got, want)
		}
	}
}

func TestEqual(t *testing.T) {
	var s seq
	if !step.Equal(s.start, s.start) {
		t.Error("same function not equal")
	}
	if step.Equal(s.start, s.stepA) {
		t.Error("different functions equal")
	}
}

type skipper bool

func (s *skipper) step1(context.Context) (step.Func[skipper], error) {
	return s.step2, nil
}

func (s *skipper) step2(context.Context) (step.Func[skipper], error) {
	*s = true
	return s.step3, fmt.Errorf("skip")
}

func (s *skipper) step3(context.Context) (step.Func[skipper], error) {
	return nil, nil
}

func TestNonFatalError(t *testing.T) {
	var s skipper
	var got step.Info
	h := step.HandlerFunc(func(i step.Info) {
		if i.Name == "step2" {
			got = i
		}
	})
	err := step.Do(t.Context(), s.step1, h)
	if err != nil {
		t.Fatalf("Do err: %v", err)
	}
	if !s {
		t.Error("step2 was not reached")
	}
	if got.Err == nil {
		t.Fatal("handler did not receive error")
	}
	if got.Next == "" {
		t.Error("expected non-empty Next for non-fatal step")
	}
}

type failer struct{}

func (f failer) step1(context.Context) (step.Func[failer], error) {
	return f.step2, nil
}

func (f failer) step2(context.Context) (step.Func[failer], error) {
	return nil, fmt.Errorf("fatal error")
}

func TestFatalError(t *testing.T) {
	var f failer
	var got step.Info
	h := step.HandlerFunc(func(i step.Info) {
		if i.Name == "step2" {
			got = i
		}
	})
	err := step.Do(t.Context(), f.step1, h)
	if err == nil {
		t.Fatal("expected error")
	}
	var stepErr *step.Error
	if !errors.As(err, &stepErr) {
		t.Fatalf("expected *step.Error, got %T", err)
	}
	if stepErr.Name != "step2" {
		t.Errorf("Error.Name: got %q, want %q",
			stepErr.Name, "step2")
	}
	if got.Err == nil {
		t.Fatal("handler did not receive error")
	}
	if got.Next != "" {
		t.Errorf("expected empty Next, got %q", got.Next)
	}
}
