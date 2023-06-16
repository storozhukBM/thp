package hamt1

import (
	"fmt"
	"github.com/zeebo/xxh3"
	"math/rand"
	"unsafe"
)

func zero[T any]() T {
	return *new(T)
}

type treeNode[K comparable, V any] struct {
	isLeaf byte
}

type branchNode[K comparable, V any] struct {
	treeNode[K, V]
	mask     uint64
	branches []*treeNode[K, V]
}

//
//type leafNode[K comparable, V any] struct {
//	isLeaf                      byte
//	size                        byte   // 15 max
//	maskOfBranchesWithCollision uint16 // if bit is set branch contains a *[]pair
//	branchHashes                [15]uint32
//	// End of 1st cache line
//
//	branches [15]unsafe.Pointer // union(*pair,*[]pair)
//}

type pair[K comparable, V any] struct {
	key   K
	value V
	next  *pair[K, V]
}

type leafNode[K comparable, V any] struct {
	treeNode[K, V]
	branches [16]*pair[K, V]
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
	rootNode := *(*treeNode[K, V])(m.root)

	if rootNode.isLeaf == 1 {
		leafPointer := (*leafNode[K, V])(m.root)
		idx := keyHash >> 60
		targetChain := leafPointer.branches[idx]
		for targetChain != nil {
			if targetChain.key == key {
				return targetChain.value, true
			}
			targetChain = targetChain.next
		}
		return zero[V](), false
	}

	panic("not implemented")
}

func (m *Map[K, V]) Store(key K, value V) *Map[K, V] {
	level := 0
	keyHash := m.hash(key)

	rootNode := *(*treeNode[K, V])(m.root)

	if rootNode.isLeaf == 1 {
		leafPointer := (*leafNode[K, V])(m.root)
		if level != 10 {
			counter := 0
			for _, ptr := range leafPointer.branches {
				if ptr != nil {
					counter++
				}
			}

			if counter > len(leafPointer.branches)/2 {
				fmt.Printf("Consider spliting\n")
			}
		}
		newLeaf := m.StoreToLeaf(keyHash, key, value, leafPointer)
		return &Map[K, V]{
			seed:    m.seed,
			keySize: m.keySize,
			root:    unsafe.Pointer(newLeaf),
		}
	}

	panic("not implemented")
}

func (m *Map[K, V]) StoreToLeaf(keyHash uint64, key K, value V, leafPointer *leafNode[K, V]) *leafNode[K, V] {
	copyOfLeaf := *leafPointer
	idx := keyHash >> 60 // last 4 bits

	if copyOfLeaf.branches[idx] == nil {
		copyOfLeaf.branches[idx] = &pair[K, V]{
			key:   key,
			value: value,
			next:  nil,
		}
	} else {
		oldChainPointer := copyOfLeaf.branches[idx]
		newChainPointer := &pair[K, V]{
			key:   key,
			value: value,
			next:  oldChainPointer,
		}
		copyOfLeaf.branches[idx] = newChainPointer
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
	return &copyOfLeaf
}

func (m *Map[K, V]) Range(f func(key K, value V) bool) {
	node := (*treeNode[K, V])(m.root)
	stackOfNodes := []*treeNode[K, V]{node}
	for len(stackOfNodes) != 0 {
		node = stackOfNodes[len(stackOfNodes)-1]
		stackOfNodes = stackOfNodes[:len(stackOfNodes)-1]

		if node.isLeaf == 1 {
			leaf := (*leafNode[K, V])(unsafe.Pointer(node))
			for _, pair := range leaf.branches {
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
	node := (*treeNode[K, V])(m.root)
	stackOfNodes := []*treeNode[K, V]{node}
	for len(stackOfNodes) != 0 {
		node = stackOfNodes[len(stackOfNodes)-1]
		stackOfNodes = stackOfNodes[:len(stackOfNodes)-1]

		if node.isLeaf == 1 {
			leaf := (*leafNode[K, V])(unsafe.Pointer(node))
			fmt.Printf("leaf {\n")
			for i := 0; i < len(leaf.branches); i++ {
				fmt.Printf("	[%v] -> ", i)
				node := leaf.branches[i]
				for {
					if node == nil {
						fmt.Printf("%+v", nil)
						break
					}
					fmt.Printf("&{key:%+v value:%+v} -> ", node.key, node.value)
					node = node.next
				}
				fmt.Printf("\n")
			}
			fmt.Printf("}\n")
			continue
		}

		branch := (*branchNode[K, V])(unsafe.Pointer(node))
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
