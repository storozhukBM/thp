package thp

import (
	"context"
	"runtime"
	"sync"
)

const ErrChanBatchSize chanError = "Batch size for thp.Chan can't be lower than 1"

// Chan represents a concurrent channel with batching capability.
// It allows efficient batched communication between producers and consumers,
// reducing the overhead of individual item transfers.
//
// The channel operates in a concurrent manner, but each producer and consumer
// should be exclusively used by a single goroutine to ensure thread safety,
// so create separate Producer[T any] or Consumer[T any] for every goroutine
// that sends or receives messages.
// The producer is responsible for adding items to the channel's buffer
// and flushing them when the batch size is reached. The consumer
// retrieves items from the channel's buffer and processes them sequentially.
//
// The channel's batch size determines the number of items accumulated in the buffer
// before a flush operation is triggered. Adjusting the batch size can impact
// the trade-off between throughput and latency. Smaller batch sizes result in more
// frequent flushes and lower latency, while larger batch sizes increase throughput
// at the cost of higher latency.
// You can also manually trigger flushes.
//
// Context cancellation is supported via separate methods,
// allowing graceful termination of producers and consumers.
//
// The channel internally manages a sync.Pool to reuse batch buffers and avoid
// unnecessary allocations. This optimization improves performance by reducing
// memory allocations during batch creation and disposal.
type Chan[T any] struct {
	// the number of items to accumulate in the buffer
	// before triggering a flush operation to the internal channel.
	batchSize int
	// the internal channel used for communication between producers and consumers.
	internalChan chan *batch[T]
	// a sync.Pool used to reuse batch buffers and avoid unnecessary allocations.
	batchPool sync.Pool
}

// batch represents a batch of elements.
// We use a pointer to a batch instead of a regular slice to place it into sync.Pool.
// This helps avoid extra allocation when storing it as interface{}/any.
type batch[T any] struct {
	buf []T
}

// NewChan creates a new concurrent channel.
// batchSize specifies the number of elements to batch together before sending them.
// It returns a pointer to Chan[T] and a cleanup function to close the channel.
func NewChan[T any](batchSize int) (*Chan[T], func()) {
	if batchSize < 1 {
		panic(ErrChanBatchSize)
	}

	ch := &Chan[T]{
		batchSize:    batchSize,
		internalChan: make(chan *batch[T], runtime.NumCPU()),
		batchPool: sync.Pool{
			New: func() any {
				// We use a pointer to a batch instead of a regular slice to place it into sync.Pool.
				// This helps avoid extra allocation when storing it as interface{}/any.
				return &batch[T]{buf: make([]T, 0, batchSize)}
			},
		},
	}

	return ch, ch.Close
}

// Close closes the concurrent channel.
// Close panics on attempted close of already close Chan.
func (ch *Chan[T]) Close() {
	close(ch.internalChan)
}

// getBatchFromPool retrieves a batch from the sync.Pool.
// It returns a pointer to the batch.
// Note: It is assumed that this method is called exclusively by producers,
// and it should return initialised batch with
// cap(batch.buf) == ch.batchSize and len(batch.buf) == 0.
func (ch *Chan[T]) getBatchFromPool() *batch[T] {
	//nolint:forcetypeassert // Panic on type mismatch is fine here.
	return ch.batchPool.Get().(*batch[T])
}

// putBatchToPool returns a batch to the sync.Pool.
// It takes a pointer to the batch.
// Note: It is assumed that this method is called exclusively by consumers,
// when all batch items are consumed and/or not required anymore.
func (ch *Chan[T]) putBatchToPool(batch *batch[T]) {
	batch.buf = batch.buf[:0]
	ch.batchPool.Put(batch)
}

// Producer represents a producer for the concurrent channel.
// Each producer should be exclusively used by a single goroutine to ensure thread safety.
// Create separate Producer instance for every goroutine that sends messages.
type Producer[T any] struct {
	parent *Chan[T]
	batch  *batch[T]
	// We unpack the batch.buf into a separate field to avoid extra memory hop
	// every time we access it, this yields significant speedup on our tests.
	// But we still need to store pointer to a batch to avoid allocations when working with sync.Pool.
	buf []T
}

// Producer creates a producer for the concurrent channel.
// The producer is responsible for adding items to the channel's buffer and flushing
// them when the batch size is reached.
//
// Note: flush method should be called by the same goroutine that will use the producer.
//
// Example usage:
//
//	producer, flush := channel.Producer()
//	defer flush() // Ensure sending items through the channel
//	producer.Put(item1)
//	producer.Put(item2)
//
// Methods with provided `ctx` allows for graceful termination of the producer. If the
// context is canceled, the producer stops accepting new items, any remaining items stay in
// the buffer.
// WARNING: do not use returned flush method if you want context aware operations,
// use FlushCtx instead.
//
// Example of ctx aware operations usage:
//
//	producer, _ := channel.Producer()
//	defer producer.FlushCtx(ctx) // Ensure sending items through the channel
//	err := producer.PutCtx(ctx, item)
//	if err != nil {
//		return err
//	}
//
// Returns:
//   - producer: The created producer instance.
//   - flush: A function to send any remaining items.
func (ch *Chan[T]) Producer() (*Producer[T], func()) {
	initialBatch := ch.getBatchFromPool()
	result := &Producer[T]{
		parent: ch,
		batch:  initialBatch,
		buf:    initialBatch.buf,
	}
	return result, result.Flush
}

