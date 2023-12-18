package depcheck

import (
	"fmt"
	"strconv"

	"golang.org/x/tools/go/packages"
)

type PkgDepInfo struct {
	Pkg        string        `json:"pkg"`
	Depends    []*PkgDepInfo `json:"depends"`
	DependedBy []string      `json:"dependedBy,omitempty"`
}
type CollectOptions struct {
	NeedDependedBy bool
}

func CollectDeps(pkgs []*packages.Package, opts *CollectOptions) ([]*PkgDepInfo, map[string]*PkgDepInfo, error) {
	var needDependedBy bool
	if opts != nil {
		needDependedBy = opts.NeedDependedBy
	}
	// packages should be collected at least with:
	//   packages.NeedDeps | packages.NeedName | packages.NeedSyntax | packages.NeedImports
	pkgMapping := make(map[string]*PkgDepInfo)

	var visit func(parent *PkgDepInfo, p *packages.Package) error
	visit = func(parent *PkgDepInfo, p *packages.Package) error {
		pkgInfo := pkgMapping[p.PkgPath]
		if pkgInfo != nil {
			if needDependedBy {
				pkgInfo.DependedBy = append(pkgInfo.DependedBy, parent.Pkg)
			}
			// already init
			return nil
		}
		pkgInfo = &PkgDepInfo{
			Pkg: p.PkgPath,
		}
		pkgMapping[p.PkgPath] = pkgInfo
		parent.Depends = append(parent.Depends, pkgInfo)
		if needDependedBy {
			pkgInfo.DependedBy = append(pkgInfo.DependedBy, parent.Pkg)
		}
		for _, f := range p.Syntax {
			for _, imp := range f.Imports {
				depPkgPath, err := strconv.Unquote(imp.Path.Value)
				if err != nil {
					return fmt.Errorf("parse import path: %s %w", imp.Path.Value, err)
				}
				impPkg := p.Imports[depPkgPath]
				if impPkg == nil {
					return fmt.Errorf("getting imported package failed: %s->%s", p.PkgPath, depPkgPath)
				}
				err = visit(pkgInfo, impPkg)
				if err != nil {
					return err
				}
			}
		}
		return nil
	}

	root := &PkgDepInfo{}
	for _, pkg := range pkgs {
		err := visit(root, pkg)
		if err != nil {
			return nil, nil, err
		}
	}
	return root.Depends, pkgMapping, nil
}

func GetImportTrace(depInfo []*PkgDepInfo, pkg string) []string {
	var result []string
	var deepFind func(prefix []string, pkgInfo *PkgDepInfo) bool
	deepFind = func(prefix []string, pkgInfo *PkgDepInfo) bool {
		impTrace := append(prefix, pkgInfo.Pkg)
		if pkgInfo.Pkg == pkg {
			result = impTrace
			return true
		}
		for _, dep := range pkgInfo.Depends {
			if deepFind(impTrace, dep) {
				return true
			}
		}
		return false
	}
	for _, info := range depInfo {
		if deepFind(nil, info) {
			break
		}
	}
	return result
}
