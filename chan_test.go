package thp_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/storozhukBM/thp"
)

func TestNewChan(t *testing.T) {
	t.Parallel()

	expectPanic(t, func() {
		thp.NewChan[*int](-1)
	}, thp.ErrChanBatchSize)
	expectPanic(t, func() {
		thp.NewChan[*int](0)
	}, thp.ErrChanBatchSize)
	_, _ = thp.NewChan[*int](1)
}

func TestPerefetch(t *testing.T) {
	t.Parallel()

	ch, chCloser := thp.NewChan[int](3)
	defer chCloser()

	producer, flush := ch.Producer(context.Background())
	go func() {
		defer flush()
		for i := 0; i < 3; i++ {
			producer.Put(1)
		}
	}()

	res := 0
	ctx, cancel := context.WithCancel(context.Background())
	consumer := ch.Consumer(ctx)
	for i := 0; i < 3; i++ {
		value, ok := consumer.Poll()
		eq(t, true, ok)
		res += value
	}
	eq(t, 3, res)

	cancel()
	value, ok := consumer.Poll()
	eq(t, false, ok)
	eq(t, 0, value)

	oneMoreValue, ok := consumer.Poll()
	eq(t, false, ok)
	eq(t, 0, oneMoreValue)
}

func TestNonBlockingFlush(t *testing.T) {
	t.Parallel()

	ch, chCloser := thp.NewChan[int](3)
	defer chCloser()

	consumer := ch.Consumer(context.Background())
	// Check NonBlockingPoll is empty on empty channel
	{
		s, ok, stillOpen := consumer.NonBlockingPoll()
		eq(t, 0, s)
		eq(t, false, ok)
		eq(t, true, stillOpen)
	}
	pCtx, pCtxCancel := context.WithCancel(context.Background())
	defer pCtxCancel()

	producer, flush := ch.Producer(pCtx)
	flush()

	// Check NonBlockingPoll is empty after empty flush
	{
		s, ok, stillOpen := consumer.NonBlockingPoll()
		eq(t, 0, s)
		eq(t, false, ok)
		eq(t, true, stillOpen)
	}

	// Check NonBlockingFlush goes returns false on empty batch
	{
		result := producer.NonBlockingFlush()
		eq(t, false, result)
	}

	producer.Put(1)

	// Check NonBlockingPoll is empty without flush
	{
		s, ok, stillOpen := consumer.NonBlockingPoll()
		eq(t, 0, s)
		eq(t, false, ok)
		eq(t, true, stillOpen)
	}
	// Check NonBlockingPoll goes through with first item
	{
		result := producer.NonBlockingFlush()
		eq(t, true, result)
	}

	// thp.Chan internally has capacity == runtime.NumCPU
	for i := 1; i < runtime.NumCPU(); i++ {
		producer.Put(i + 1)
		result := producer.NonBlockingFlush()
		eq(t, true, result)
	}

	// Next Flush should block, but non-blocking fluch just returns false
	{
		producer.Put(runtime.NumCPU())
		result := producer.NonBlockingFlush()
		eq(t, false, result)
	}
}

