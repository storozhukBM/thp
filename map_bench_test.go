package thp

import (
	"fmt"
	"hash/maphash"
	"math"
	"math/rand"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"unsafe"
)

type maplike[K comparable, V any] interface {
	Put(key K, value V)
	Get(key K) (V, bool)
}

func BenchmarkMap(b *testing.B) {
	runBenchForOperation(b)
}

func runBenchForOperation(b *testing.B) {
	sizes := []int{
		32,
		//512,
		100_000,
	}
	mutators := []int{1, 2, 4, 6, 7}
	implementations := []struct {
		name        string
		constructor func() maplike[string, int]
	}{
		{
			name: "StripedMuMap",
			constructor: func() maplike[string, int] {
				return newStripedMuMap[string, int]()
			},
		},
		{
			name: "StripedRWMap",
			constructor: func() maplike[string, int] {
				return newStripedRWMap[string, int]()
			},
		},
		{
			name: "SyncMap",
			constructor: func() maplike[string, int] {
				return newSyncMap[string, int]()
			},
		},
	}
	for _, size := range sizes {
		for _, mutatorsCount := range mutators {
			readersCount := 8 - mutatorsCount
			mutatorsPct := int(math.Round(float64(mutatorsCount) / 8.0 * 100.0))
			for _, impl := range implementations {
				b.Run(fmt.Sprintf("type:%s;mapSize:%d;mutations%%:%d", impl.name+"Read", size, mutatorsPct), func(b *testing.B) {
					target := impl.constructor()
					benchmapReadStr(b, target, size, mutatorsCount, readersCount)
				})
				b.Run(fmt.Sprintf("type:%s;mapSize:%d;mutations%%:%d", impl.name+"Write", size, mutatorsPct), func(b *testing.B) {
					target := impl.constructor()
					benchmapStoreStr(b, target, size, mutatorsCount, readersCount)
				})
			}
		}
	}
}

func benchmapReadStr(b *testing.B, target maplike[string, int], size int, mutators int, readers int) {
	activeKeySetSize := size
	keySet := make([]string, 0, activeKeySetSize)
	for i := 0; i < activeKeySetSize; i++ {
		keySet = append(keySet, strconv.Itoa(rand.Int()))
	}

	mutatorsStop := atomic.Bool{}
	defer mutatorsStop.Store(true)

	canRun := &sync.WaitGroup{}
	canRun.Add(1)

	for i := 0; i < mutators; i++ {
		value := i
		go func() {
			canRun.Wait()

			for !mutatorsStop.Load() {
				for _, key := range keySet {
					target.Put(key, value)
				}
			}
		}()
	}

	wg := &sync.WaitGroup{}
	wg.Add(readers)

	readersBlackHole := atomic.Int64{}

	readsPerReader := b.N / readers
	for i := 0; i < readers; i++ {
		go func() {
			defer wg.Done()

			var blackHole = 0
			keyIdx := rand.Intn(activeKeySetSize)

			canRun.Wait()

			for j := 0; j < readsPerReader; j++ {
				value, _ := target.Get(keySet[keyIdx])
				blackHole += value
				keyIdx++
				if keyIdx >= activeKeySetSize {
					keyIdx = 0
				}
			}
			readersBlackHole.Add(int64(blackHole))
		}()
	}

	b.ResetTimer()
	b.ReportAllocs()
	canRun.Done()

	wg.Wait()
	b.StopTimer()

	almostNever := rand.Float64() == 0.0
	if almostNever {
		fmt.Printf("Blackhole:%v\n", readersBlackHole.Load())
	}
}

func benchmapStoreStr(b *testing.B, target maplike[string, int], size int, mutators int, readers int) {
	activeKeySetSize := size
	keySet := make([]string, 0, activeKeySetSize)
	for i := 0; i < activeKeySetSize; i++ {
		keySet = append(keySet, strconv.Itoa(rand.Int()))
	}

	canRun := &sync.WaitGroup{}
	canRun.Add(1)

	readersStop := atomic.Bool{}
	defer readersStop.Store(true)

	readersBlackHole := atomic.Int64{}
	for i := 0; i < readers; i++ {
		go func() {
			var blackHole = 0
			keyIdx := rand.Intn(activeKeySetSize)
			canRun.Wait()

			for !readersStop.Load() {
				value, _ := target.Get(keySet[keyIdx])
				blackHole += value
				keyIdx++
				if keyIdx >= activeKeySetSize {
					keyIdx = 0
				}
			}
			readersBlackHole.Add(int64(blackHole))
		}()
	}

	wg := &sync.WaitGroup{}
	wg.Add(mutators)

	mutationsPerMutator := b.N / mutators
	for i := 0; i < mutators; i++ {
		go func() {
			defer wg.Done()

			keyIdx := rand.Intn(activeKeySetSize)

			canRun.Wait()

			for j := 0; j < mutationsPerMutator; j++ {
				target.Put(keySet[keyIdx], j)
				keyIdx++
				if keyIdx >= activeKeySetSize {
					keyIdx = 0
				}
			}
		}()
	}

	b.ResetTimer()
	b.ReportAllocs()
	canRun.Done()

	wg.Wait()
	b.StopTimer()

	almostNever := rand.Float64() == 0.0
	if almostNever {
		fmt.Printf("Blackhole:%v\n", readersBlackHole.Load())
	}
}

type syncMap[K comparable, V any] struct {
	m sync.Map
}

func (s *syncMap[K, V]) Put(key K, value V) {
	s.m.Store(key, value)
}

func (s *syncMap[K, V]) Get(key K) (V, bool) {
	v, ok := s.m.Load(key)
	if ok {
		return v.(V), ok
	}
	return zero[V](), false
}

