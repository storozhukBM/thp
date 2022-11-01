package thp_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/storozhukBM/thp"
)

func TestChan(t *testing.T) {
	ch, chCloser := thp.NewChan[int](32)

	producersCnt := 8
	itemsPerProducer := 100
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

	consumersCnt := 8
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
	fmt.Printf("Produsers finished\n")
	consumersWg.Wait()
	fmt.Printf("Consumers finished: %v\n", counter.Load())

	expectedResult := int64(producersCnt * itemsPerProducer)
	if counter.Load() != expectedResult {
		t.Errorf("result is not as expected: %v != %v", counter.Load(), expectedResult)
	}
}
