package hamt2

import (
	"fmt"
	"github.com/zeebo/xxh3"
	"math/bits"
	"math/rand"
	"strings"
	"unsafe"
)

const hashPartMask = uint64((1 << 6) - 1)

func zero[T any]() T {
	return *new(T)
}

type treeNode[K comparable, V any] struct {
	isLeaf byte
}

type branchNode[K comparable, V any] struct {
	treeNode[K, V]
	level    uint8
	mask     uint64
	branches []unsafe.Pointer
}

func (b *branchNode[K, V]) copy() *branchNode[K, V] {
	result := &branchNode[K, V]{
		treeNode: treeNode[K, V]{
			isLeaf: 0,
		},
		level:    b.level,
		mask:     b.mask,
		branches: make([]unsafe.Pointer, len(b.branches)),
	}
	copy(result.branches, b.branches)
	return result
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
	cells [16]*pair[K, V]
}

func (l *leafNode[K, V]) copy() *leafNode[K, V] {
	newLeaf := &leafNode[K, V]{
		treeNode: treeNode[K, V]{
			isLeaf: 1,
		},
		cells: [16]*pair[K, V]{},
	}
	copy(newLeaf.cells[:], l.cells[:])
	return newLeaf
}

type Map[K comparable, V any] struct {
	seed    uint64
	keySize int
	root    unsafe.Pointer // *leafNode[K, V] | *branchNode[K, V]
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

	return &Map[K, V]{
		seed:    rand.Uint64(),
		keySize: keySize,
		root: unsafe.Pointer(&leafNode[K, V]{
			treeNode: treeNode[K, V]{isLeaf: 1},
		}),
	}
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
		node = (*treeNode[K, V])(branch.branches[branchIdx])
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
	result := &Map[K, V]{
		seed:    m.seed,
		keySize: m.keySize,
		root:    nil,
	}
	// pointer to pointer. I officially hate this, but it looks like
	// something I need to suffer through if I want to avoid recursive calls
	pointerChainPointer := &result.root
	node := (*treeNode[K, V])(m.root)
	for node.isLeaf == 0 {
		branch := (*branchNode[K, V])(unsafe.Pointer(node)).copy()
		bitIdx := (keyHash >> (6 * branch.level)) & hashPartMask
		branchIdx := bits.OnesCount64(branch.mask & ((1 << bitIdx) - 1))
		if branch.mask&(1<<bitIdx) == 0 {
			// this branch is not initialized
			// so we init it
			branch.mask = branch.mask | 1<<bitIdx
			branch.branches = append(branch.branches, nil)
			copy(branch.branches[branchIdx+1:], branch.branches[branchIdx:])
			branch.branches[branchIdx] = unsafe.Pointer(&leafNode[K, V]{
				treeNode: treeNode[K, V]{
					isLeaf: 1,
				},
				cells: [16]*pair[K, V]{},
			})
		}
		*pointerChainPointer = unsafe.Pointer(branch)
		pointerChainPointer = &branch.branches[branchIdx]
		node = (*treeNode[K, V])(branch.branches[branchIdx])
		depth++
	}

	leafPointer := (*leafNode[K, V])(unsafe.Pointer(node))
	newLeaf := m.StoreToLeaf(keyHash, key, value, leafPointer)
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

func (m *Map[K, V]) splitLeafIntoBranches(level int, newLeaf *leafNode[K, V]) *branchNode[K, V] {
	branch := &branchNode[K, V]{
		level: uint8(level + 1),
	}
	for i := 0; i < len(newLeaf.cells); i++ {
		node := newLeaf.cells[i]
		if node == nil {
			continue
		}
		for node != nil {
			nodeHash := m.hash(node.key)
			bitIdx := (nodeHash >> int64(6*branch.level)) & hashPartMask
			branch.mask = branch.mask | (1 << bitIdx)
			branchSliceIdx := bits.OnesCount64(branch.mask) - 1
			if branchSliceIdx >= len(branch.branches) {
				branch.branches = append(branch.branches, unsafe.Pointer(&leafNode[K, V]{
					treeNode: treeNode[K, V]{isLeaf: 1},
				}))
			}
			idxLeaf := (*leafNode[K, V])(branch.branches[branchSliceIdx])
			branch.branches[branchSliceIdx] = unsafe.Pointer(
				m.StoreToLeaf(nodeHash, node.key, node.value, idxLeaf),
			)
			node = node.next
		}
	}
	return branch
}

func (m *Map[K, V]) StoreToLeaf(keyHash uint64, key K, value V, leafPointer *leafNode[K, V]) *leafNode[K, V] {
	copyOfLeaf := leafPointer.copy()
	idx := keyHash >> 60 // last 4 bits

	if copyOfLeaf.cells[idx] == nil {
		copyOfLeaf.cells[idx] = &pair[K, V]{
			key:   key,
			value: value,
			next:  nil,
		}
	} else {
		oldChainPointer := copyOfLeaf.cells[idx]
		newChainPointer := &pair[K, V]{
			key:   key,
			value: value,
			next:  oldChainPointer,
		}
		copyOfLeaf.cells[idx] = newChainPointer
		// filter out old link with the same key
		for newChainPointer != nil && newChainPointer.next != nil {
			if newChainPointer.next.key == key {
				// skip one link,
				// we avoid mutation of already existent link, by copying it
				newLink := *newChainPointer
				newChainPointer = &newLink
				newChainPointer.next = newChainPointer.next.next
			}
			newChainPointer = newChainPointer.next
		}
	}
	return copyOfLeaf
}

func (m *Map[K, V]) Range(f func(key K, value V) bool) {
	node := (*treeNode[K, V])(m.root)
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
		stackOfNodes = append(stackOfNodes, branch.branches...)
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
		stackOfNodes = append(stackOfNodes, branch.branches...)
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
