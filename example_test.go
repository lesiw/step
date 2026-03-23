package step_test

import (
	"context"
	"os"
	"strings"

	"lesiw.io/step"
)

type etl struct {
	raw    []byte
	parsed []string
}

func Example() {
	var e etl
	err := step.Do(context.Background(), e.extract, step.Log(os.Stdout))
	if err != nil {
		os.Exit(1)
	}
	// Output:
	// ✔ extract
	// ✔ transform
	// ✔ load
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
	return nil, nil
}
