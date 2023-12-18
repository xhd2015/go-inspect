package depcheck

import (
	"fmt"
	"go/token"
	"path/filepath"
	"testing"

	"golang.org/x/tools/go/packages"
)

// go test -run TestP1 -v ./depcheck
func TestP1(t *testing.T) {
	testPkgOrder(t, "./testdata/p1")
}
func TestP2(t *testing.T) {
	testPkgOrder(t, "./testdata/p2")
}

// go test -run TestP3 -v ./depcheck
func TestP3(t *testing.T) {
	testPkgOrder(t, "./testdata/p3")
}

// go test -run TestP4 -v ./depcheck
func TestP4(t *testing.T) {
	testPkgOrder(t, "./testdata/p4")
}
func testPkgOrder(t *testing.T, dir string) {
	fset := token.NewFileSet()
	cfg := &packages.Config{
		Dir:  dir,
		Mode: packages.NeedDeps | packages.NeedName | packages.NeedSyntax | packages.NeedImports,
		Fset: fset,
	}
	pkgs, err := packages.Load(cfg, "./")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("files: %v\n", pkgs[0].GoFiles)
	for _, f := range pkgs[0].Syntax {
		finfo := fset.File(f.Package)
		fmt.Printf("file: %s\n", filepath.Base(finfo.Name()))
	}
	packages.Visit(pkgs, func(p *packages.Package) bool {
		fmt.Printf("visit: %s\n", p.PkgPath)
		return true
	}, nil)
}
