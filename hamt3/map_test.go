package hamt3

import (
	"bytes"
	"fmt"
	"github.com/zeebo/xxh3"
	"math/bits"
	"strconv"
	"testing"
)

func TestSimpleMap(t *testing.T) {
	m1 := NewMap[string, int]()
	//thp.Dump(printMap(m1))
	//fmt.Printf("\n")
	//
	//m11 := m1.Store("a", 1)
	//thp.Dump(printMap(m11))
	//thp.Dump(printMap(m1))
	//fmt.Printf("\n")
	//
	//m111 := m11.Store("b", 2)
	//thp.Dump(printMap(m111))
	//thp.Dump(printMap(m11))
	//thp.Dump(printMap(m1))
	//fmt.Printf("\n")
	//
	//m12 := m1.Store("b", 3)
	//thp.Dump(printMap(m12))
	//thp.Dump(printMap(m111))
	//thp.Dump(printMap(m11))
	//thp.Dump(printMap(m1))
	//
	//m1111 := m111.Store("b", 4)
	//thp.Dump(printMap(m1111))
	//thp.Dump(printMap(m12))
	//thp.Dump(printMap(m111))
	//thp.Dump(printMap(m11))
	//thp.Dump(printMap(m1))
	//
	//{
	//	key := "a"
	//	val, ok := m1111.Load(key)
	//	fmt.Printf("%v -> %v, %v\n", key, val, ok)
	//}
	//{
	//	key := "b"
	//	val, ok := m1111.Load(key)
	//	fmt.Printf("%v -> %v, %v\n", key, val, ok)
	//}
	//{
	//	key := "z"
	//	val, ok := m1111.Load(key)
	//	fmt.Printf("%v -> %v, %v\n", key, val, ok)
	//}

	for i := 0; i < 20000; i++ {
		m1 = m1.Store(strconv.Itoa(i), i)
	}
	//thp.Dump(printMap(m1))
	m1.Render()
}

func TestHashParts(t *testing.T) {
	hash := xxh3.HashStringSeed("some", 92834)
	branchMask := xxh3.HashStringSeed("other", 92834)

	for i := 0; i < 11; i++ {
		mask := uint64((1 << 6) - 1)

		fmt.Printf("%.64b <- hash\n", hash)
		fmt.Printf("%.64b <- sifted hash\n", hash>>(6*i))
		fmt.Printf("%.64b <- mask\n", mask)
		fmt.Printf("%.64b <- hash & mask\n", (hash>>(6*i))&mask)
		bitIdx := (hash >> (6 * i)) & mask
		fmt.Printf("%.64d <- bitIdx\n", bitIdx)
		fmt.Printf("%.64b <- branchIdx\n", 1<<bitIdx)
		fmt.Printf("\n")
		fmt.Printf("%.64b <- branchMask\n", branchMask)
		fmt.Printf("%.64b <- branchSelectionMask\n", (1<<bitIdx)-1)
		fmt.Printf("%.64b <- branchMask & branchSelectionMask\n", branchMask&((1<<bitIdx)-1))
		fmt.Printf("%.64d <- popcnt\n", bits.OnesCount64(branchMask&((1<<bitIdx)-1)))
		fmt.Printf("---\n")
	}

}

func printMap[K comparable, V any](m *Map[K, V]) string {
	buf := &bytes.Buffer{}
	_, _ = fmt.Fprintf(buf, "[")
	m.Range(func(key K, value V) bool {
		_, _ = fmt.Fprintf(buf, "%#v: %#v, ", key, value)
		return true
	})
	_, _ = fmt.Fprintf(buf, "]")
	return buf.String()
}