// NonBlockingFlush attempts to flush the items in the buffer to the channel without blocking.
// It returns true if the flush was successful, or false if the channel is full.
// In most cases you should use regular flush method provided to you upon Producer creation.
// Note: This method is intended to be used exclusively by a goroutine that owns this Producer.
func (p *Producer[T]) NonBlockingFlush() bool {
	if len(p.buf) == 0 {
		return false
	}
	p.batch.buf = p.buf
	select {
	case p.parent.internalChan <- p.batch:
		// Batch sent successfully, get a new batch from the pool for the next flush.
		p.batch = p.parent.getBatchFromPool()
		p.buf = p.batch.buf
		return true
	default:
		// Channel is full, unable to flush at the moment.
		return false
	}
}

// Flush flushes the items in the buffer to the channel, blocking if necessary.
// If the channel is full, it blocks until there is space available.
// Note: This method is intended to be used exclusively by a goroutine that owns this Producer.
func (p *Producer[T]) Flush() {
	if len(p.buf) == 0 {
		return
	}
	p.batch.buf = p.buf
	p.parent.internalChan <- p.batch
	// Batch sent successfully, get a new batch from the pool for the next flush.
	p.batch = p.parent.getBatchFromPool()
	p.buf = p.batch.buf
}

// FlushCtx flushes the items in the buffer to the channel, blocking if necessary.
// If the channel is full, it blocks until there is space available.
// It returns error if context gets canceled during flush operation.
// If the provided context is canceled, the remaining items stay in the buffer.
// Note: This method is intended to be used exclusively by a goroutine that owns this Producer.
func (p *Producer[T]) FlushCtx(ctx context.Context) error {
	ctxErr := ctx.Err()
	if ctxErr != nil {
		return ctxErr
	}
	if len(p.buf) == 0 {
		return nil
	}

	p.batch.buf = p.buf
	select {
	case p.parent.internalChan <- p.batch:
		// Batch sent successfully, get a new batch from the pool for the next flush.
		p.batch = p.parent.getBatchFromPool()
		p.buf = p.batch.buf
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// NonBlockingPut adds an item to the producer's buffer without blocking.
// If the buffer reaches the batchSize, it attempts a non-blocking flush.
// It returns true if the item was successfully added, or false if the channel is full.
// Note: This method is intended to be used exclusively by a goroutine that owns this Producer.
func (p *Producer[T]) NonBlockingPut(v T) bool {
	p.buf = append(p.buf, v)
	if len(p.buf) >= p.parent.batchSize {
		return p.NonBlockingFlush()
	}
	return true
}

// Put adds an item to the producer's buffer.
// If the buffer reaches the batchSize, it triggers a flush to the channel.
// Note: This method is intended to be used exclusively by a goroutine that owns this Producer.
func (p *Producer[T]) Put(v T) {
	p.buf = append(p.buf, v)
	if len(p.buf) >= p.parent.batchSize {
		p.Flush()
	}
}

// PutCtx adds an item to the producer's buffer.
// If the buffer reaches the batchSize, it triggers a flush to the channel.
// It returns error if context gets canceled during flush operation.
// Note: This method is intended to be used exclusively by a goroutine that owns this Producer.
func (p *Producer[T]) PutCtx(ctx context.Context, v T) error {
	p.buf = append(p.buf, v)
	if len(p.buf) >= p.parent.batchSize {
		return p.FlushCtx(ctx)
	}
	return nil
}

// Consumer represents a consumer for the concurrent channel.
// It retrieves items from the channel's buffer and processes them sequentially.
//
// The consumer operates in a concurrent manner, but it should be exclusively used
// by a single goroutine to ensure thread safety.
// Create separate Producer instance for every goroutine that sends messages.
//
// The consumer retrieves items by calling the Poll method,
// which returns the next item from the buffer. If the
// buffer is empty, the consumer will prefetch the next batch of items from the
// internal channel to ensure a continuous supply.
//
// The consumer supports both blocking and non-blocking operations.
//
// Context cancellation is supported via separate method,
// allowing graceful termination of the consumer.
// When the consumer's context is canceled, it stops fetching new batches from the
// internal channel and signals the end of consumption.
type Consumer[T any] struct {
	parent *Chan[T]
	idx    int
	batch  *batch[T]
	// We unpack the batch.buf into a separate field to avoid extra memory hop
	// every time we access it, this yields significant speedup on our tests.
	// But we still need to store pointer to a batch to avoid allocations when working with sync.Pool.
	buf []T
}

// Consumer creates a consumer for the concurrent channel with the given context.
// The consumer is responsible for retrieving items from the channel's buffer and
// processing them sequentially.
//
// Note: This method should be called by the same goroutine that will use the consumer.
//
// Example usage:
//
//	consumer := channel.Consumer()
//	for {
//	    item, ok := consumer.Poll()
//	    if !ok {
//	        break
//	    }
//	    // Process the item
//	}
//
// Returns:
//   - consumer: The created consumer instance.
func (ch *Chan[T]) Consumer() *Consumer[T] {
	result := &Consumer[T]{
		parent: ch,
		idx:    0,
		batch:  &batch[T]{},
		buf:    nil,
	}
	return result
}

// nonBlockingPrefetch attempts to prefetch the next batch of items from the internal channel
// in a non-blocking manner, ensuring a continuous supply of items for consumption.
// Returns:
//   - readSuccess: A boolean indicating whether a new batch was successfully fetched.
//   - channelIsOpen: A boolean indicating whether the internal channel is still open for
//     further consumption. If false, no more batches will be available.
//
//nolint:nonamedreturns // here we usenamesreturns to documents meaning of two returned booleans
func (c *Consumer[T]) nonBlockingPrefetch() (readSuccess bool, channelIsOpen bool) {
	if cap(c.buf) > 0 {
		c.parent.putBatchToPool(c.batch)
	}

	c.idx = 0
	c.batch = nil
	c.buf = nil

	select {
	case batch, ok := <-c.parent.internalChan:
		c.batch = batch
		if batch != nil {
			c.buf = batch.buf
		}
		return ok, ok
	default:
		return false, true
	}
}

// prefetch fetches the next batch from the channel and prepares the consumer for reading.
// It returns true if a new batch is fetched successfully,
// or false if the channel is closed.
func (c *Consumer[T]) prefetch() bool {
	if cap(c.buf) > 0 {
		// Return the current batch to the pool.
		c.parent.putBatchToPool(c.batch)
	}

	c.idx = 0
	c.batch = nil
	c.buf = nil

	batch, ok := <-c.parent.internalChan
	c.batch = batch
	if batch != nil {
		c.buf = batch.buf
	}
	return ok
}

// prefetchCtx fetches the next batch from the channel and prepares the consumer for reading.
// It returns (true, nil) if a new batch is fetched successfully,
// or (false, nil) if the channel is closed
// or (false, error) if the context is canceled.
func (c *Consumer[T]) prefetchCtx(ctx context.Context) (bool, error) {
	if cap(c.buf) > 0 {
		// Return the current batch to the pool.
		c.parent.putBatchToPool(c.batch)
	}

	c.idx = 0
	c.batch = nil
	c.buf = nil

	select {
	case batch, ok := <-c.parent.internalChan:
		c.batch = batch
		if batch != nil {
			c.buf = batch.buf
		}
		return ok, nil
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

// NonBlockingPoll retrieves the next item from the consumer's buffer in a non-blocking manner.
// It returns the item, a boolean indicating whether the retrieval was successful, and a boolean
// indicating whether the internal channel is still open for further consumption.
//
// Note: This method is intended to be used exclusively by a goroutine that owns this Consumer.
//
//nolint:nonamedreturns // here we usenamesreturns to documents meaning of two last returned booleans
func (c *Consumer[T]) NonBlockingPoll() (value T, readSuccess bool, channelIsOpen bool) {
	if c.idx >= len(c.buf) {
		success, open := c.nonBlockingPrefetch()
		if !success {
			return zero[T](), success, open
		}
	}
	item := c.buf[c.idx]
	c.idx++
	return item, true, true
}

// Poll retrieves the next item from the consumer's buffer.
// It returns the item and true if successful, or a zero value and false if there are no more items.
// Note: This method is intended to be used exclusively by a goroutine that owns this Consumer.
func (c *Consumer[T]) Poll() (T, bool) {
	if c.idx >= len(c.buf) {
		ok := c.prefetch()
		if !ok {
			return zero[T](), false
		}
	}
	item := c.buf[c.idx]
	c.idx++
	return item, true
}

// PollCtx retrieves the next item from the consumer's buffer.
// It returns the item and true if successful,
// or a (zero value, false, nil) if there are no more items
// or a (zero value, false, error) if context is canceled.
// Note: This method is intended to be used exclusively by a goroutine that owns this Consumer.
func (c *Consumer[T]) PollCtx(ctx context.Context) (T, bool, error) {
	if c.idx >= len(c.buf) {
		ok, err := c.prefetchCtx(ctx)
		if err != nil {
			return zero[T](), false, err
		}
		if !ok {
			return zero[T](), false, nil
		}
	}
	item := c.buf[c.idx]
	c.idx++
	return item, true, nil
}

type chanError string

func (e chanError) Error() string {
	return string(e)
}
