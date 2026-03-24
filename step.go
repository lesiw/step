// Package lesiw.io/step runs sequences built from functions.
//
// A step is a function that receives a context and returns the next step
// to run. Step functions are typically defined as methods on a state type,
// then passed to [Do] as method values:
//
//	type etl struct {
//		raw    []byte
//		parsed []string
//	}
//
//	func (e *etl) extract(context.Context) (step.Func[etl], error) {
//		e.raw = []byte("a,b,c")
//		return e.transform, nil
//	}
//
//	func (e *etl) transform(context.Context) (step.Func[etl], error) {
//		e.parsed = strings.Split(string(e.raw), ",")
//		return e.load, nil
//	}
//
//	func (e *etl) load(context.Context) (step.Func[etl], error) {
//		fmt.Println(e.parsed)
//		return nil, nil
//	}
//
// Run the sequence by passing a context and the first step:
//
//	var e etl
//	if err := step.Do(ctx, e.extract); err != nil {
//		log.Fatal(err)
//	}
//
// # Branching
//
// Steps can branch by returning different functions:
//
//	func (d *deploy) detectOS(context.Context) (step.Func[deploy], error) {
//		switch d.os {
//		case "linux":
//			return d.installLinux, nil
//		case "darwin":
//			return d.installDarwin, nil
//		}
//		return nil, fmt.Errorf("unsupported OS: %s", d.os)
//	}
//
// # Error Handling
//
// When a step returns a non-nil error, [Do] wraps it in [*Error] with the
// step name:
//
//	err := step.Do(ctx, e.extract)
//	if stepErr, ok := errors.AsType[*step.Error](err); ok {
//		fmt.Println("failed at:", stepErr.Name)
//	}
//
// [Do] also checks for context cancellation before each step.
//
// To signal a non-fatal condition, wrap the error with [Continue]:
//
//	func (d *deploy) install(context.Context) (step.Func[deploy], error) {
//		if !d.needsInstall {
//			return d.configure, step.Continue(fmt.Errorf("skip"))
//		}
//		// ... do the install ...
//	}
//
// [Do] passes [Continue] errors to handlers but does not stop the
// sequence. Handlers can inspect the underlying error to decide how to
// render it. [Log] prints continued steps with ⊘:
//
//	✔ detectOS
//	⊘ install: skip
//	✔ configure
//
// # Handlers
//
// A [Handler] receives step completion events. [Log] provides a default
// handler that prints check marks and X marks:
//
//	step.Do(ctx, e.extract, step.Log(os.Stderr))
//
//	✔ extract
//	✔ transform
//	✘ load: something went wrong
//
// Multiple handlers run in sequence:
//
//	step.Do(ctx, e.extract, step.Log(os.Stderr), step.HandlerFunc(e.handle))
//
// Since the handler is called after each step, the handler itself can be
// a method on the state type. This is useful for buffered logging, where
// step output is captured and only shown on failure:
//
//	type etl struct {
//		bytes.Buffer
//		raw    []byte
//		parsed []string
//	}
//
//	func (e *etl) handle(i step.Info, err error) {
//		if err != nil {
//			io.Copy(os.Stderr, e)
//		}
//		e.Reset()
//	}
//
// # Testing
//
// Use [Equal] and [Name] to test transitions:
//
//	func TestDetectLinux(t *testing.T) {
//		d := &deploy{os: "linux"}
//		got, err := d.detectOS(t.Context())
//		if err != nil {
//			t.Fatalf("detectOS err: %v", err)
//		}
//		if want := d.installLinux; !step.Equal(got, want) {
//			t.Errorf("got %s, want %s", step.Name(got), step.Name(want))
//		}
//	}
//
// [Equal] and [Name] compare and identify functions by name using the
// runtime, making step transitions testable without comparing function
// values directly.
package step

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"runtime"
	"strings"
)

// Func is a step in a sequence. Each step receives a context and returns
// the next step to run or nil to stop. Step functions are typically methods
// on a state type, bound as method values:
//
//	step.Do(ctx, e.extract)
type Func[T any] func(context.Context) (Func[T], error)

// Info holds metadata about a step.
type Info struct {
	// Name is the name of the step function.
	Name string
}

// Error is the error type returned by [Do] when a step fails.
type Error struct {
	Info
	error
}

func (e *Error) Error() string { return e.Name + ": " + e.error.Error() }
func (e *Error) Unwrap() error { return e.error }

// Continue wraps an error to indicate that the sequence should not stop.
// When a step returns an error wrapped with Continue, [Do] passes it to
// handlers but continues to the next step instead of returning. This
// allows custom non-fatal signals like skip or warn:
//
//	var Skip = step.Continue(fmt.Errorf("skip"))
//
// Handlers can inspect the underlying error with [errors.Is] or
// [errors.As]:
//
//	func handle(i step.Info, err error) {
//		if errors.Is(err, Skip) {
//			fmt.Printf("⊘ %s\n", i.Name)
//		}
//	}
func Continue(err error) error { return &continueError{err: err} }

type continueError struct{ err error }

func (c *continueError) Error() string { return c.err.Error() }
func (c *continueError) Unwrap() error { return c.err }

// Handler handles step completion events.
type Handler interface {
	Handle(Info, error)
}

// HandlerFunc is an adapter to allow the use of ordinary functions as step
// handlers.
type HandlerFunc func(Info, error)

// Handle calls f(i, err).
func (f HandlerFunc) Handle(i Info, err error) { f(i, err) }

// Do executes a sequence starting from fn. It checks for context
// cancellation before each step and stops on the first non-nil error.
// If the error wraps [Continue], handlers are called but the sequence
// continues to the next step. Handlers are called in order after each
// step.
func Do[T any](ctx context.Context, fn Func[T], handlers ...Handler) error {
	for fn != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
		i := Info{Name: Name(fn)}
		var err error
		fn, err = fn(ctx)
		if err != nil {
			err = &Error{Info: i, error: err}
		}
		for _, h := range handlers {
			h.Handle(i, err)
		}
		if ce := new(continueError); err != nil && !errors.As(err, &ce) {
			return err
		}
	}
	return nil
}

// Name returns the short name of a step function.
func Name[T any](fn Func[T]) string {
	s := strings.Split(fullName(fn), ".")
	return strings.TrimSuffix(s[len(s)-1], "-fm")
}

// Equal reports whether two step functions refer to the same function. The
// type parameter ensures both functions belong to the same sequence.
// Equal compares fully qualified runtime names, so identically named
// functions in different packages are not considered equal.
func Equal[T any](a, b Func[T]) bool { return fullName(a) == fullName(b) }

func fullName[T any](fn Func[T]) string {
	// This use of reflect does not affect dead code elimination.
	// As of Go 1.26, the linker only disables DCE when it sees
	// reflect.Value.Method or reflect.Type.MethodByName (flagged
	// REFLECTMETHOD); Pointer does not trigger that path.
	// See cmd/link/internal/ld/deadcode.go.
	return runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name()
}

// Log returns a [Handler] that writes step results to w.
//
//	✔ passed step
//	✘ failed step: error message
//	⊘ continued step: error message
//
//ignore:errcheck
func Log(w io.Writer) Handler {
	return HandlerFunc(func(i Info, err error) {
		if ce := new(continueError); err == nil {
			fmt.Fprintf(w, "✔ %s\n", i.Name)
		} else if errors.As(err, &ce) {
			fmt.Fprintf(w, "⊘ %s: %s\n", i.Name, ce.err)
		} else {
			fmt.Fprintf(w, "✘ %s\n", err)
		}
	})
}
