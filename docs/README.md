# lesiw.io/step

[![Go Reference](https://pkg.go.dev/badge/lesiw.io/step.svg)](https://pkg.go.dev/lesiw.io/step)

Package step runs sequences of step functions.

A step is a function that receives a context and returns the next step
to run. Step functions are typically defined as methods on a state
type, then passed to `Do` as method values:

```go
type etl struct {
    raw    []byte
    parsed []string
}

func (e *etl) extract(context.Context) (step.Func[etl], error) {
    e.raw = []byte("a,b,c")
    return e.transform, nil
}

func (e *etl) transform(context.Context) (step.Func[etl], error) {
    e.parsed = strings.Split(string(e.raw), ",")
    return e.load, nil
}

func (e *etl) load(context.Context) (step.Func[etl], error) {
    fmt.Println(e.parsed)
    return nil, nil
}
```

Run the sequence by passing a context and the first step:

```go
var e etl
if err := step.Do(ctx, e.extract); err != nil {
    log.Fatal(err)
}
```

## Branching

Steps can branch by returning different functions:

```go
func (d *deploy) install(context.Context) (step.Func[deploy], error) {
    switch d.os {
    case "linux":
        return d.installLinux, nil
    case "darwin":
        return d.installDarwin, nil
    }
    return nil, fmt.Errorf("unsupported OS: %s", d.os)
}
```

## Error Handling

When a step returns a non-nil error, `Do` wraps it in `*Error` with
the step name:

```go
err := step.Do(ctx, e.extract)
if stepErr, ok := errors.AsType[*step.Error](err); ok {
    fmt.Println("failed at:", stepErr.Name)
}
```

`Do` also checks for context cancellation before each step.

To signal a non-fatal condition, wrap the error with `Continue`:

```go
func (d *deploy) install(context.Context) (step.Func[deploy], error) {
    if !d.needsInstall {
        return d.configure, step.Continue(fmt.Errorf("skip"))
    }
    // ... do the install ...
}
```

`Do` passes `Continue` errors to handlers but does not stop the
sequence. `Log` prints continued steps with ⊘:

```
✔ download
⊘ install: skip
✔ configure
```

## Handlers

A `Handler` receives step completion events. `Log` provides a default
handler that prints check marks and X marks:

```go
step.Do(ctx, e.extract, step.Log(os.Stderr))
```

```
✔ extract
✔ transform
✘ load: something went wrong
```

Multiple handlers run in sequence:

```go
step.Do(ctx, e.extract, step.Log(os.Stderr), step.HandlerFunc(e.handle))
```

Since the handler is called after each step, the handler itself can
be a method on the state type. This is useful for buffered logging,
where step output is captured and only shown on failure:

```go
type etl struct {
    bytes.Buffer
    raw    []byte
    parsed []string
}

func (e *etl) handle(i step.Info, err error) {
    if err != nil {
        io.Copy(os.Stderr, e)
    }
    e.Reset()
}
```

## Testing

Use `Equal` and `Name` to test transitions:

```go
func TestInstallLinux(t *testing.T) {
    d := &deploy{os: "linux"}
    got, err := d.install(t.Context())
    if err != nil {
        t.Fatalf("install err: %v", err)
    }
    if want := d.installLinux; !step.Equal(got, want) {
        t.Errorf("got %s, want %s", step.Name(got), step.Name(want))
    }
}
```

`Equal` and `Name` compare and identify functions by name using the
runtime, making step transitions testable without comparing function
values directly.
