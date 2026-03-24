package step_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"lesiw.io/step"
)

type deploy struct {
	os string
}

func (d *deploy) detectOS(context.Context) (step.Func[deploy], error) {
	switch d.os {
	case "linux":
		return d.installLinux, nil
	case "darwin":
		return d.installDarwin, nil
	}
	return nil, nil
}

func (d *deploy) installLinux(context.Context) (step.Func[deploy], error) {
	return d.deploy, nil
}

func (d *deploy) installDarwin(context.Context) (step.Func[deploy], error) {
	return d.deploy, nil
}

func (d *deploy) deploy(context.Context) (step.Func[deploy], error) {
	return nil, nil
}

func TestDetectLinux(t *testing.T) {
	d := &deploy{os: "linux"}
	got, err := d.detectOS(t.Context())
	if err != nil {
		t.Fatalf("detectOS err: %v", err)
	}
	if want := d.installLinux; !step.Equal(got, want) {
		t.Errorf("got %s, want %s", step.Name(got), step.Name(want))
	}
}

func TestDetectDarwin(t *testing.T) {
	d := &deploy{os: "darwin"}
	got, err := d.detectOS(t.Context())
	if err != nil {
		t.Fatalf("detectOS err: %v", err)
	}
	if want := d.installDarwin; !step.Equal(got, want) {
		t.Errorf("got %s, want %s", step.Name(got), step.Name(want))
	}
}

func TestDetectUnknown(t *testing.T) {
	d := &deploy{os: "plan9"}
	got, err := d.detectOS(t.Context())
	if err != nil {
		t.Fatalf("detectOS err: %v", err)
	}
	if got != nil {
		t.Errorf("got %s, want nil", step.Name(got))
	}
}

func TestInstallLinux(t *testing.T) {
	var d deploy
	got, err := d.installLinux(t.Context())
	if err != nil {
		t.Fatalf("installLinux err: %v", err)
	}
	if want := d.deploy; !step.Equal(got, want) {
		t.Errorf("got %s, want %s", step.Name(got), step.Name(want))
	}
}

func TestNameMethod(t *testing.T) {
	var d deploy
	tests := map[string]step.Func[deploy]{
		"detectOS":      d.detectOS,
		"installLinux":  d.installLinux,
		"installDarwin": d.installDarwin,
		"deploy":        d.deploy,
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
	}
	for want, fn := range tests {
		if got := step.Name(fn); got != want {
			t.Errorf("Name: got %q, want %q", got, want)
		}
	}
}

func TestEqualSame(t *testing.T) {
	var d deploy
	if !step.Equal(d.detectOS, d.detectOS) {
		t.Error("same function not equal")
	}
}

func TestEqualDifferent(t *testing.T) {
	var d deploy
	if step.Equal(d.detectOS, d.installLinux) {
		t.Error("different functions equal")
	}
}

var errSkip = step.Continue(fmt.Errorf("skip"))

type skipper bool

func (s *skipper) step1(context.Context) (step.Func[skipper], error) {
	return s.step2, nil
}

func (s *skipper) step2(context.Context) (step.Func[skipper], error) {
	*s = true
	return s.step3, errSkip
}

func (s *skipper) step3(context.Context) (step.Func[skipper], error) {
	return nil, nil
}

func TestContinueError(t *testing.T) {
	var s skipper
	err := step.Do(t.Context(), s.step1)
	if err != nil {
		t.Fatalf("Do err: %v", err)
	}
	if !s {
		t.Error("step2 was not reached")
	}
}

func TestContinueErrorHandlerReceivesError(t *testing.T) {
	var s skipper
	var got error
	h := step.HandlerFunc(func(i step.Info, err error) {
		if i.Name == "step2" {
			got = err
		}
	})
	err := step.Do(t.Context(), s.step1, h)
	if err != nil {
		t.Fatalf("Do err: %v", err)
	}
	if got == nil {
		t.Fatal("handler did not receive error for step2")
	}
	if !errors.Is(got, errSkip) {
		t.Errorf("got %v, want %v", got, errSkip)
	}
}

type failer struct{}

func (f failer) step1(context.Context) (step.Func[failer], error) {
	return f.step2, nil
}

func (f failer) step2(context.Context) (step.Func[failer], error) {
	return nil, fmt.Errorf("fatal error")
}

func TestNonContinueErrorStops(t *testing.T) {
	var f failer
	err := step.Do(t.Context(), f.step1)
	if err == nil {
		t.Fatal("expected error")
	}
}
