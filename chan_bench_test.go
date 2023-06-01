package thp_test

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/storozhukBM/thp"
)

func BenchmarkChanThroughput(b *testing.B) {
	for pIdx := 1; pIdx < 32; pIdx *= 2 {
		for bufSize := 1; bufSize <= 1024; bufSize *= 2 {
			b.Run(fmt.Sprintf("type:%s;pCnt:%d;cCnt:%d;buf:%d", "standard", pIdx, 8, bufSize), func(b *testing.B) {
				runStandardChan(b, pIdx, 8, bufSize)
			})
		}
		for bufSize := 1; bufSize <= 1024; bufSize *= 2 {
			b.Run(fmt.Sprintf("type:%s;pCnt:%d;cCnt:%d;buf:%d", "thp", pIdx, 8, bufSize), func(b *testing.B) {
				runThpChan(b, pIdx, 8, bufSize)
			})
		}
	}
}

//nolint:thelper // This is not exactly helper and in case of error we want to know line
func runStandardChan(b *testing.B, producersCnt int, consumersCnt int, bufferSize int) {
	canRun := &sync.WaitGroup{}
	canRun.Add(1)

	ch := make(chan int, bufferSize)

	itemsPerProducer := b.N / producersCnt
	producersWg := &sync.WaitGroup{}
	producersWg.Add(producersCnt)
	for i := 0; i < producersCnt; i++ {
		go func() {
			defer producersWg.Done()
			canRun.Wait()
			for j := 0; j < itemsPerProducer; j++ {
				ch <- 1
			}
		}()
	}

	consumersWg := &sync.WaitGroup{}
	consumersWg.Add(consumersCnt)
	counter := &atomic.Int64{}
	for i := 0; i < consumersCnt; i++ {
		go func() {
			defer consumersWg.Done()
			result := 0
			canRun.Wait()
			for item := range ch {
				result += item
			}
			counter.Add(int64(result))
		}()
	}

	b.ResetTimer()
	b.ReportAllocs()
	canRun.Done()

	producersWg.Wait()
	close(ch)
	consumersWg.Wait()
	b.StopTimer()

	expectedResult := int64(producersCnt * itemsPerProducer)
	if counter.Load() != expectedResult {
		b.Errorf("result is not as expected: %v != %v", counter.Load(), expectedResult)
	}
}

//nolint:thelper // This is not exactly helper and in case of error we want to know line
func runThpChan(b *testing.B, producersCnt int, consumersCnt int, bufferSize int) {
	canRun := &sync.WaitGroup{}
	canRun.Add(1)

	ch, chCloser := thp.NewChan[int](bufferSize)

	itemsPerProducer := b.N / producersCnt
	producersWg := &sync.WaitGroup{}
	producersWg.Add(producersCnt)
	for i := 0; i < producersCnt; i++ {
		go func() {
			defer producersWg.Done()
			producer, flush := ch.Producer()
			defer flush()
			canRun.Wait()
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
			canRun.Wait()
			for item, ok := consumer.Poll(); ok; item, ok = consumer.Poll() {
				result += item
			}
			counter.Add(int64(result))
		}()
	}

	b.ResetTimer()
	b.ReportAllocs()
	canRun.Done()

	producersWg.Wait()
	chCloser()
	consumersWg.Wait()
	b.StopTimer()

	expectedResult := int64(producersCnt * itemsPerProducer)
	if counter.Load() != expectedResult {
		b.Errorf("result is not as expected: %v != %v", counter.Load(), expectedResult)
	}
}
