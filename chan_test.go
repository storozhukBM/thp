package thp_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/storozhukBM/thp"
)

func TestChanPrimitives(t *testing.T) {
	ch := thp.NewChan[int]()
	consumer, consumerClose := ch.Consumer(context.Background())
	defer consumerClose()
	val, _ := consumer.Poll()
	fmt.Printf("val: %T: `%+v`\n", val, val)
}

func TestChanObj(t *testing.T) {
	ch := thp.NewChan[*int]()
	consumer, consumerClose := ch.Consumer(context.Background())
	defer consumerClose()
	val, _ := consumer.Poll()
	fmt.Printf("val: %T: `%+v`\n", val, val)
}
