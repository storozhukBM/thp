package goid

import (
	"reflect"
	"unsafe"
)

//go:linkname typelinks reflect.typelinks
func typelinks() (sections []unsafe.Pointer, offset [][]int32)

//go:linkname resolveTypeOff reflect.resolveTypeOff
func resolveTypeOff(rtype unsafe.Pointer, off int32) unsafe.Pointer

// getg returns the pointer to the current runtime.g.
//
//go:nosplit
func getg() unsafe.Pointer

type iface struct {
	tab  unsafe.Pointer
	data unsafe.Pointer
}

var _goIDOffset uintptr

func init() {
	_goIDOffset = getGoroutineIdOffsetInRuntimeGStruct()
}

func getGoroutineIdOffsetInRuntimeGStruct() uintptr {
	typ := reflect.TypeOf(0)
	face := (*iface)(unsafe.Pointer(&typ))

	sections, offset := typelinks()
	for i, offs := range offset {
		rodata := sections[i]
		for _, off := range offs {
			face.data = resolveTypeOff(rodata, off)
			if typ.Kind() == reflect.Ptr && len(typ.Elem().Name()) > 0 {
				if "runtime.g" == typ.Elem().String() {
					typ = typ.Elem()
				}
				if "runtime.g" == typ.String() {
					for i := 0; i < typ.NumField(); i++ {
						f := typ.Field(i)
						if f.Name == "goid" && f.Type == reflect.TypeOf(uint64(0)) {
							return f.Offset
						}
					}
				}
			}
		}
	}
	panic("runtime.g.goid not found")
}

// ID returns current goroutine's runtime ID
func ID() uint64 {
	gp := getg()
	// fmt.Printf("%v\n", uintptr(gp))
	return *(*uint64)(unsafe.Pointer(uintptr(gp) + _goIDOffset))
}
