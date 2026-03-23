package step_test

import (
	"context"
	"log"
	"os"

	"lesiw.io/step"
)

func ExampleFunc_plain() {
	err := step.Do(context.Background(), fetch, step.Log(os.Stdout))
	if err != nil {
		log.Fatal(err)
	}
	// Output:
	// ✔ fetch
	// ✔ process
	// ✔ store
}

func fetch(context.Context) (step.Func[any], error)   { return process, nil }
func process(context.Context) (step.Func[any], error) { return store, nil }
func store(context.Context) (step.Func[any], error)   { return nil, nil }