func TestNonBlockingPut(t *testing.T) {
	t.Parallel()

	ch, chCloser := thp.NewChan[int](3)
	defer chCloser()

	consumer := ch.Consumer(context.Background())
	// Check NonBlockingPoll is empty on empty channel
	{
		s, ok, stillOpen := consumer.NonBlockingPoll()
		eq(t, 0, s)
		eq(t, false, ok)
		eq(t, true, stillOpen)
	}
	pCtx, pCtxCancel := context.WithCancel(context.Background())
	defer pCtxCancel()

	producer, flush := ch.Producer(pCtx)
	flush()

	// Check NonBlockingPoll is empty after empty flush
	{
		s, ok, stillOpen := consumer.NonBlockingPoll()
		eq(t, 0, s)
		eq(t, false, ok)
		eq(t, true, stillOpen)
	}

	// Check NonBlockingFlush goes returns false on empty batch
	{
		result := producer.NonBlockingFlush()
		eq(t, false, result)
	}

	{
		ok := producer.NonBlockingPut(1)
		eq(t, true, ok)
	}

	// Check NonBlockingPoll is empty without flush
	{
		s, ok, stillOpen := consumer.NonBlockingPoll()
		eq(t, 0, s)
		eq(t, false, ok)
		eq(t, true, stillOpen)
	}
	// Check NonBlockingPoll goes through with first item
	{
		result := producer.NonBlockingFlush()
		eq(t, true, result)
	}

	// thp.Chan internally has capacity == runtime.NumCPU
	for i := 1; i < runtime.NumCPU(); i++ {
		eq(t, true, producer.NonBlockingPut(i+1))
		eq(t, true, producer.NonBlockingPut(i+1))
		eq(t, true, producer.NonBlockingPut(i+1))
	}

	// Next Flush should block, but non-blocking fluch just returns false
	{
		eq(t, true, producer.NonBlockingPut(runtime.NumCPU()+1))
		eq(t, true, producer.NonBlockingPut(runtime.NumCPU()+1))
		result := producer.NonBlockingPut(runtime.NumCPU() + 1)
		eq(t, false, result)
	}
}

func TestNonBlockingFetch(t *testing.T) {
	t.Parallel()

	// New channel
	ch, chCloser := thp.NewChan[string](3)

	consumer := ch.Consumer(context.Background())
	// Check NonBlockingPoll is empty on empty channel
	{
		s, ok, stillOpen := consumer.NonBlockingPoll()
		eq(t, "", s)
		eq(t, false, ok)
		eq(t, true, stillOpen)
	}

	producerCtx, producerCtxCancel := context.WithCancel(context.Background())

	// Put one item into a batch, but don't flush
	producer, flush := ch.Producer(producerCtx)
	producer.Put("a")

	// Check that NonBlockingPoll is still empty on empty channel
	{
		s, ok, stillOpen := consumer.NonBlockingPoll()
		eq(t, "", s)
		eq(t, false, ok)
		eq(t, true, stillOpen)
	}

	// Flush to commit batch
	flush()

	// Check that NonBlockingPoll returns expected value
	{
		s, ok, stillOpen := consumer.NonBlockingPoll()
		eq(t, "a", s)
		eq(t, true, ok)
		eq(t, true, stillOpen)
	}
	// Now check that channel is empty
	{
		s, ok, stillOpen := consumer.NonBlockingPoll()
		eq(t, "", s)
		eq(t, false, ok)
		eq(t, true, stillOpen)
	}

	// Empty batch flush
	flush()

	// Check that NonBlockingPoll is still empty on empty channel
	{
		s, ok, stillOpen := consumer.NonBlockingPoll()
		eq(t, "", s)
		eq(t, false, ok)
		eq(t, true, stillOpen)
	}

	producerCtxCancel()

	// Check that NonBlockingPoll is still empty on empty channel
	{
		s, ok, stillOpen := consumer.NonBlockingPoll()
		eq(t, "", s)
		eq(t, false, ok)
		eq(t, true, stillOpen)
	}

	chCloser()

	// Check that NonBlockingPoll is still empty on closed channel
	{
		s, ok, stillOpen := consumer.NonBlockingPoll()
		eq(t, "", s)
		eq(t, false, ok)
		eq(t, false, stillOpen)
	}
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
					iPP := itemsPerProducer
					t.Run(
						fmt.Sprintf(
							"primitive;p:%v;c:%v;bSz:%v;iPP:%v",
							p, c, batchSize, iPP,
						), func(t *testing.T) {
							t.Parallel()
							runPrimitiveChanTest(t, batchSize, p, c, iPP)
						},
					)
					t.Run(
						fmt.Sprintf(
							"obj;p:%v;c:%v;bSz:%v;iPP:%v",
							p, c, batchSize, iPP,
						),
						func(t *testing.T) {
							t.Parallel()
							runObjChanTest(t, batchSize, p, c, iPP)
						},
					)
				}
			}
		}
	}
}

//nolint:thelper // This is not exactly helper and in case of error we want to know line
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

//nolint:thelper // This is not exactly helper and in case of error we want to know line
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
