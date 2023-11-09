package util

import (
	"fmt"
	"go/ast"
	"go/types"
	"os"
	"os/exec"
	"path"
	"strings"
	"sync"

	"golang.org/x/tools/go/packages"
)

func IsInternalPkg(pkgPath string) bool {
	return ContainsSplitWith(pkgPath, "internal", '/')
}

func refInternalPkg(typesInfo *types.Info, n *ast.FuncDecl) bool {
	found := false
	// special case, if the function references to internal
	// packages,we skip
	ast.Inspect(n.Type, func(n ast.Node) bool {
		if found {
			return false
		}
		if x, ok := n.(*ast.SelectorExpr); ok {
			if id, ok := x.X.(*ast.Ident); ok {
				ref := typesInfo.Uses[id]
				if pkgName, ok := ref.(*types.PkgName); ok {
					extPkgPath := pkgName.Pkg().Path()
					if IsInternalPkg(extPkgPath) {
						found = true
						return false
					}
				}
			}
		}
		return true
	})
	return found
}

const versionStd = "pseduo-version: go-std"

func IsStdModule(m *packages.Module) bool {
	return m.Path == "" && m.Version == versionStd
}

var stdModule *packages.Module
var stdModuleOnce sync.Once
var goroot string
var gorootInitOnce sync.Once

func GetStdModule() *packages.Module {
	stdModuleOnce.Do(func() {
		stdModule = &packages.Module{
			Path:    "", // will this have problem?
			Dir:     path.Join(GetGOROOT(), "src"),
			Version: versionStd,
		}
	})
	return stdModule
}

func GetGOROOT() string {
	gorootInitOnce.Do(func() {
		var err error
		goroot, err = ComputeGOROOT()
		if err != nil {
			panic(err)
		}
	})
	return goroot
}

func ComputeGOROOT() (string, error) {
	goRootEnv := os.Getenv("GOROOT")
	if goRootEnv != "" {
		return goRootEnv, nil
	}

	gorootBytes, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		return "", fmt.Errorf("cannot get GOROOT, please consider add 'export GOROOT=$(go env GOROOT)' and retry")
	}
	return strings.TrimSuffix(string(gorootBytes), "\n"), nil
}

func IsTestPkgOfModule(module string, pkgPath string) bool {
	if !strings.HasPrefix(pkgPath, module) {
		panic(fmt.Errorf("pkgPath %s not child of %s", pkgPath, module))
	}
	x := strings.TrimPrefix(pkgPath[len(module):], "/")
	return strings.HasPrefix(x, "test") && (len(x) == len("test") || x[len("test")] == '/')
}

// Module is nil, and ends with .test or _test
func IsGoTestPkg(pkg *packages.Package) bool {
	// may even have pkg.Name == "main", or pkg.Name == "xxx"(defined in your package)
	// actually just to check the "forTest" property can confirm that, but that field is not exported.
	return pkg.Module == nil && (strings.HasSuffix(pkg.PkgPath, ".test") || strings.HasSuffix(pkg.PkgPath, "_test"))
}

func IsVendor(modDir string, subPath string) bool {
	if modDir == "" || subPath == "" {
		return false
	}
	if !strings.HasPrefix(subPath, modDir) {
		return false
	}
	rel := subPath[len(modDir):]
	vdr := "/vendor/"
	if subPath[len(subPath)-1] == '/' {
		vdr = "vendor/"
	}
	return strings.HasPrefix(rel, vdr)
}

func ExtractSingleMod(starterPkgs []*packages.Package) (modPath string, modDir string) {
	// debug
	// for _, p := range starterPkgs {
	// 	fmt.Printf("starter pkg:%v\n", p.PkgPath)
	// 	if p.Module != nil {
	// 		fmt.Printf("starter model:%v %v\n", p.PkgPath, p.Module.Path)
	// 	}
	// }
	for _, p := range starterPkgs {
		mod := p.Module
		if p.Module == nil {
			if IsGoTestPkg(p) {
				continue
			}
			panic(fmt.Errorf("package %s has no module", p.PkgPath))
		}
		if mod.Replace != nil {
			panic(fmt.Errorf("package %s has a replacement module %s, but want a self-rooted module: %s", p.PkgPath, mod.Replace.Dir, mod.Path))
		}
		if modPath == "" {
			modPath = mod.Path
			modDir = mod.Dir
			continue
		}
		if modPath != mod.Path || modDir != mod.Dir {
			panic(fmt.Errorf("package %s has different module %s, want a single module:%s", p.PkgPath, mod.Path, modPath))
		}
	}
	if modPath == "" || modDir == "" {
		panic("no modules loaded")
	}
	return
}