func newSyncMap[K comparable, V any]() *syncMap[K, V] {
	return &syncMap[K, V]{
		m: sync.Map{},
	}
}

type stdRWMap[K comparable, V any] struct {
	rwmu sync.RWMutex
	m    map[K]V
}

func (s *stdRWMap[K, V]) Put(key K, value V) {
	s.rwmu.Lock()
	defer s.rwmu.Unlock()

	s.m[key] = value
}

func (s *stdRWMap[K, V]) Get(key K) (V, bool) {
	s.rwmu.RLock()
	defer s.rwmu.RUnlock()

	v, ok := s.m[key]
	return v, ok
}

func newStdRWMap[K comparable, V any]() *stdRWMap[K, V] {
	return &stdRWMap[K, V]{
		m: make(map[K]V),
	}
}

type stdMuMap[K comparable, V any] struct {
	mu sync.Mutex
	m  map[K]V
}

func (s *stdMuMap[K, V]) Put(key K, value V) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.m[key] = value
}

func (s *stdMuMap[K, V]) Get(key K) (V, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	v, ok := s.m[key]
	return v, ok
}

func newStdMuMap[K comparable, V any]() *stdMuMap[K, V] {
	return &stdMuMap[K, V]{
		m: make(map[K]V),
	}
}

type singleMuStripe[K comparable, V any] struct {
	_  [cacheLineSize - 16]byte
	mu sync.Mutex
	m  map[K]V
}

type stripedMuMap[K comparable, V any] struct {
	keyIsString bool
	keyTypeSize int
	seed        maphash.Seed
	stripe      []*singleMuStripe[K, V]
}

func (s *stripedMuMap[K, V]) Put(key K, value V) {
	hash := s.hash(key)
	idx := (len(s.stripe) - 1) & int(hash)
	individualStripe := s.stripe[idx]

	individualStripe.mu.Lock()
	defer individualStripe.mu.Unlock()

	individualStripe.m[key] = value
}

func (s *stripedMuMap[K, V]) Get(key K) (V, bool) {
	hash := s.hash(key)
	idx := (len(s.stripe) - 1) & int(hash)
	individualStripe := s.stripe[idx]

	individualStripe.mu.Lock()
	defer individualStripe.mu.Unlock()

	v, ok := individualStripe.m[key]
	return v, ok
}

func (s *stripedMuMap[K, V]) hash(key K) uint64 {
	var strKey string
	if s.keyIsString {
		strKey = *(*string)(unsafe.Pointer(&key))
	} else {
		strKey = *(*string)(unsafe.Pointer(&struct {
			data unsafe.Pointer
			len  int
		}{unsafe.Pointer(&key), s.keyTypeSize}))
	}
	// Now for the actual hashing.
	return maphash.String(s.seed, strKey)
}

func newStripedMuMap[K comparable, V any]() *stripedMuMap[K, V] {
	numStripes := int(nextHighestPowerOf2(int32(runtime.NumCPU())))
	stripe := make([]*singleMuStripe[K, V], numStripes)
	for i := 0; i < numStripes; i++ {
		stripe[i] = &singleMuStripe[K, V]{
			m: make(map[K]V),
		}
	}
	result := &stripedMuMap[K, V]{
		seed:   maphash.MakeSeed(),
		stripe: stripe,
	}

	var k K
	switch ((interface{})(k)).(type) {
	case string:
		result.keyIsString = true
	default:
		result.keyTypeSize = int(unsafe.Sizeof(k))
	}
	return result
}

type singleRWStripe[K comparable, V any] struct {
	_  [cacheLineSize - 32]byte
	mu sync.RWMutex
	m  map[K]V
}

type stripedRWMap[K comparable, V any] struct {
	keyIsString bool
	keyTypeSize int
	seed        maphash.Seed
	stripe      []*singleRWStripe[K, V]
}

func (s *stripedRWMap[K, V]) Put(key K, value V) {
	hash := s.hash(key)
	idx := (len(s.stripe) - 1) & int(hash)
	individualStripe := s.stripe[idx]

	individualStripe.mu.Lock()
	defer individualStripe.mu.Unlock()

	individualStripe.m[key] = value
}

func (s *stripedRWMap[K, V]) Get(key K) (V, bool) {
	hash := s.hash(key)
	idx := (len(s.stripe) - 1) & int(hash)
	individualStripe := s.stripe[idx]

	individualStripe.mu.RLock()
	defer individualStripe.mu.RUnlock()

	v, ok := individualStripe.m[key]
	return v, ok
}

func (s *stripedRWMap[K, V]) hash(key K) uint64 {
	var strKey string
	if s.keyIsString {
		strKey = *(*string)(unsafe.Pointer(&key))
	} else {
		strKey = *(*string)(unsafe.Pointer(&struct {
			data unsafe.Pointer
			len  int
		}{unsafe.Pointer(&key), s.keyTypeSize}))
	}
	// Now for the actual hashing.
	return maphash.String(s.seed, strKey)
}

func newStripedRWMap[K comparable, V any]() *stripedRWMap[K, V] {
	numStripes := int(nextHighestPowerOf2(int32(runtime.NumCPU())))
	stripe := make([]*singleRWStripe[K, V], numStripes)
	for i := 0; i < numStripes; i++ {
		stripe[i] = &singleRWStripe[K, V]{
			m: make(map[K]V),
		}
	}
	result := &stripedRWMap[K, V]{
		seed:   maphash.MakeSeed(),
		stripe: stripe,
	}

	var k K
	switch ((interface{})(k)).(type) {
	case string:
		result.keyIsString = true
	default:
		result.keyTypeSize = int(unsafe.Sizeof(k))
	}
	return result
}
