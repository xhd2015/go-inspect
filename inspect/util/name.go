package util

import (
	"fmt"
	"go/ast"
	"go/types"
	"io/ioutil"

	"golang.org/x/tools/go/packages"
)

// NextName `addIfNotExists` returns true if `s` available
func NextName(addIfNotExists func(string) bool, name string) string {
	if addIfNotExists(name) {
		return name
	}
	for i := 1; i < 100000; i++ {
		namei := fmt.Sprintf("%s%d", name, i)
		if addIfNotExists(namei) {
			return namei
		}
	}
	panic(fmt.Errorf("nextName failed, tried 10,0000 times.name: %v", name))
}

func NextFileNameUnderDir(dir string, name string, suffix string) string {
	starterNames, _ := ioutil.ReadDir(dir)
	return NextName(func(s string) bool {
		for _, f := range starterNames {
			if f.Name() == s+suffix {
				return false
			}
		}
		return true
	}, name) + suffix
}

// NOTE: a replacement of implements. No successful try made yet.
// TODO: test types.AssignableTo() for types from the same Load.
func HasQualifiedName(t types.Type, pkg, name string) bool {
	switch t := t.(type) {
	case *types.Named:
		o := t.Obj()
		p := o.Pkg()
		if (p == nil && pkg != "") || (p != nil && p.Path() != pkg) {
			return false
		}
		return o.Name() == name
	}
	return false
}

func TokenHasQualifiedName(p *packages.Package, t ast.Expr, pkg string, name string) bool {
	argType := p.TypesInfo.TypeOf(t)
	return HasQualifiedName(argType, pkg, name)
}
