package thp

import (
	"context"
)

type Chan[T any] struct{}

func NewChan[T any]() *Chan[T] {
	return &Chan[T]{}
}

func (ch *Chan[T]) Producer(ctx context.Context) (*Producer[T], func()) {
	result := &Producer[T]{ctx: ctx}
	return result, result.Flush
}

type Producer[T any] struct {
	ctx context.Context
}

func (p *Producer[T]) Flush() {
}

func (p *Producer[T]) Put(v T) {
}

func (ch *Chan[T]) Consumer(ctx context.Context) (*Consumer[T], func()) {
	result := &Consumer[T]{ctx: ctx}
	return result, result.ReturnUnconsumed
}

type Consumer[T any] struct {
	ctx context.Context
}

func (c *Consumer[T]) ReturnUnconsumed() {
	// check buffered messages and schedule
	// separate goroutine to re-submit them if some are unconsumed
}

func (c *Consumer[T]) Poll() (T, bool) {
	return zero[T](), false
}

func (c *Consumer[T]) NonBlockingPoll() (T, bool) {
	return zero[T](), false
}

func zero[T any]() T {
	return *new(T)
}
