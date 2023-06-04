package thp_test

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/storozhukBM/thp"
)

func TestExample(t *testing.T) {
	ch, chCloser := thp.NewChan[int](1024)
	producersWg := &sync.WaitGroup{}
	producersCount := 16
	itemsPerProducer := 1_000_000
	producersWg.Add(producersCount)
	for i := 0; i < producersCount; i++ {
		go func() {
			defer producersWg.Done()
			producer, flush := ch.Producer()
			defer flush()
			for j := 0; j < itemsPerProducer; j++ {
				producer.Put(1)
			}
		}()
	}

	consumersCount := 16
	consumersWg := &sync.WaitGroup{}
	consumersWg.Add(consumersCount)
	counter := &atomic.Int64{}
	for i := 0; i < consumersCount; i++ {
		go func() {
			defer consumersWg.Done()
			consumer := ch.Consumer()
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

	expectedResult := int64(producersCount * itemsPerProducer)
	if counter.Load() != expectedResult {
		t.Errorf("result is not as expected: %v != %v", counter.Load(), expectedResult)
	}
}

func TestStandardExample(t *testing.T) {
	ch := make(chan int, 1024)
	producersWg := &sync.WaitGroup{}
	producersCount := 16
	itemsPerProducer := 1_000_000
	producersWg.Add(producersCount)
	for i := 0; i < producersCount; i++ {
		go func() {
			defer producersWg.Done()
			for j := 0; j < itemsPerProducer; j++ {
				ch <- 1
			}
		}()
	}

	consumersCount := 16
	consumersWg := &sync.WaitGroup{}
	consumersWg.Add(consumersCount)
	counter := &atomic.Int64{}
	for i := 0; i < consumersCount; i++ {
		go func() {
			defer consumersWg.Done()
			result := 0
			for item := range ch {
				result += item
			}
			counter.Add(int64(result))
		}()
	}

	producersWg.Wait()
	close(ch)
	consumersWg.Wait()

	expectedResult := int64(producersCount * itemsPerProducer)
	if counter.Load() != expectedResult {
		t.Errorf("result is not as expected: %v != %v", counter.Load(), expectedResult)
	}
}

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

func TestPrefetch(t *testing.T) {
	t.Parallel()

	ch, chCloser := thp.NewChan[int](3)
	defer chCloser()

	producer, flush := ch.Producer()
	go func() {
		defer flush()
		for i := 0; i < 3; i++ {
			producer.Put(1)
		}
	}()

	res := 0
	ctx, cancel := context.WithCancel(context.Background())
	consumer := ch.Consumer()
	for i := 0; i < 3; i++ {
		value, ok, err := consumer.PollCtx(ctx)
		eq(t, true, ok)
		eq(t, nil, err)
		res += value
	}
	eq(t, 3, res)

	cancel()
	value, ok, err := consumer.PollCtx(ctx)
	eq(t, true, err != nil)
	eq(t, "context canceled", err.Error())
	eq(t, false, ok)
	eq(t, 0, value)

	oneMoreValue, ok, err := consumer.PollCtx(ctx)
	eq(t, true, err != nil)
	eq(t, "context canceled", err.Error())
	eq(t, false, ok)
	eq(t, 0, oneMoreValue)
}

func TestFlushCtx(t *testing.T) {
	t.Parallel()

	ch, chCloser := thp.NewChan[int](3)
	defer chCloser()

	consumerCtx, consumerCtxCancel := context.WithCancel(context.Background())
	consumer := ch.Consumer()

	producerCtx, producerCtxCancel := context.WithCancel(context.Background())
	producer, _ := ch.Producer()

	// FlushCtx of empty context returns no error
	{
		errFlush := producer.FlushCtx(producerCtx)
		eq(t, nil, errFlush)
	}

	{
		errPut := producer.PutCtx(producerCtx, 1)
		eq(t, nil, errPut)
	}

	// Check FlushCtx goes through with first item
	{
		errFlush := producer.FlushCtx(producerCtx)
		eq(t, nil, errFlush)
	}

	// thp.Chan internally has capacity == runtime.NumCPU
	for i := 1; i < runtime.NumCPU(); i++ {
		{
			errPut := producer.PutCtx(producerCtx, i+1)
			eq(t, nil, errPut)
		}
		{
			errPut := producer.PutCtx(producerCtx, i+1)
			eq(t, nil, errPut)
		}
		{
			errPut := producer.PutCtx(producerCtx, i+1)
			eq(t, nil, errPut)
		}
	}

	pwg := &sync.WaitGroup{}
	pwg.Add(1)
	// Next FlushCtx should block, but context cancellation unblocks it
	go func() {
		defer pwg.Done()
		errPut := producer.PutCtx(producerCtx, runtime.NumCPU())
		eq(t, nil, errPut)
		errFlush := producer.FlushCtx(producerCtx)
		eq(t, true, errFlush != nil)
		eq(t, "context canceled", errFlush.Error())
	}()

	time.Sleep(10 * time.Millisecond)
	producerCtxCancel()
	pwg.Wait()

	// FlushCtx on canceled ctx returns error right away
	{
		errFlush := producer.FlushCtx(producerCtx)
		eq(t, true, errFlush != nil)
		eq(t, "context canceled", errFlush.Error())
	}

	for {
		_, success, _ := consumer.NonBlockingPoll()
		if !success {
			break
		}
	}

	cwg := &sync.WaitGroup{}
	cwg.Add(1)
	// Next PollCtx should block, but context cancellation unblocks it
	go func() {
		defer cwg.Done()
		value, success, errPoll := consumer.PollCtx(consumerCtx)
		eq(t, 0, value)
		eq(t, false, success)
		eq(t, true, errPoll != nil)
		eq(t, "context canceled", errPoll.Error())
	}()

	time.Sleep(10 * time.Millisecond)
	consumerCtxCancel()
	cwg.Wait()
}

func TestNonBlockingFlush(t *testing.T) {
	t.Parallel()

	ch, chCloser := thp.NewChan[int](3)
	defer chCloser()

	consumer := ch.Consumer()
	// Check NonBlockingPoll is empty on empty channel
	{
		s, ok, stillOpen := consumer.NonBlockingPoll()
		eq(t, 0, s)
		eq(t, false, ok)
		eq(t, true, stillOpen)
	}

	producer, flush := ch.Producer()
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

	// Next Flush should block, but non-blocking flush just returns false
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

	consumer := ch.Consumer()
	// Check NonBlockingPoll is empty on empty channel
	{
		s, ok, stillOpen := consumer.NonBlockingPoll()
		eq(t, 0, s)
		eq(t, false, ok)
		eq(t, true, stillOpen)
	}

	producer, flush := ch.Producer()
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

	// Next Flush should block, but non-blocking flush just returns false
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

	consumer := ch.Consumer()
	// Check NonBlockingPoll is empty on empty channel
	{
		s, ok, stillOpen := consumer.NonBlockingPoll()
		eq(t, "", s)
		eq(t, false, ok)
		eq(t, true, stillOpen)
	}

	// Put one item into a batch, but don't flush
	producer, flush := ch.Producer()
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
			producer, flush := ch.Producer()
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
			consumer := ch.Consumer()
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
			producer, flush := ch.Producer()
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
			consumer := ch.Consumer()
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
