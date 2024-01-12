package source_import

import (
	"fmt"
	"path"
	"sort"

	"github.com/xhd2015/go-inspect/rewrite/model"
	"github.com/xhd2015/go-inspect/rewrite/session"
	"github.com/xhd2015/go-vendor-pack/packfs/map_fs"

	source_import_internal "github.com/xhd2015/go-inspect/rewrite/internal/source_import"
	"github.com/xhd2015/go-vendor-pack/go_info"
	"github.com/xhd2015/go-vendor-pack/unpack/helper"
)

var _ source_import_internal.SourceImportRegistryRetriever = (*registry)(nil)

// GetModules implements internal.SourceImportRegistryRetriever.
func (c *registry) GetModules() map[string][]*model.Module {
	return c.mods
}

// OnSessionGenOverlay will be called after every GenOverlay called
func OnSessionGenOverlay(sess session.Session) {
	mods := sess.(source_import_internal.SourceImportRegistryRetriever).GetModules()
	genOverlay(sess, mods)
}

// GenOverlay implements project.Rewriter
func genOverlay(session session.Session, modMapping Modules) {
	g := session.Global()
	proj := session.Project()
	dirs := session.Dirs()

	wfs := session.RewriteFS()

	goVersion, err := go_info.GetGoVersionCached()
	if err != nil {
		panic(err)
	}
	var rewriteVendorDir string
	isVendor := proj.IsVendor()
	if isVendor {
		rewriteVendorDir = path.Join(dirs.RewriteProjectRoot(), "vendor")
	} else {
		rewriteVendorDir = dirs.RewriteProjectVendorRoot()
	}

	// process in order
	type modInfo struct {
		modPath string
		mods    []*model.Module
	}
	modInfos := make([]*modInfo, 0, len(modMapping))
	for modPath, mods := range modMapping {
		modInfos = append(modInfos, &modInfo{
			modPath: modPath,
			mods:    mods,
		})
	}
	sort.Slice(modInfos, func(i, j int) bool {
		return modInfos[i].modPath < modInfos[j].modPath
	})
	// copy each module groups
	for _, modInfo := range modInfos {
		modPath := modInfo.modPath
		mods := modInfo.mods
		projectMod := g.GetModule(modPath)
		var projectModAdapter *model.Module
		if projectMod != nil && !projectMod.IsStd() {
			projectModAdapter = &model.Module{
				Path:              modPath,
				Version:           projectMod.ModInfo().Version,
				ReplacedWithLocal: projectMod.ModInfo().Replace != nil && projectMod.ModInfo().Dir != "",
			}
		}
		// sort each modules group
		sort.Slice(mods, func(i, j int) bool {
			return compareModule(mods[i], mods[j]) > 0
		})

		// copy packages
		// guranted that only newer module will be placed into copiedModMapping
		copiedModMapping := make(map[string]*model.Module)
		for _, mod := range mods {
			for pkgPath := range mod.Packages {
				// check if we need to copy
				prevMod := copiedModMapping[pkgPath]
				if prevMod != nil {
					continue
				}

				projPkg := g.GetPkg(pkgPath)
				if projPkg != nil && projectModAdapter != nil && compareModuleWithoutUpdateTime(projectModAdapter, mod) >= 0 {
					// project package is newer, skip copy
					copiedModMapping[pkgPath] = projectModAdapter
					continue
				}

				// copy files without override
				err := helper.OverrideFilesFS(mod.FS, wfs, "vendor/"+pkgPath, rewriteVendorDir+"/"+pkgPath)
				if err != nil {
					panic(fmt.Errorf("GenOverlay: %w", err))
				}
				copiedModMapping[pkgPath] = mod
			}
		}
		maxMod := mods[0]
		if projectModAdapter != nil && compareModuleWithoutUpdateTime(projectModAdapter, maxMod) >= 0 {
			maxMod = projectModAdapter
		}

		// for non-vendor: copy all files of modules into tmp directory,
		// as we are going to replace module paths
		var replaceModPath string
		if !isVendor {
			replaceModPath = rewriteVendorDir + "/" + modPath
			mod := g.GetModule(modPath)
			if mod != nil {
				// copy mod.Dir()
				mapFS, err := map_fs.NewFromDir(mod.Dir(), modPath)
				if err != nil {
					panic(err)
				}
				err = helper.CopyFilesFS(mapFS, wfs, modPath, replaceModPath, nil)
				if err != nil {
					panic(err)
				}
			}
		}

		// edit module replace
		// update go.mod, go.sum
		// and if vendor, update vendor/modules.txt.
		// NOTE: can always ignore sums if we make absolution replace here
		// and also NOTE that, seems sums not necessary when in vendor mode without replace
		err := helper.AddVersionAndSumFS(wfs, dirs.RewriteProjectRoot(), modPath, maxMod.Version, "" /*sums optional*/, replaceModPath)
		if err != nil {
			panic(fmt.Errorf("adding sum:%s %w", modPath, err))
		}
		if !isVendor {
			err := helper.TruncateGoModFS(wfs, path.Join(replaceModPath, "go.mod"), modPath, goVersion.Major, goVersion.Minor)
			if err != nil {
				panic(fmt.Errorf("truncating go mod: %s %w", modPath, err))
			}
		}
	}
}

func compareModuleWithoutUpdateTime(a *model.Module, b *model.Module) int {
	if a == b {
		return 0
	}
	if a.ReplacedWithLocal != b.ReplacedWithLocal {
		// only one is replaced, return the replaced one
		if a.ReplacedWithLocal {
			return 1
		}
		return -1
	}
	// both not replaced and version unsame
	if a.Version != b.Version {
		// parseVersion
		aVersion := model.ParseVersion(a.Version)
		bVersion := model.ParseVersion(b.Version)

		return aVersion.Compare(&bVersion)
	}
	return 0
}

func compareModule(a *model.Module, b *model.Module) int {
	v := compareModuleWithoutUpdateTime(a, b)
	if v != 0 {
		return v
	}
	// both replaced or version same
	return int(a.UpdateTime.Sub(b.UpdateTime))
}
