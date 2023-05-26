# **thp** - High throughput primitives library
[![Go Reference](https://pkg.go.dev/badge/github.com/storozhukBM/thp.svg)](https://pkg.go.dev/github.com/storozhukBM/thp)
![Build](https://github.com/storozhukBM/thp/actions/workflows/go.yml/badge.svg) 
[![Go Report Card](https://goreportcard.com/badge/github.com/storozhukBM/thp)](https://goreportcard.com/report/github.com/storozhukBM/thp) 
[![Coverage Status](https://coveralls.io/repos/github/storozhukBM/thp/badge.svg)](https://coveralls.io/github/storozhukBM/thp)

## **thp.Chan[T any]**

**Chan** represents a concurrent channel with batching capability.
It allows efficient batched communication between producers and consumers,
reducing the overhead of individual item transfers.

The channel operates in a concurrent manner, but each producer and consumer
should be exclusively used by a single goroutine to ensure thread safety,
so create separate **Producer[T any]** or **Consumer[T any]** for every goroutine
that sends or receives messages.
The producer is responsible for adding items to the channel's buffer
and flushing them when the batch size is reached. The consumer
retrieves items from the channel's buffer and processes them sequentially.

The channel's batch size determines the number of items accumulated in the buffer
before a flush operation is triggered. Adjusting the batch size can impact
the trade-off between throughput and latency. Smaller batch sizes result in more
frequent flushes and lower latency, while larger batch sizes increase throughput
at the cost of higher latency.
You can also manually trigger flushes.

The channel internally manages a sync.Pool to reuse batch buffers and avoid
unnecessary allocations. This optimization improves performance by reducing
memory allocations during batch creation and disposal.

### Example with comparison to built-in channel:

<table>
<tr>
<th>Built-in channel</th>
<th>thp.Chan</th>
</tr>
<tr>
<td>

```go
 

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

expectedResult := int64(
  producersCount * itemsPerProducer
)
if counter.Load() != expectedResult {
  t.Errorf(
    "result is not as expected: %v != %v",
    counter.Load(), expectedResult,
  )
}
```

</td>
<td>

```go
ctx := context.Background()

ch, chCloser := thp.NewChan[int](1024)
producersWg := &sync.WaitGroup{}
producersCount := 16
itemsPerProducer := 1_000_000
producersWg.Add(producersCount)

for i := 0; i < producersCount; i++ {
  go func() {
    defer producersWg.Done()
    producer, flush := ch.Producer(ctx)
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
    consumer := ch.Consumer(ctx)
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

expectedResult := int64(
  producersCount * itemsPerProducer
)
if counter.Load() != expectedResult {
  t.Errorf(
    "result is not as expected: %v != %v", 
    counter.Load(), expectedResult,
  )
}
```

</td>
</tr>
</table>

### Performance

Run `make bench` to get results on your machine.

<img width="1043" alt="Benchmark results" src="https://github.com/storozhukBM/thp/assets/3532750/c327cfe0-3435-4fb0-98f2-8ccf0d401a33">
