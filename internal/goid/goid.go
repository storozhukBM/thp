package goid

import (
	"reflect"
	"unsafe"
)

//go:linkname typelinks reflect.typelinks
func typelinks() ([]unsafe.Pointer, [][]int32)

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

//nolint:gochecknoglobals // this is better than hardcoding, `goid` field offset
var _goIDOffset uintptr

//nolint:gochecknoinits // this is better than hardcoding, `goid` field offset
func init() {
	_goIDOffset = getGoroutineIDOffsetInRuntimeGStruct()
}

func getGoroutineIDOffsetInRuntimeGStruct() uintptr {
	typ := reflect.TypeOf(0)
	face := (*iface)(unsafe.Pointer(&typ))

	sections, offset := typelinks()
	for i, offs := range offset {
		rodata := sections[i]
		for _, off := range offs {
			face.data = resolveTypeOff(rodata, off)
			if typ.Kind() != reflect.Ptr || len(typ.Elem().Name()) == 0 {
				continue
			}
			if typ.Elem().String() == "runtime.g" {
				typ = typ.Elem()
			}
			if typ.String() == "runtime.g" {
				for i := 0; i < typ.NumField(); i++ {
					f := typ.Field(i)
					if f.Name == "goid" && f.Type == reflect.TypeOf(uint64(0)) {
						return f.Offset
					}
				}
			}
		}
	}
	panic("runtime.g.goid not found")
}

// ID returns current goroutine's runtime ID.
func ID() uint64 {
	gp := getg()
	return *(*uint64)(unsafe.Pointer(uintptr(gp) + _goIDOffset))
}
