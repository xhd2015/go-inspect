package session

import (
	"github.com/xhd2015/go-inspect/rewrite/model"
	"github.com/xhd2015/go-vendor-pack/packfs"
)

type SourceImportRegistry interface {
	AddModule(mod *model.Module) error

	// unpack all dependencies
	ImportPackedModules(fs packfs.FS) error

	ImportPackedModulesBase64(s string) error
}
