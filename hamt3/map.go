package hamt3

import (
	"fmt"
	"github.com/zeebo/xxh3"
	"math/bits"
	"math/rand"
	"strings"
	"sync"
	"time"
	"unsafe"
)

const hashPartMask = uint64((1 << 6) - 1)
const leafCellsSize = 16

type treeNode[K comparable, V any] struct {
	isLeaf byte
}

type branchSlice struct {
	b     []unsafe.Pointer
	fines int
}

type branchNode[K comparable, V any] struct {
	treeNode[K, V]
	level    uint8
	mask     uint64
	branches *branchSlice
}

//
//type leafNode[K comparable, V any] struct {
//	isLeaf                      byte
//	size                        byte   // 15 max
//	maskOfBranchesWithCollision uint16 // if bit is set branch contains a *[]pair
//	branchHashes                [15]uint32
//	// End of 1st cache line
//
//	cells [15]unsafe.Pointer // union(*pair,*[]pair)
//}

type pair[K comparable, V any] struct {
	key   K
	value V
	next  *pair[K, V]
}

type leafNode[K comparable, V any] struct {
	treeNode[K, V]
	cells [leafCellsSize]*pair[K, V]
}

type Map[K comparable, V any] struct {
	seed    uint64
	keySize int
	root    unsafe.Pointer // *leafNode[K, V] | *branchNode[K, V]

	headAllocator        *sync.Pool
	leafAllocator        *sync.Pool
	pairAllocator        *sync.Pool
	branchAllocator      *sync.Pool
	branchSliceAllocator [3]*sync.Pool
}

func NewMap[K comparable, V any]() *Map[K, V] {
	var keySize int
	zeroKey := zero[K]()
	switch ((any)(zeroKey)).(type) {
	case string:
		keySize = -1 // negative means string
	default:
		keySize = int(unsafe.Sizeof(zeroKey))
	}

	newMap := &Map[K, V]{
		seed:    rand.Uint64(),
		keySize: keySize,
		root:    nil,

		headAllocator: &sync.Pool{New: func() any {
			return &Map[K, V]{}
		}},
		leafAllocator: &sync.Pool{New: func() any {
			return &leafNode[K, V]{
				treeNode: treeNode[K, V]{
					isLeaf: 1,
				},
				cells: [16]*pair[K, V]{},
			}
		}},
		pairAllocator: &sync.Pool{New: func() any {
			return &pair[K, V]{}
		}},
		branchAllocator: &sync.Pool{New: func() any {
			return &branchNode[K, V]{
				treeNode: treeNode[K, V]{
					isLeaf: 0,
				},
				level:    0,
				mask:     0,
				branches: nil,
			}
		}},

		branchSliceAllocator: [3]*sync.Pool{
			{New: func() any {
				return &branchSlice{b: make([]unsafe.Pointer, 0, 16), fines: 0}
			}},
			{New: func() any {
				return &branchSlice{b: make([]unsafe.Pointer, 0, 32), fines: 0}
			}},
			{New: func() any {
				return &branchSlice{b: make([]unsafe.Pointer, 0, 64), fines: 0}
			}},
		},
	}
	newMap.root = unsafe.Pointer(newMap.allocLeaf())
	return newMap
}

func (m *Map[K, V]) allocLeaf() *leafNode[K, V] {
	leaf := m.leafAllocator.Get().(*leafNode[K, V])
	//runtime.SetFinalizer(
	//	leaf,
	//	func(l any) {
	//		runInPool(func() {
	//			// we make compost here
	//			deadLeaf := l.(*leafNode[K, V])
	//			defer m.leafAllocator.Put(deadLeaf)
	//			// this pattern will be recognized by compiler and optimized
	//			for i := range deadLeaf.cells {
	//				deadLeaf.cells[i] = nil
	//			}
	//
	//			//for i := range deadLeaf.cells {
	//			//	p := deadLeaf.cells[i]
	//			//	for p != nil {
	//			//		deadPair := p
	//			//		p = deadPair.next
	//			//
	//			//		deadPair.key = zero[K]()
	//			//		deadPair.value = zero[V]()
	//			//		deadPair.next = nil
	//			//		m.pairAllocator.Put(deadPair)
	//			//	}
	//			//	deadLeaf.cells[i] = nil
	//			//}
	//		})
	//	},
	//)
	return leaf
}

func (m *Map[K, V]) allocPair() *pair[K, V] {
	p := m.pairAllocator.Get().(*pair[K, V])
	return p
}

