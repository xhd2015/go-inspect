package inspect

import (
	"fmt"
	"go/ast"
	"go/token"
	"reflect"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/tools/go/packages"

	"github.com/xhd2015/go-inspect/inspect/util"
)

type Global interface {
	// code of an ast Node
	FileSet() *token.FileSet
	Code(n Node) string
	// NOTE: for ast.File, always use CodeAST() or FileCode() instead of CodeSlice
	CodeAST(n ast.Node) string
	CodeSlice(begin token.Pos, end token.Pos) string

	FileCode(absPath string) string

	GetModule(modPath string) Module
	GetPkg(pkgPath string) Pkg

	// Registry
	Registry() Registry

	// RangeModule(fn func(m Module) bool)
	RangePkg(fn func(pkg Pkg) bool)

	// RewriteAstNode
	// rewritter can be nil, filters can be nil, in which case it works as a String() method for the node
	RewriteAstNode(node ast.Node, rewritter func(node ast.Node, g Global) (string, bool), filters ...func(node ast.Node, text string) string) string

	// GOROOT the root directory of go
	GOROOT() string

	LoadInfo() LoadInfo
}

// only used in build stage
type GlobalBuild interface {
}

type LoadInfo interface {
	Root() string
	FileSet() *token.FileSet
	MainModule() Module

	// StarterPkgs returns the pkgs
	// specified by args,
	// if test packages are loaded,
	// they are filtered for this list
	StarterPkgs() []Pkg

	// RangePkgs visit all pkgs
	RangePkgs(fn func(pkg Pkg) bool)
}

type global struct {
	fset     *token.FileSet
	loadInfo LoadInfo
	// fileMap type of map[string]*fileContent
	fileMap sync.Map

	registry Registry

	modMap map[string]Module
	pkgMap map[string]Pkg
}

var _ Global = ((*global)(nil))

// pkgs: starter packages
func NewGlobal(fset *token.FileSet, root string, pkgs []*packages.Package) Global {
	g := &global{
		fset: fset,
	}
	modMap := make(map[string]Module)
	pkgMap := make(map[string]Pkg)

	// pkgPath -> Pkg itself
	testPkgMap := make(map[string]*packages.Package)
	// pkgPath's forTest -> Pkg itself
	forTestPkgMap := make(map[string]*packages.Package)

	regBuilder := NewRegistryBuilder()
	packages.Visit(pkgs, func(pkg *packages.Package) bool {
		forTest := getForTest(pkg)
		// test package
		// if strings.HasSuffix(pkg.ID, ".test") && pkg.ID[:len(pkg.ID)-len(".test")] == pkg.PkgPath {
		if forTest != "" {
			testPkgMap[pkg.PkgPath] = pkg
			forTestPkgMap[forTest] = pkg
			return true
		}
		if pkg.Module == nil {
			// this could be caused by loadSyntax only
			// only builtin pkg allow no module
			// also, when loading test packages, a package with
			// Name=main, PkgPath={PkgPath}.test, is a generated pkg
			// we can skip such generated pkg
			if pkg.Name == "main" && strings.HasSuffix(pkg.PkgPath, ".test") {
				// skip such generated test sugar package
				return true
			}
			panic(fmt.Errorf("non-test pkg %s has no module", pkg.PkgPath))
		}

		mod := pkg.Module
		m := modMap[mod.Path]
		if m == nil {
			m = NewModule(g, mod)
			modMap[mod.Path] = m
		}
		newPkg := NewPkg(m, pkg)
		pkgMap[pkg.PkgPath] = newPkg
		regBuilder.addPkg(newPkg)
		return true
	}, nil)

	// associate test package,and tested package
	for pkgPath, p := range pkgMap {
		t := testPkgMap[pkgPath]
		if t == nil {
			// try {pkgPath}_test
			t = forTestPkgMap[pkgPath]
		}
		if t != nil {
			p := p.(*pkg)
			t := NewPkg(p.mod, t)
			t.(*pkg).testedPkg = p
			p.testPkg = t.(*pkg)
			regBuilder.addPkg(t)
		}
	}

	// filter out start pkgs
	starterPkgs := make([]Pkg, 0, len(pkgs))
	for _, pkg := range pkgs {
		forTest := getForTest(pkg)
		if forTest != "" {
			continue
		}
		// generated test pkg maybe skipped
		loadedPkg := pkgMap[pkg.PkgPath]
		if loadedPkg == nil {
			continue
		}
		starterPkgs = append(starterPkgs, loadedPkg)
	}
	// main
	mainGoMod := extractSingleMod(pkgs)

	g.modMap = modMap
	g.pkgMap = pkgMap
	g.loadInfo = &loadInfo{
		g:         g,
		root:      root,
		fset:      fset,
		startPkgs: starterPkgs,
		mainMod:   modMap[mainGoMod.Path],
	}
	g.registry = regBuilder.build()
	return g
}

func getForTest(goPkg *packages.Package) string {
	goPkgVal := reflect.ValueOf(goPkg).Elem() // ptr -> struct
	f := getUnexportedField(tryGetField(goPkgVal, "forTest"))

	// there may have three possible pkgs:
	//   1. X, forTest=""
	//   2. X.test, forTest=""
	//   3. X, forTest=X   or X_test, forTest=X, the X_test case appears when a package name ends with _test, which means if a/b/c has name X_test, then the package path appears to be a/b/c_test
	// that is, in most cases X's test pkg has path X, and forTest=X,
	// but in some case it may(maybe an edge case) the test package has an extra suffix _test
	// pkgPath := goPkg.PkgPath

	if f != nil {
		forTest := f.(string)
		if forTest != "" {
			return forTest
		}
	}

	if goPkg.Name == "main" {
		// the package is generated for linking
		return ""
	}
	// the XX_test package is for XX
	const suffix = "_test"
	if strings.HasSuffix(goPkg.PkgPath, suffix) {
		return goPkg.PkgPath[:len(goPkg.PkgPath)-len(suffix)]
	}
	// otherwise, if there are _test.go files, that's a test package
	for _, goFile := range goPkg.GoFiles {
		if strings.HasSuffix(goFile, "_test.go") {
			return goPkg.PkgPath
		}
	}
	return ""
}

