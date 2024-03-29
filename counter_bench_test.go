package thp_test

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/storozhukBM/thp"
)

func BenchmarkCounterThroughput(b *testing.B) {
	for pIdx := 1; pIdx <= 32; pIdx *= 2 {
		b.Run(fmt.Sprintf("type:%s;goroutines:%d", "atomic", pIdx), func(b *testing.B) {
			regularAtomicCnt(b, pIdx)
		})
		b.Run(fmt.Sprintf("type:%s;goroutines:%d", "thp", pIdx), func(b *testing.B) {
			thpCnt(b, pIdx)
		})
	}
}

//nolint:thelper // This is not exactly helper and in case of error we want to know line
func thpCnt(b *testing.B, goroutines int) {
	counter := thp.NewCounterWithWideness(goroutines)

	canRun := &sync.WaitGroup{}
	canRun.Add(1)

	wg := &sync.WaitGroup{}
	wg.Add(goroutines)

	incsPerGoroutine := b.N / goroutines
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			canRun.Wait()

			for j := 0; j < incsPerGoroutine; j++ {
				counter.Add(1)
			}
		}()
	}

	b.ResetTimer()
	b.ReportAllocs()
	canRun.Done()

	wg.Wait()
	b.StopTimer()

	expectedResult := int64(goroutines * incsPerGoroutine)
	if counter.Load() != expectedResult {
		b.Errorf("result is not as expected: %v != %v", counter.Load(), expectedResult)
	}
}

//nolint:thelper // This is not exactly helper and in case of error we want to know line
func regularAtomicCnt(b *testing.B, goroutines int) {
	counter := atomic.Int64{}

	canRun := &sync.WaitGroup{}
	canRun.Add(1)

	wg := &sync.WaitGroup{}
	wg.Add(goroutines)

	incsPerGoroutine := b.N / goroutines
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			canRun.Wait()

			for j := 0; j < incsPerGoroutine; j++ {
				counter.Add(1)
			}
		}()
	}

	b.ResetTimer()
	b.ReportAllocs()
	canRun.Done()

	wg.Wait()
	b.StopTimer()

	expectedResult := int64(goroutines * incsPerGoroutine)
	if counter.Load() != expectedResult {
		b.Errorf("result is not as expected: %v != %v", counter.Load(), expectedResult)
	}
}
