// Package lesiw.io/step runs sequences of step functions.
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
//	func (d *deploy) install(context.Context) (step.Func[deploy], error) {
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
// The returned function controls whether the sequence continues (non-nil)
// or stops (nil). The returned error is passed to handlers.
//
// [Do] may return an [*Error] containing the step name:
//
//	err := step.Do(ctx, e.extract)
//	if stepErr, ok := errors.AsType[*step.Error](err); ok {
//		fmt.Println("failed at:", stepErr.Name)
//	}
//
// [Do] also checks for context cancellation before each step.
//
// When a step returns an error but the sequence does not stop, [Log]
// renders it with ⊘:
//
//	✔ download
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
//	func (e *etl) handle(i step.Info) {
//		if i.Err != nil {
//			io.Copy(os.Stderr, e)
//		}
//		e.Reset()
//	}
//
// # Testing
//
// Use [Equal] and [Name] to test transitions:
//
//	func TestInstallLinux(t *testing.T) {
//		d := &deploy{os: "linux"}
//		got, err := d.install(t.Context())
//		if err != nil {
//			t.Fatalf("install err: %v", err)
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

// Func is a step in a sequence. Each step receives a context and returns
// the next step to run or nil to stop. Step functions are typically
// methods on a state type, bound as method values:
//
//	step.Do(ctx, e.extract)
type Func[T any] func(context.Context) (Func[T], error)

// Info holds metadata about a completed step.
type Info struct {
	// Name is the name of the step function.
	Name string
	// Next is the name of the next step, if any.
	Next string
	// Err is the error returned by the step, if any.
	Err error
}

// Error is the error type returned by [Do] when a step fails.
type Error Info

func (e *Error) Error() string { return e.Name + ": " + e.Err.Error() }
func (e *Error) Unwrap() error { return e.Err }

// Handler handles step completion events.
type Handler interface {
	Handle(Info)
}

// HandlerFunc is an adapter to allow the use of ordinary functions as
// step handlers.
type HandlerFunc func(Info)

// Handle calls f(i).
func (f HandlerFunc) Handle(i Info) { f(i) }

// Do executes a sequence starting from fn. It checks for context
// cancellation before each step. The returned function controls whether
// the sequence continues (non-nil) or stops (nil). The returned error
// is passed to handlers. Handlers are called in order after each step.
func Do[T any](ctx context.Context, f Func[T], h ...Handler) (err error) {
	for f != nil {
		if err = ctx.Err(); err != nil {
			return err
		}
		i := Info{Name: Name(f)}
		f, i.Err = f(ctx)
		i.Next = Name(f)
		for _, handler := range h {
			handler.Handle(i)
		}
		if f == nil && i.Err != nil {
			e := Error(i)
			return &e
		}
	}
	return nil
}

// Name returns the short name of a step function.
func Name[T any](fn Func[T]) string {
	if fn == nil {
		return ""
	}
	s := strings.Split(fullName(fn), ".")
	return strings.TrimSuffix(s[len(s)-1], "-fm")
}

// Equal reports whether two step functions refer to the same function.
// The type parameter ensures both functions belong to the same sequence.
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
	return HandlerFunc(func(i Info) {
		if i.Err == nil {
			fmt.Fprintf(w, "✔ %s\n", i.Name)
		} else if i.Next != "" {
			fmt.Fprintf(w, "⊘ %s: %s\n", i.Name, i.Err)
		} else {
			fmt.Fprintf(w, "✘ %s: %s\n", i.Name, i.Err)
		}
	})
}
