package thp_test

import (
	"context"
	"fmt"
	"github.com/storozhukBM/thp"
	"sync"
	"sync/atomic"
	"testing"
)

func BenchmarkChanThroughput(b *testing.B) {
    for bufSize := 1; bufSize <= 1024; bufSize *= 2 {
        b.Run(fmt.Sprintf("type:%s;pCnt:%d;cCnt:%d;buf:%d", "standard", 8, 8, bufSize), func(b *testing.B) {
            runStandardChan(b, 8, 8, bufSize)
        })
    }
    for bufSize := 1; bufSize <= 1024; bufSize *= 2 {
		b.Run(fmt.Sprintf("type:%s;pCnt:%d;cCnt:%d;buf:%d", "thp", 8, 8, bufSize), func(b *testing.B) {
			runThpChan(b, 8, 8, bufSize)
		})
	}
}

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
			producer, flush := ch.Producer(context.Background())
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
			consumer := ch.Consumer(context.Background())
			result := 0
			canRun.Wait()
			item, ok := consumer.Poll()
			for ; ok; item, ok = consumer.Poll() {
				result += item
			}
			counter.Add(int64(result))
		}()
	}

	b.ResetTimer()
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
