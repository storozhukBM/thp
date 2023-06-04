package thp_test

import (
	"errors"
	"reflect"
	"testing"
)

func expectPanic(t *testing.T, f func(), expectedError error) {
	t.Helper()
	var caughtPanic error
	func() {
		defer func() {
			actualPanic, ok := recover().(error)
			if !ok {
				t.Fatal("recovered panic is not error")
			}
			caughtPanic = actualPanic
			if expectedError != nil {
				if actualPanic == nil {
					t.Fatalf(
						"expected error didn't happen. expected %T(%v)",
						expectedError, expectedError,
					)
				}
				if !errors.Is(actualPanic, expectedError) {
					t.Fatalf(
						"unexpected error type. expected %T(%v); actual: %T(%v)",
						expectedError, expectedError, actualPanic, actualPanic,
					)
				}
				if actualPanic.Error() != expectedError.Error() {
					t.Fatalf(
						"unexpected error formatting. expected %T(%v); actual: %T(%v)",
						expectedError, expectedError, actualPanic, actualPanic,
					)
				}
			}
		}()
		f()
	}()
	if caughtPanic == nil {
		t.Fatal("panic isn't detected")
	}
}

func eq[V any](t *testing.T, expected V, actual V) {
	t.Helper()
	if !reflect.DeepEqual(expected, actual) {
		t.Fatalf("\nexp: %T:`%#v`\nact: %T:`%#v`", expected, expected, actual, actual)
	}
}
