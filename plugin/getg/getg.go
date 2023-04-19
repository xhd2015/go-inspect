package getg

import "unsafe"

var GetImpl func() unsafe.Pointer

func Enabled() bool {
	return GetImpl != nil
}

func G() unsafe.Pointer {
	if GetImpl == nil {
		return nil
	}
	return GetImpl()
}
