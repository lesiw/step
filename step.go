// Package lesiw.io/step runs state machines built from functions.
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
// Run the machine by passing a context and the first step:
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
// # Stateless Steps
//
// Steps do not require a state type. Plain functions work with any type
// parameter:
//
//	func fetch(context.Context) (step.Func[any], error) {
//		return process, nil
//	}
//
// # Testing
//
// Use [Equal] and [Name] to test state transitions:
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
	"fmt"
	"io"
	"reflect"
	"runtime"
	"strings"
)

// Func is a step in a state machine. Each step receives a context and
// returns the next step to run or nil to stop. Step functions are typically
// methods on a state type, bound as method values:
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

// Handler handles step completion events.
type Handler interface {
	Handle(Info, error)
}

// HandlerFunc is an adapter to allow the use of ordinary functions as step
// handlers.
type HandlerFunc func(Info, error)

// Handle calls f(i, err).
func (f HandlerFunc) Handle(i Info, err error) { f(i, err) }

// Do executes a state machine starting from fn. It checks for context
// cancellation before each step and stops on the first non-nil error.
// Handlers are called in order after each step.
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
		if err != nil {
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
// type parameter ensures both functions belong to the same state machine.
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

// Log returns a [Handler] that writes step results to w. Passed steps are
// prefixed with a check mark; failed steps are prefixed with an X followed
// by the error.
//
//ignore:errcheck
func Log(w io.Writer) Handler {
	return HandlerFunc(func(i Info, err error) {
		if err != nil {
			fmt.Fprintf(w, "✘ %s\n", err)
		} else {
			fmt.Fprintf(w, "✔ %s\n", i.Name)
		}
	})
}
