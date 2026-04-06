package convert

import (
	"unsafe"
)

type Iface struct {
	Typ   unsafe.Pointer
	Value unsafe.Pointer
}
