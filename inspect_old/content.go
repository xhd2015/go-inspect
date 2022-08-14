package inspect

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/packages"
)

func getContent(fset *token.FileSet, content []byte, begin token.Pos, end token.Pos) []byte {
	beginOff := fset.Position(begin).Offset
	endOff := len(content)
	if end != token.NoPos {
		endOff = fset.Position(end).Offset
	}
	return content[beginOff:endOff]
}

func IsExportedName(name string) bool {
	if name == "" {
		return false
	}
	c := name[0]
	return c >= 'A' && c <= 'Z'

	// buggy: _X is not exported, but this still gets it.
	// return len(name) > 0 && strings.ToUpper(name[0:1]) == name[0:1]
}
func StripNewline(s string) string {
	return strings.ReplaceAll(s, "\n", "")
}
func ToExported(name string) string {
	if name == "" {
		return name
	}
	c := name[0]
	if c >= 'A' && c <= 'Z' {
		return name
	}
	// c is lower or other
	c1 := strings.ToUpper(name[0:1])
	if c1[0] != c {
		return c1 + name[1:]

	}
	// failed to make expored, such as "_A", just make a "M_" prefix
	return "M_" + name
}
func fileNameOf(fset *token.FileSet, f *ast.File) string {
	tokenFile := fset.File(f.Package)
	if tokenFile == nil {
		panic(fmt.Errorf("no filename of:%v", f))
	}
	return tokenFile.Name()
}

// empty if not set
func typeName(pkg *packages.Package, t types.Type) (ptr bool, name string) {
	ptr, named := getNamedOrPtr(t)
	if named != nil {
		name = named.Obj().Name()
	}
	return
}
func resolveType(pkg *packages.Package, t ast.Expr) types.Type {
	return pkg.TypesInfo.TypeOf(t)
}

func getNamedOrPtr(t types.Type) (ptr bool, n *types.Named) {
	if x, ok := t.(*types.Pointer); ok {
		ptr = true
		t = x.Elem()
	}
	if named, ok := t.(*types.Named); ok {
		n = named
	}
	return
}
