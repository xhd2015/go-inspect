package export_g

import "unsafe"

var GetImpl func() unsafe.Pointer

func Get() unsafe.Pointer {
	if GetImpl == nil {
		return nil
	}
	return GetImpl()
}
