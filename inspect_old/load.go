package inspect

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	"golang.org/x/tools/go/packages"

	inspect "github.com/xhd2015/go-inspect/inspect"
	"github.com/xhd2015/go-inspect/inspect/util"
)

func MakePackageMap(pkgs []*packages.Package) map[string]*packages.Package {
	m := make(map[string]*packages.Package, len(pkgs))
	for _, pkg := range pkgs {
		m[normalizePkgPath(pkg)] = pkg
	}
	return m
}

var lowCase = regexp.MustCompile("^[a-z0-9]+$")

func GetPkgModule(p *packages.Package) *packages.Module {
	if p.Module != nil {
		return p.Module
	}
	// check if it is std module(i.e. src inside $GOROOT)
	pkgPath := p.PkgPath
	idx := strings.Index(pkgPath, "/")
	//  excluding: github.com, xxx.com, golang.x.org...
	isStd := idx < 0 || lowCase.MatchString(pkgPath[:idx])
	if isStd {
		return util.GetStdModule()
	}
	return nil
}

func GetModuleDir(m *packages.Module) string {
	if m.Replace != nil {
		return m.Replace.Dir
	}
	return m.Dir
}

// GetSameModulePackagesAndPkgsGiven expand to all packages under
// the same module that depended by starter packages
func GetSameModulePackagesAndPkgsGiven(loadInfo inspect.LoadInfo, needPkg func(pkgPath string) bool, needMod func(modPath string) bool) (sameModulePkgs []inspect.Pkg, extraPkgs []inspect.Pkg) {
	modules := make(map[string]bool)
	for _, pkg := range loadInfo.StarterPkgs() {
		modules[pkg.Module().OrigPath()] = true
	}

	extraPkgs = make([]inspect.Pkg, 0, len(loadInfo.StarterPkgs()))
	loadInfo.RangePkgs(func(p inspect.Pkg) bool {
		// filter test package of current module
		if modules[p.Module().OrigPath()] {
			if !util.IsTestPkgOfModule(p.Module().OrigPath(), p.Path()) {
				// do not add test package to result, but still traverse its dependencies
				sameModulePkgs = append(sameModulePkgs, p)
			}
			return true
		}

		//  extra packages
		if needMod != nil && needMod(p.Module().OrigPath()) {
			extraPkgs = append(extraPkgs, p)
			return true
		}
		if needPkg != nil && needPkg(p.Path()) {
			extraPkgs = append(extraPkgs, p)
			return true
		}

		return true
	})
	return
}

func normalizePackage(pkg *packages.Package) {
	// normalize pkg path
	pkg.PkgPath = normalizePkgPath(pkg)
	pkg.Name = normalizePkgName(pkg)
	pkg.Module = GetPkgModule(pkg)
}

func normalizePkgName(pkg *packages.Package) string {
	name := pkg.Name
	if name == "" {
		name = pkg.Types.Name()
	}
	return name
}

func normalizePkgPath(pkg *packages.Package) string {
	pkgPath := pkg.PkgPath
	if pkgPath == "" {
		pkgPath = pkg.Types.Path()
	}

	// normalize pkgPath
	if pkgPath == "command-line-arguments" {
		if len(pkg.GoFiles) > 0 {
			pkgPath = GetPkgPathOfFile(pkg.Module, path.Dir(pkg.GoFiles[0]))
		}
	}
	return pkgPath
}

// GetPath the returned result is guranteed to not end with "/"
// `filePath` is an absolute path on the filesystem.
func GetPkgPathOfFile(mod *packages.Module, fsPath string) string {
	modPath, modDir := mod.Path, mod.Dir
	if mod.Replace != nil {
		modPath, modDir = mod.Replace.Path, mod.Replace.Dir
	}

	rel, ok := util.RelPath(modDir, fsPath)
	if !ok {
		panic(fmt.Errorf("%s not child of %s", fsPath, modDir))
	}

	return strings.TrimSuffix(path.Join(modPath, rel), "/")
}

func GetFsPathOfPkg(mod *packages.Module, pkgPath string) string {
	modPath, modDir := mod.Path, mod.Dir
	if mod.Replace != nil {
		modPath, modDir = mod.Replace.Path, mod.Replace.Dir
	}
	if modPath == pkgPath {
		return modDir
	}
	if strings.HasPrefix(pkgPath, modPath) {
		return path.Join(modDir, pkgPath[len(modPath):])
	}
	panic(fmt.Errorf("%s not child of %s", pkgPath, modPath))
}
func GetRelativePath(modPath string, pkgPath string) string {
	if pkgPath == "" {
		panic(fmt.Errorf("GetRelativePath empty pkgPath"))
	}
	if strings.HasPrefix(pkgPath, modPath) {
		return strings.TrimPrefix(pkgPath[len(modPath):], "/")
	}
	panic(fmt.Errorf("%s not child of %s", pkgPath, modPath))
}
func GetFsPath(mod *packages.Module, relPath string) string {
	if mod.Replace != nil {
		return path.Join(mod.Replace.Dir, relPath)
	}
	return path.Join(mod.Dir, relPath)
}