func (m *Map[K, V]) allocHead() *Map[K, V] {
	head := m.headAllocator.Get().(*Map[K, V])
	//runtime.SetFinalizer(head, func(h any) {
	//	runInPool(func() {
	//		deadHead := h.(*Map[K, V])
	//		defer m.headAllocator.Put(deadHead)
	//
	//		deadHead.seed = 0
	//		deadHead.keySize = 0
	//		deadHead.root = nil
	//
	//		deadHead.headAllocator = nil
	//		deadHead.leafAllocator = nil
	//		deadHead.pairAllocator = nil
	//		deadHead.branchAllocator = nil
	//		deadHead.branchSliceAllocator[0] = nil
	//		deadHead.branchSliceAllocator[1] = nil
	//		deadHead.branchSliceAllocator[2] = nil
	//	})
	//})
	return head
}

func (m *Map[K, V]) allocBranch() *branchNode[K, V] {
	branch := m.branchAllocator.Get().(*branchNode[K, V])
	//runtime.SetFinalizer(branch, func(b any) {
	//	runInPool(func() {
	//		deadBranch := b.(*branchNode[K, V])
	//		defer m.branchAllocator.Put(deadBranch)
	//
	//		deadBranch.level = 0
	//		deadBranch.mask = 0
	//		deadBranch.branches = nil
	//	})
	//})
	return branch
}

func (m *Map[K, V]) allocBranchSlice(capacity int) *branchSlice {
	var targetPool *sync.Pool
	switch capacity {
	case 16:
		targetPool = m.branchSliceAllocator[0]
	case 32:
		targetPool = m.branchSliceAllocator[1]
	case 64:
		targetPool = m.branchSliceAllocator[2]
	default:
		panic(fmt.Sprintf("unsupported branch slice capacity: %v", capacity))
	}

	bSlice := targetPool.Get().(*branchSlice)
	//runtime.SetFinalizer(bSlice, func(bs any) {
	//	runInPool(func() {
	//		deadBranchSlice := bs.(*branchSlice)
	//		occupation := 0
	//		for _, ptr := range deadBranchSlice.b {
	//			if ptr != nil {
	//				occupation++
	//			}
	//		}
	//		if occupation < cap(deadBranchSlice.b)/2 {
	//			deadBranchSlice.fines++
	//		} else {
	//			deadBranchSlice.fines = 0
	//		}
	//		if deadBranchSlice.fines <= 3 {
	//			deadBranchSlice.b = deadBranchSlice.b[:0]
	//			targetPool.Put(deadBranchSlice)
	//		}
	//	})
	//})
	return bSlice
}

func (m *Map[K, V]) copyLeaf(l *leafNode[K, V]) *leafNode[K, V] {
	newLeaf := m.allocLeaf()
	copy(newLeaf.cells[:], l.cells[:])
	return newLeaf
}

func (m *Map[K, V]) copyBranch(b *branchNode[K, V]) *branchNode[K, V] {
	result := m.allocBranch()
	result.level = b.level
	result.mask = b.mask
	if len(b.branches.b) > 0 {
		result.branches = m.allocBranchSlice(cap(b.branches.b))
		result.branches.b = append(result.branches.b, b.branches.b...)
	}
	return result
}

func (m *Map[K, V]) Load(key K) (V, bool) {
	keyHash := m.hash(key)
	node := (*treeNode[K, V])(m.root)
	for node.isLeaf == 0 {
		branch := (*branchNode[K, V])(unsafe.Pointer(node))
		bitIdx := (keyHash >> (6 * branch.level)) & hashPartMask
		branchIdx := bits.OnesCount64(branch.mask & ((1 << bitIdx) - 1))
		if branch.mask&(1<<bitIdx) == 0 {
			return zero[V](), false
		}
		node = (*treeNode[K, V])(branch.branches.b[branchIdx])
	}

	leafPointer := (*leafNode[K, V])(unsafe.Pointer(node))
	idx := keyHash >> 60
	targetChain := leafPointer.cells[idx]
	for targetChain != nil {
		if targetChain.key == key {
			return targetChain.value, true
		}
		targetChain = targetChain.next
	}
	return zero[V](), false
}

