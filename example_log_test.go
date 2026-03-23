package step_test

import (
	"context"
	"fmt"
	"os"

	"lesiw.io/step"
)

func ExampleLog() {
	var p pipeline
	err := step.Do(context.Background(), p.step1, step.Log(os.Stdout))
	if err != nil {
		fmt.Fprintln(os.Stderr, "pipeline failed")
	}
	// Output:
	// ✔ step1
	// ✔ step2
	// ✘ step3: something went wrong
}

type pipeline struct{}

func (p pipeline) step1(context.Context) (step.Func[pipeline], error) {
	return p.step2, nil
}

func (p pipeline) step2(context.Context) (step.Func[pipeline], error) {
	return p.step3, nil
}

func (p pipeline) step3(context.Context) (step.Func[pipeline], error) {
	return nil, fmt.Errorf("something went wrong")
}
