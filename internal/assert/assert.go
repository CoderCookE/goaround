package assert

import (
	"fmt"
	"testing"
)

type Asserter struct {
	T *testing.T
}

func (a *Asserter) Equal(actual, expected interface{}) {
	a.T.Helper()

	if expected != actual {
		message := fmt.Sprintf("expected: %s did not equal actual: %s", expected, actual)
		a.T.Error(message)
	}
}

func (a *Asserter) NotEqual(actual, expected interface{}) {
	a.T.Helper()

	if expected == actual {
		message := fmt.Sprintf("expected: %s was equal to actual: %s", expected, actual)
		a.T.Error(message)
	}
}

func (a *Asserter) True(actual bool) {
	a.T.Helper()

	expected := true
	if actual != expected {
		message := fmt.Sprintf("expected %v got  %v", expected, actual)
		a.T.Error(message)
	}
}

func (a *Asserter) False(actual bool) {
	a.T.Helper()

	expected := false
	if actual != expected {
		message := fmt.Sprintf("expected %v got %v", expected, actual)
		a.T.Error(message)
	}
}