func (m *Map[K, V]) Store(key K, value V) *Map[K, V] {
	depth := 0
	keyHash := m.hash(key)
	result := m.allocHead()
	result.seed = m.seed
	result.keySize = m.keySize
	result.root = nil
	result.headAllocator = m.headAllocator
	result.leafAllocator = m.leafAllocator
	result.pairAllocator = m.pairAllocator
	result.branchAllocator = m.branchAllocator
	result.branchSliceAllocator = m.branchSliceAllocator

	// pointer to pointer. I officially hate this, but it looks like
	// something I need to suffer through if I want to avoid recursive calls
	pointerChainPointer := &result.root
	node := (*treeNode[K, V])(m.root)
	for node.isLeaf == 0 {
		branch := m.copyBranch((*branchNode[K, V])(unsafe.Pointer(node)))
		bitIdx := (keyHash >> (6 * branch.level)) & hashPartMask
		branchIdx := bits.OnesCount64(branch.mask & ((1 << bitIdx) - 1))
		if branch.mask&(1<<bitIdx) == 0 {
			// this branch is not initialized
			// so we init it
			branch.mask = branch.mask | 1<<bitIdx
			m.insertEmptyLeaf(branch, branchIdx)
		}

		*pointerChainPointer = unsafe.Pointer(branch)
		pointerChainPointer = &branch.branches.b[branchIdx]
		node = (*treeNode[K, V])(branch.branches.b[branchIdx])
		depth++
	}

	leafPointer := (*leafNode[K, V])(unsafe.Pointer(node))
	newLeaf := m.storeToLeaf(keyHash, key, value, leafPointer)
	*pointerChainPointer = unsafe.Pointer(newLeaf)
	if depth != 10 {
		counter := 0
		for _, ptr := range leafPointer.cells {
			if ptr != nil {
				counter++
			}
		}
		if counter > len(newLeaf.cells)/2 {
			*pointerChainPointer = unsafe.Pointer(m.splitLeafIntoBranches(depth, newLeaf))
		}
	}
	return result
}

func (m *Map[K, V]) insertEmptyLeaf(branch *branchNode[K, V], idx int) {
	if len(branch.branches.b)+1 <= cap(branch.branches.b) {
		if len(branch.branches.b) == idx {
			branch.branches.b = append(branch.branches.b, unsafe.Pointer(m.allocLeaf()))
			return
		}
		branch.branches.b = append(branch.branches.b[:idx+1], branch.branches.b[idx:]...)
		branch.branches.b[idx] = unsafe.Pointer(m.allocLeaf())
		return
	}
	// current capacity is not enough
	targetCapacity := 16
	if cap(branch.branches.b)*2 > targetCapacity {
		targetCapacity = cap(branch.branches.b) * 2
	}
	newBranchSlice := m.allocBranchSlice(targetCapacity)
	newBranchSlice.b = append(newBranchSlice.b, branch.branches.b[:idx]...)
	newBranchSlice.b = append(newBranchSlice.b, unsafe.Pointer(m.allocLeaf()))
	newBranchSlice.b = append(newBranchSlice.b, branch.branches.b[idx:]...)
	branch.branches = newBranchSlice
}

func (m *Map[K, V]) splitLeafIntoBranches(level int, newLeaf *leafNode[K, V]) *branchNode[K, V] {
	branch := m.allocBranch()
	branch.level = byte(level + 1)
	for i := 0; i < len(newLeaf.cells); i++ {
		cell := newLeaf.cells[i]
		if cell == nil {
			continue
		}
		for cell != nil {
			nodeHash := m.hash(cell.key)
			bitIdx := (nodeHash >> int64(6*branch.level)) & hashPartMask
			branch.mask = branch.mask | (1 << bitIdx)
			branchSliceIdx := bits.OnesCount64(branch.mask) - 1
			if branch.branches == nil {
				branch.branches = m.allocBranchSlice(16)
				branch.branches.b = append(branch.branches.b, unsafe.Pointer(m.allocLeaf()))
			}
			if branchSliceIdx >= len(branch.branches.b) {
				if len(branch.branches.b)+1 > cap(branch.branches.b) {
					targetCapacity := cap(branch.branches.b) * 2
					newBranchSlice := m.allocBranchSlice(targetCapacity)
					newBranchSlice.b = append(newBranchSlice.b, branch.branches.b...)
					branch.branches = newBranchSlice
				}
				branch.branches.b = append(branch.branches.b, unsafe.Pointer(m.allocLeaf()))
			}
			idxLeaf := (*leafNode[K, V])(branch.branches.b[branchSliceIdx])
			branch.branches.b[branchSliceIdx] = unsafe.Pointer(
				m.storeToLeaf(nodeHash, cell.key, cell.value, idxLeaf),
			)
			cell = cell.next
		}
	}
	return branch
}

