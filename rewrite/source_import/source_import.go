package source_import

import (
	"fmt"
	"strings"
	"time"

	source_import_internal "github.com/xhd2015/go-inspect/rewrite/internal/source_import"
	"github.com/xhd2015/go-inspect/rewrite/model"
	"github.com/xhd2015/go-inspect/rewrite/session"
	"github.com/xhd2015/go-vendor-pack/packfs"
	"github.com/xhd2015/go-vendor-pack/unpack"
)

type Modules map[string][]*model.Module

type registry struct {
	mods Modules
}

func NewRegistry() source_import_internal.SourceImportRegistryRetriever {
	return &registry{
		mods: make(Modules),
	}
}

var _ session.SourceImportRegistry = (*registry)(nil)

func (c *registry) AddModule(mod *model.Module) error {
	if mod == nil || mod.Path == "" {
		return fmt.Errorf("no module path: %+v", mod)
	}

	c.mods[mod.Path] = append(c.mods[mod.Path], mod)
	return nil
}

func (c *registry) ImportPackedModulesBase64(s string) error {
	fs, err := unpack.NewTarFSWithBase64Decode(s)
	if err != nil {
		return err
	}
	return c.ImportPackedModules(fs)
}

// unpack all dependencies
func (c *registry) ImportPackedModules(fs packfs.FS) error {
	goList, err := unpack.ReadGoList(fs)
	if err != nil {
		return err
	}

	modReplace := make(map[string]string)
	for _, rep := range goList.GoMod.Replace {
		modReplace[rep.Old.Path] = rep.New.Path
	}

	packTime, err := time.ParseInLocation("2006-01-02 15:04:05", goList.PackTimeUTC, time.UTC)
	if err != nil {
		return fmt.Errorf("invalid pack time")
	}

	for _, m := range goList.Modules {
		// skip main module
		if m.Path == goList.GoMod.Module.Path {
			continue
		}

		rep := modReplace[m.Path]
		packages := make(map[string]*model.Package, len(m.Packages))
		for _, pkg := range m.Packages {
			packages[pkg.ImportPath] = &model.Package{
				Path: pkg.ImportPath,
			}
		}
		c.AddModule(&model.Module{
			Path:              m.Path,
			Version:           m.Version,
			ReplacedWithLocal: rep != "" && (strings.HasPrefix(rep, "/") || strings.HasPrefix(rep, "./") || strings.HasPrefix(rep, "../")),
			UpdateTime:        packTime,
			FS:                fs,
			Packages:          packages,
		})
	}
	return nil
}
