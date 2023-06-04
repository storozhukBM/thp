package thp_test

import (
	"fmt"
	"math/rand"
	"runtime"
	"sync"
	"testing"

	"github.com/storozhukBM/thp"
)

func TestCounter(t *testing.T) {
	t.Parallel()

	counterWidenessList := []int{-1, 0, 1, 2, runtime.NumCPU(), runtime.NumCPU() + 1}
	for _, w := range counterWidenessList {
		t.Run(
			fmt.Sprintf("wideness: %v", w),
			func(t *testing.T) {
				t.Parallel()
				counter := thp.NewCounter(w)
				runCounterTest(t, counter)
			},
		)
	}
}

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
}
