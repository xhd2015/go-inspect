package inspect

import (
	"fmt"
	"go/ast"
	"go/token"
	"sync"
)

// Registry holds ast mapping in a load info
//
// This is a pattern we used, we call it
//
// Each AST node is globally unique within its load,
// this property makes it possible to associate
// extra properties to the original AST Node to provide
// convienent wrapper.
// Then the question comes: how do we get the wrapper of
// the original AST node? Answer is here: the Registry

// Registry is an immutable mapping that holds
// the ast Node to their convienent wrapper
// types appear here meaning they have special support
// it is effectively an AST node factory.
type Registry interface {
	// File reverse the look up
	// background: any AST Node must belong to a certain file
	// NOTE: except Package node.
	FileOf(node ast.Node) FileContext
	Parent(node ast.Node) ast.Node

	Pkg(node *ast.Package) Pkg
	File(node *ast.File) FileContext
	// Func
	FuncDecl(node *ast.FuncDecl) FuncContext
	FuncType(node *ast.FuncType) FuncType

	RangeNodes(fn func(node ast.Node) bool)
	// expose a Node map keyed by position
	GetNodeByPos(start token.Pos, end token.Pos) ast.Node
}

type registryBuilder struct {
	r     *registry
	stack []ast.Node
}

func NewRegistryBuilder() *registryBuilder {
	return &registryBuilder{
		r: &registry{
			parentMap: make(map[ast.Node]ast.Node),
			nodeMap:   make(map[ast.Node]Node),
			pkgMap:    make(map[*ast.Package]Pkg),
			fileMap:   make(map[*ast.File]FileContext),
		},
	}
}
func (c *registryBuilder) addPkgs(pkgs func(func(pkg Pkg) bool)) {
	pkgs(func(pkg Pkg) bool {
		c.addPkg(pkg)
		return true
	})
}

func (c *registryBuilder) addPkg(pkg Pkg) {
	c.r.pkgMap[pkg.AST()] = pkg
	pkg.RangeFiles(func(i int, f FileContext) bool {
		c.r.fileMap[f.AST()] = f

		TraverseNodeParentMap(f.AST(), func(n, parent ast.Node) bool {
			c.r.parentMap[n] = parent
			return true
		})

		if len(c.stack) != 0 {
			panic(fmt.Errorf("internal error: stack not balanced"))
		}
		return true
	})
}

func (c *registryBuilder) build() Registry {
	return NewRegistry(c.r.parentMap, c.r.nodeMap, c.r.pkgMap, c.r.fileMap)
}

type registry struct {
	parentMap map[ast.Node]ast.Node
	nodeMap   map[ast.Node]Node
	pkgMap    map[*ast.Package]Pkg
	fileMap   map[*ast.File]FileContext

	posMapInit sync.Once
	posMap     posMap
}

var _ Registry = ((*registry)(nil))

// NewRegistry must
func NewRegistry(parentMap map[ast.Node]ast.Node, nodeMap map[ast.Node]Node, pkgMap map[*ast.Package]Pkg, fileMap map[*ast.File]FileContext) Registry {
	return &registry{
		parentMap: parentMap,
		nodeMap:   nodeMap,
		pkgMap:    pkgMap,
		fileMap:   fileMap,
	}
}

// FileOf implements Registry
func (c *registry) FileOf(node ast.Node) FileContext {
	return c.File(c.mustFileOf(node))
}

// Parent implements Registry
func (c *registry) Parent(node ast.Node) ast.Node {
	return c.parentMap[node]
}

func (c *registry) Pkg(node *ast.Package) Pkg {
	return c.pkgMap[node]
}

func (c *registry) File(node *ast.File) FileContext {
	return c.fileMap[node]
}

// Func implements Registry
func (c *registry) FuncDecl(node *ast.FuncDecl) FuncContext {
	if node == nil {
		return nil
	}
	f := c.nodeMap[node]
	if f == nil {
		f := NewFunc(c.fileMap[c.mustFileOf(node)], node)
		c.nodeMap[node] = f
		return f
	}
	return f.(FuncContext)
}

// Func implements Registry
func (c *registry) FuncType(node *ast.FuncType) FuncType {
	if node == nil {
		return nil
	}
	f := c.nodeMap[node]
	if f == nil {
		f := NewFuncType(c.fileMap[c.mustFileOf(node)].Pkg(), node)
		c.nodeMap[node] = f
		return f
	}
	return f.(FuncType)
}

func (c *registry) RangeNodes(fn func(node ast.Node) bool) {
	for n := range c.parentMap {
		if !fn(n) {
			return
		}
	}
}
func (c *registry) GetNodeByPos(start token.Pos, end token.Pos) ast.Node {
	c.posMapInit.Do(func() {
		c.posMap = makePos2NodeMap(c)
	})
	return c.posMap.Get(start, end)
}

func (c *registry) mustFileOf(n ast.Node) *ast.File {
	var ok bool
	var pf *ast.File
	for p := n; p != nil; p = c.parentMap[p] {
		if pf, ok = p.(*ast.File); ok {
			return pf
		}
	}
	panic(fmt.Errorf("no file found for:%v", n))
}

type posMap map[nodePosInfo]ast.Node

type nodePosInfo struct {
	start token.Pos
	end   token.Pos
}

func (c posMap) Get(start token.Pos, end token.Pos) ast.Node {
	return c[nodePosInfo{start: start, end: end}]
}

func makePos2NodeMap(reg Registry) posMap {
	posMap := make(posMap)
	reg.RangeNodes(func(node ast.Node) bool {
		posMap[nodePosInfo{start: node.Pos(), end: node.End()}] = node
		return true
	})
	return posMap
}
