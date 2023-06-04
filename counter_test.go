package thp_test

import (
	"fmt"
	"math/rand"
	"runtime"
	"sync"
	"testing"

	"github.com/storozhukBM/thp"
)

func TestCounterExample(t *testing.T) {
	counter := thp.NewCounter()
	incsPerGoroutine := 1_000_000
	wg := &sync.WaitGroup{}
	wg.Add(runtime.NumCPU())
	for i := 0; i < runtime.NumCPU(); i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < incsPerGoroutine; j++ {
				counter.Add(1)
			}
		}()
	}
	wg.Wait()
	expectedResult := int64(runtime.NumCPU() * incsPerGoroutine)
	if counter.Load() != expectedResult {
		t.Errorf("result is not as expected: %v != %v", counter.Load(), expectedResult)
	}
}

func TestCounter(t *testing.T) {
	t.Parallel()

	t.Run("default", func(t *testing.T) {
		t.Parallel()
		defaultCounter := thp.NewCounter()
		runCounterTest(t, defaultCounter)
	})

	counterWidenessList := []int{-1, 0, 1, 2, runtime.NumCPU(), runtime.NumCPU() + 1}
	for _, w := range counterWidenessList {
		wideness := w
		t.Run(
			fmt.Sprintf("wideness: %v", wideness),
			func(t *testing.T) {
				t.Parallel()
				counter := thp.NewCounterWithWideness(wideness)
				runCounterTest(t, counter)
			},
		)
	}
}

//nolint:thelper // This is not exactly helper and in case of error we want to know line
func runCounterTest(t *testing.T, counter *thp.Counter) {
	result := 256 + (rand.Int31() / 256)
	perGoroutineIncs := int(result) / runtime.NumCPU()
	counter.Add(int64(result) % int64(runtime.NumCPU()))

	wg := &sync.WaitGroup{}
	wg.Add(runtime.NumCPU())
	for i := 0; i < runtime.NumCPU(); i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < perGoroutineIncs; j++ {
				counter.Add(1)
			}
		}()
	}

	wg.Wait()

	eq(t, int64(result), counter.Load())
	counter.Clear()
	eq(t, 0, counter.Load())
}