func (m *Map[K, V]) storeToLeaf(keyHash uint64, key K, value V, leafPointer *leafNode[K, V]) *leafNode[K, V] {
	leaf := m.copyLeaf(leafPointer)
	idx := keyHash >> 60 // last 4 bits

	if leaf.cells[idx] == nil {
		p := m.allocPair()
		p.key = key
		p.value = value
		leaf.cells[idx] = p
	} else {
		oldChainPointer := leaf.cells[idx]
		p := m.allocPair()
		p.key = key
		p.value = value
		p.next = oldChainPointer
		newChainPointer := p
		leaf.cells[idx] = newChainPointer
		// filter out old link with the same key
		for newChainPointer != nil && newChainPointer.next != nil {
			if newChainPointer.next.key == key {
				// skip one link,
				// we avoid mutation of already existent link, by copying it
				newLink := m.allocPair()
				newLink.key = newChainPointer.key
				newLink.value = newChainPointer.value
				newLink.next = newChainPointer.next
				newChainPointer = newLink
				newChainPointer.next = newChainPointer.next.next
				break
			}
			newChainPointer = newChainPointer.next
		}
	}
	return leaf
}

func (m *Map[K, V]) Range(f func(key K, value V) bool) {
	node := (*treeNode[K, V])(m.root)
	// maybe use pool here as well
	stackOfNodes := []unsafe.Pointer{unsafe.Pointer(node)}
	for len(stackOfNodes) != 0 {
		node = (*treeNode[K, V])(stackOfNodes[len(stackOfNodes)-1])
		stackOfNodes = stackOfNodes[:len(stackOfNodes)-1]

		if node.isLeaf == 1 {
			leaf := (*leafNode[K, V])(unsafe.Pointer(node))
			for _, pair := range leaf.cells {
				if pair != nil {
					shouldContinue := f(pair.key, pair.value)
					if !shouldContinue {
						return
					}
				}
			}
			continue
		}

		branch := (*branchNode[K, V])(unsafe.Pointer(node))
		stackOfNodes = append(stackOfNodes, branch.branches.b...)
	}
}

func (m *Map[K, V]) Render() {
	level := 0
	node := (*treeNode[K, V])(m.root)
	stackOfNodes := []unsafe.Pointer{unsafe.Pointer(node)}
	for len(stackOfNodes) != 0 {
		node = (*treeNode[K, V])(stackOfNodes[len(stackOfNodes)-1])
		stackOfNodes = stackOfNodes[:len(stackOfNodes)-1]

		if node.isLeaf == 1 {
			leaf := (*leafNode[K, V])(unsafe.Pointer(node))
			fmt.Printf(strings.Repeat("	", level) + "leaf {")
			size := 0
			for i := 0; i < len(leaf.cells); i++ {
				//fmt.Printf(strings.Repeat("	", level)+" [%v] -> ", i)

				node := leaf.cells[i]
				depth := 0
				for {
					if node == nil {
						//fmt.Printf(strings.Repeat("	", level)+"%+v", nil)
						break
					}
					fmt.Printf(strings.Repeat("	", level)+"(%+v:%+v)[%v] ", node.key, node.value, depth)
					size++
					depth++
					node = node.next
				}
				//fmt.Printf("\n")
			}
			fmt.Printf("}\n")
			continue
		}
		branch := (*branchNode[K, V])(unsafe.Pointer(node))
		level = int(branch.level)
		fmt.Printf(strings.Repeat("	", int(branch.level))+"branch<%v>[%v]{%.64b}\n", branch.level, bits.OnesCount64(branch.mask), branch.mask)
		stackOfNodes = append(stackOfNodes, branch.branches.b...)
	}
}

func (m *Map[K, V]) hash(key K) uint64 {
	var keyAsString string
	if m.keySize < 0 {
		keyAsString = *(*string)(unsafe.Pointer(&key))
	} else {
		keyAsString = unsafe.String(
			(*byte)(unsafe.Pointer(&key)),
			m.keySize,
		)
	}
	return xxh3.HashStringSeed(keyAsString, m.seed)
}

//nolint:gochecknoglobals // taskQueue is global to maximise goroutine pool utilization
var taskQueue = make(chan func())

func runInPool(task func()) {
	select {
	case taskQueue <- task:
		// submitted, everything is ok
	default:
		go func() {
			// do the given task
			task()

			const cleanupDuration = 100 * time.Millisecond
			cleanupTicker := time.NewTicker(cleanupDuration)
			defer cleanupTicker.Stop()

			for {
				select {
				case t := <-taskQueue:
					t()
					cleanupTicker.Reset(cleanupDuration)
				case <-cleanupTicker.C:
					return
				}
			}
		}()
	}
}

func zero[T any]() T {
	return *new(T)
}
