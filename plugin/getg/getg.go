package getg

import "unsafe"

var GetImpl func() unsafe.Pointer

func G() unsafe.Pointer {
	if GetImpl == nil {
		return nil
	}
	return GetImpl()
}