func tryGetField(val reflect.Value, name string) reflect.Value {
	defer func() {
		if e := recover(); e != nil {
			// ignore
		}
	}()
	return val.FieldByName(name)
}
func getUnexportedField(field reflect.Value) interface{} {
	if !field.IsValid() {
		return nil
	}
	return reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Interface()
}

func setUnexportedField(field reflect.Value, value interface{}) {
	reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).
		Elem().
		Set(reflect.ValueOf(value))
}

// FileSet implements Global
func (c *global) FileSet() *token.FileSet {
	return c.fset
}

func (c *global) Code(n Node) string {
	return c.CodeAST(n.ASTNode())
}
func (c *global) CodeAST(n ast.Node) string {
	if f, ok := n.(*ast.File); ok {
		// special treat of file, always take full code
		f := c.fset.File(f.Pos())
		return c.FileCode(f.Name())
	}
	return c.CodeSlice(n.Pos(), n.End())
}
func (c *global) CodeSlice(begin token.Pos, end token.Pos) string {
	f := c.fset.File(begin)

	fileContent := c.FileCode(f.Name())

	i := util.OffsetOf(c.fset, begin)
	e := util.OffsetOf(c.fset, end)
	if e < 0 {
		e = len(fileContent)
	}
	return fileContent[i:e]
}
func (c *global) FileCode(absPath string) string {
	fc, _ := c.fileMap.LoadOrStore(absPath, &fileContent{file: absPath})
	return fc.(*fileContent).getContent()
}

func (c *global) GetModule(modPath string) Module {
	return c.modMap[modPath]
}

// GetPkg implements Global
func (c *global) GetPkg(pkgPath string) Pkg {
	return c.pkgMap[pkgPath]
}

// Registry implements Global
func (c *global) Registry() Registry {
	return c.registry
}

func (c *global) RangeModule(fn func(m Module) bool) {

}
func (c *global) RangePkg(fn func(pkg Pkg) bool) {
	for _, p := range c.pkgMap {
		if !fn(p) {
			return
		}
	}
}

// LoadInfo implements Global
func (c *global) LoadInfo() LoadInfo {
	return c.loadInfo
}

// RewriteAstNode implements Global
func (c *global) RewriteAstNode(node ast.Node, rewritter func(node ast.Node, g Global) (string, bool), filters ...func(node ast.Node, text string) string) string {
	hook := CombineHooksStr(filters...)
	bytes := RewriteAstNodeTextHooked(node, func(start, end token.Pos) []byte {
		return []byte(c.CodeSlice(start, end))
	}, func(node ast.Node, _ func(start token.Pos, end token.Pos) []byte) ([]byte, bool) {
		s, ok := rewritter(node, c)
		if !ok {
			return nil, false
		}
		return []byte(s), true
	}, hook)
	return string(bytes)
}

func (c *global) GOROOT() string {
	return util.GetGOROOT()
}

type loadInfo struct {
	g         Global
	root      string
	fset      *token.FileSet
	startPkgs []Pkg
	mainMod   Module
}

var _ LoadInfo = ((*loadInfo)(nil))

func extractSingleMod(starterPkgs []*packages.Package) *packages.Module {
	// debug
	// for _, p := range starterPkgs {
	// 	fmt.Printf("starter pkg:%v\n", p.PkgPath)
	// 	if p.Module != nil {
	// 		fmt.Printf("starter model:%v %v\n", p.PkgPath, p.Module.Path)
	// 	}
	// }
	var resMod *packages.Module
	for _, p := range starterPkgs {
		mod := p.Module
		if p.Module == nil {
			if util.IsGoTestPkg(p) {
				continue
			}
			panic(fmt.Errorf("package %s has no module", p.PkgPath))
		}
		if mod.Replace != nil {
			panic(fmt.Errorf("package %s has a replacement module %s, but want a self-rooted module: %s", p.PkgPath, mod.Replace.Dir, mod.Path))
		}
		if resMod == nil {
			resMod = mod
			continue
		}
		// check consistence
		if resMod != mod && resMod.Path != mod.Path {
			panic(fmt.Errorf("package %s has different module %v, want a single module:%v", p.PkgPath, mod, resMod))
		}
	}
	if resMod == nil {
		panic("no modules loaded")
	}
	return resMod
}

func (c *loadInfo) Root() string {
	return c.root
}

// FileSet implements LoadInfo
func (c *loadInfo) FileSet() *token.FileSet {
	return c.fset
}

// MainModule implements LoadInfo
func (c *loadInfo) MainModule() Module {
	return c.mainMod
}

func (c *loadInfo) StarterPkgs() []Pkg {
	return c.startPkgs
}

// RangePkgs implements LoadInfo
func (c *loadInfo) RangePkgs(fn func(pkg Pkg) bool) {
	pkgs := make([]*packages.Package, 0, len(c.startPkgs))
	for _, p := range c.startPkgs {
		pkgs = append(pkgs, p.GoPkg())
	}
	end := false
	packages.Visit(pkgs, func(p *packages.Package) bool {
		end = end || !fn(c.g.GetPkg(p.PkgPath))
		return !end
	}, nil)
}
