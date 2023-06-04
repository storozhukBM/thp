package thp

import (
	"math/rand"
	"sync"
	"time"
)

type cacheLinePadding [64]byte

func zero[T any]() T {
	return *new(T)
}

func ptr[T any](v T) *T {
	return &v
}

type pid *int32

var _processId = sync.Pool{New: func() any {
	rnd := getRandom()
	defer putRandom(rnd)
	return pid(ptr(rnd.Int31()))
}}

func getProcessId() pid {
	return _processId.Get().(pid)
}

func putProcessId(id pid) {
	_processId.Put(id)
}

var _counterRandoms = sync.Pool{New: func() any {
	return rand.New(rand.NewSource(time.Now().UnixNano()))
}}

func getRandom() *rand.Rand {
	return _counterRandoms.Get().(*rand.Rand)
}

func putRandom(rnd *rand.Rand) {
	_counterRandoms.Put(rnd)
}
