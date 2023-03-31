package inspect

import (
	"fmt"
	"go/ast"
	"go/types"
	"path"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/xhd2015/go-inspect/inspect/util"
)

type Module interface {
	Global() Global
	Path() string // the path after replaced(if any)
	Dir() string  // the dir after replaced(if any)

	OrigPath() string // original non-replaced
	OrigDir() string  // original non-replaced

	// IsStd is the standard module? If so, Dir refers to GOROOT/src
	IsStd() bool
}

type Pkg interface {
	AST() *ast.Package
	ASTNode() ast.Node

	Global() Global
	Module() Module

	// Mod() Module
	Path() string // the pkg path
	Dir() string  // absolute path to directory
	Name() string // name
	GoPkg() *packages.Package
	TypePkg() *types.Package

	// TestPkg returns the {PkgPath}.test
	// parts for this pkg, if any
	TestPkg() Pkg

	// the TestedPkg returns the acutal pkg
	// that this package refers to, it this
	// package is a test package
	TestedPkg() Pkg

	// deprecated
	IsTest() bool

	RangeFiles(fn func(i int, f FileContext) bool)
}

type mod struct {
	g   Global
	mod *packages.Module
}

var _ Module = ((*mod)(nil))

func NewModule(g Global, m *packages.Module) Module {
	return &mod{
		g:   g,
		mod: m,
	}
}

// SetGlobal implements GlobalSetAware
func (c *mod) SetGlobal(g Global) {
	c.g = g
}

// Dir implements Module
func (c *mod) Dir() string {
	if c.mod.Replace != nil {
		return c.mod.Replace.Dir
	}
	return c.mod.Dir
}

// Path implements Module
func (c *mod) Path() string {
	if c.mod.Replace != nil {
		return c.mod.Replace.Path
	}
	return c.mod.Path
}

// OrigDir implements Module
func (c *mod) OrigDir() string {
	return c.mod.Dir
}

// OrigPath implements Module
func (c *mod) OrigPath() string {
	return c.mod.Path
}

// Global implements Module
func (c *mod) Global() Global {
	return c.g
}

func (c *mod) IsStd() bool {
	return util.IsStdModule(c.mod)
}

type pkg struct {
	mod   Module
	ast   *ast.Package
	goPkg *packages.Package

	testPkg   *pkg
	testedPkg *pkg

	files []FileContext
}

var _ Pkg = ((*pkg)(nil))

func NewPkg(mod Module, goPkg *packages.Package) Pkg {
	p := &pkg{
		mod:   mod,
		goPkg: goPkg,
		ast: &ast.Package{
			Name: goPkg.Name,
		},
	}
	files := make([]FileContext, 0, len(goPkg.Syntax))
	astFiles := make(map[string]*ast.File, len(goPkg.Syntax))
	for _, astFile := range goPkg.Syntax {
		f := NewFile(p, astFile)
		files = append(files, f)
		astFiles[f.AbsPath()] = astFile
	}
	p.files = files
	p.ast.Files = astFiles
	return p
}

// TestPkg implements Pkg
func (c *pkg) TestPkg() Pkg {
	// why this fucking if?
	// because c.testPkg has type *pkg, while
	// the return type is Pkg,
	// if return c.testPkg directly,
	// c.TestPkg()==nil will always be false,
	// but access to that will cause NPE
	if c.testPkg == nil {
		return nil
	}
	return c.testPkg
}
func (c *pkg) TestedPkg() Pkg {
	if c.testedPkg == nil {
		return nil
	}
	return c.testedPkg
}

// AST implements Pkg
func (c *pkg) AST() *ast.Package {
	return c.ast
}

// ASTNode implements Pkg
func (c *pkg) ASTNode() ast.Node {
	return c.ast
}

// Dir implements Pkg
func (c *pkg) Dir() string {
	// if replaced, will have a different mod path
	origModPath := c.mod.OrigPath()
	modDir := c.mod.Dir()

	pkgPath := c.Path()
	if origModPath == pkgPath {
		return modDir
	}
	if strings.HasPrefix(pkgPath, origModPath) {
		return path.Join(modDir, pkgPath[len(origModPath):])
	}
	panic(fmt.Errorf("%s not child of %s", pkgPath, origModPath))
}

// Global implements Pkg
func (c *pkg) Global() Global {
	return c.mod.Global()
}

func (c *pkg) Module() Module {
	return c.mod
}

// GoPkg implements Pkg
func (c *pkg) GoPkg() *packages.Package {
	return c.goPkg
}

// TypePkg implements Pkg
func (c *pkg) TypePkg() *types.Package {
	return c.goPkg.Types
}

// Name implements Pkg
func (c *pkg) Name() string {
	return c.goPkg.Name
}

// Path implements Pkg
func (c *pkg) Path() string {
	return c.goPkg.PkgPath
}

// IsTest implements Pkg
func (c *pkg) IsTest() bool {
	return util.IsGoTestPkg(c.goPkg)
}

// RangeFiles implements Pkg
func (c *pkg) RangeFiles(fn func(i int, f FileContext) bool) {
	for i, f := range c.files {
		if !fn(i, f) {
			return
		}
	}
}
