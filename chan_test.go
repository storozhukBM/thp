package thp_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/storozhukBM/thp"
)

func TestNewChan(t *testing.T) {
	expectPanic(t, func() {
		thp.NewChan[*int](-1)
	}, thp.ChanBatchSizeError)
	expectPanic(t, func() {
		thp.NewChan[*int](0)
	}, thp.ChanBatchSizeError)
	_, _ = thp.NewChan[*int](1)
}

func TestChan(t *testing.T) {
	t.Parallel()

	poolSizes := []int{1, 2, 4, 8, 9, 16, 31}
	batchSizes := []int{1, 2, 4, 8, 10, 1024}
	itemsPerProducers := []int{1, 2, 4, 5, 10, 31, 33, 100, 1024}

	for _, p := range poolSizes {
		for _, c := range poolSizes {
			for _, batchSize := range batchSizes {
				for _, itemsPerProducer := range itemsPerProducers {
					t.Run(fmt.Sprintf("primitive;p:%v;c:%v;bSz:%v;iPP:%v", p, c, batchSize, itemsPerProducer), func(t *testing.T) {
						runPrimitiveChanTest(t, batchSize, p, c, itemsPerProducer)
					})
					t.Run(fmt.Sprintf("obj;p:%v;c:%v;bSz:%v;iPP:%v", p, c, batchSize, itemsPerProducer), func(t *testing.T) {
						runObjChanTest(t, batchSize, p, c, itemsPerProducer)
					})
				}
			}
		}
	}
}

func runPrimitiveChanTest(t *testing.T, batchSize int, producersCnt int, consumersCnt int, itemsPerProducer int) {
	ch, chCloser := thp.NewChan[int](batchSize)
	producersWg := &sync.WaitGroup{}
	producersWg.Add(producersCnt)
	for i := 0; i < producersCnt; i++ {
		go func() {
			defer producersWg.Done()
			producer, flush := ch.Producer(context.Background())
			defer flush()
			for j := 0; j < itemsPerProducer; j++ {
				producer.Put(1)
			}
		}()
	}

	consumersWg := &sync.WaitGroup{}
	consumersWg.Add(consumersCnt)
	counter := &atomic.Int64{}
	for i := 0; i < consumersCnt; i++ {
		go func() {
			defer consumersWg.Done()
			consumer := ch.Consumer(context.Background())
			result := 0
			item, ok := consumer.Poll()
			for ; ok; item, ok = consumer.Poll() {
				result += item
			}
			counter.Add(int64(result))
		}()
	}

	producersWg.Wait()
	chCloser()
	consumersWg.Wait()

	expectedResult := int64(producersCnt * itemsPerProducer)
	if counter.Load() != expectedResult {
		t.Errorf("result is not as expected: %v != %v", counter.Load(), expectedResult)
	}
}

func runObjChanTest(t *testing.T, batchSize int, producersCnt int, consumersCnt int, itemsPerProducer int) {
	ch, chCloser := thp.NewChan[*int](batchSize)
	producersWg := &sync.WaitGroup{}
	producersWg.Add(producersCnt)
	for i := 0; i < producersCnt; i++ {
		go func() {
			defer producersWg.Done()
			producer, flush := ch.Producer(context.Background())
			defer flush()
			for j := 0; j < itemsPerProducer; j++ {
				msg := 1
				producer.Put(&msg)
			}
		}()
	}

	consumersWg := &sync.WaitGroup{}
	consumersWg.Add(consumersCnt)
	counter := &atomic.Int64{}
	for i := 0; i < consumersCnt; i++ {
		go func() {
			defer consumersWg.Done()
			consumer := ch.Consumer(context.Background())
			result := 0
			item, ok := consumer.Poll()
			for ; ok; item, ok = consumer.Poll() {
				result += *item
			}
			counter.Add(int64(result))
		}()
	}

	producersWg.Wait()
	chCloser()
	consumersWg.Wait()

	expectedResult := int64(producersCnt * itemsPerProducer)
	if counter.Load() != expectedResult {
		t.Errorf("result is not as expected: %v != %v", counter.Load(), expectedResult)
	}
}

func expectPanic(t *testing.T, f func(), expectedError error) string {
	t.Helper()
	var actualPanic interface{}
	func() {
		defer func() {
			actualPanic = recover()
			if expectedError != nil {
				if actualPanic == nil {
					t.Fatalf(
						"expected error didn't happen. expected %T(%v)",
						expectedError, expectedError,
					)
				}
				if !errors.Is(actualPanic.(error), expectedError) {
					t.Fatalf(
						"unexpected error type. expected %T(%v); actual: %T(%v)",
						expectedError, expectedError, actualPanic, actualPanic,
					)
				}
			}
		}()
		f()
	}()
	if actualPanic == nil {
		t.Fatal("panic isn't detected")
	}
	return actualPanic.(error).Error()
}
