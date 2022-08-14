package inspect

import (
	"fmt"
	"go/ast"
)

// Visitor represents a session-associated rewrite
type Visitor interface {
	Visit(n ast.Node, session Session) bool
	VisitEnd(n ast.Node, session Session)
}

type Visitors struct {
	// VisitFn is pre action applied, returns true or false
	// to indicate whether walking its children.
	VisitFn    func(n ast.Node, session Session) bool
	VisitEndFn func(n ast.Node, session Session)
}

var _ Visitor = ((*Visitors)(nil))

// Visit implements Visitor
func (c *Visitors) Visit(n ast.Node, session Session) bool {
	if c.VisitFn == nil {
		return true
	}
	return c.VisitFn(n, session)
}

// VisitEnd implements Visitor
func (c *Visitors) VisitEnd(n ast.Node, session Session) {
	if c.VisitEndFn == nil {
		return
	}
	c.VisitEndFn(n, session)
}

// Rewritter represents a package level rewritter
type Rewritter interface {
	PackageRewritter(pkg Pkg, session Session) bool
	PackageRewritterEnd(pkg Pkg, session Session)
	FileRewritter(f FileContext, session Session) bool
	FileRewritterEnd(f FileContext, session Session)

	FuncRewritter(fn FuncContext, session Session)
	FuncRewritterEnd(fn FuncContext, session Session)
}

type Rewriters struct {
	// top level function declare
	PackageFn    func(pkg Pkg, session Session) bool
	PackageEndFn func(pkg Pkg, session Session)

	FileFn    func(f FileContext, session Session) bool
	FileEndFn func(f FileContext, session Session)

	FuncFn    func(fn FuncContext, session Session) bool
	FuncEndFn func(fn FuncContext, session Session)
}

var _ Rewritter = ((*Rewriters)(nil))

// PackageRewritter implements RW
func (c *Rewriters) PackageRewritter(pkg Pkg, session Session) bool {
	if c.PackageFn == nil {
		return true
	}
	return c.PackageFn(pkg, session)
}

// PackageRewritterEnd implements RW
func (c *Rewriters) PackageRewritterEnd(pkg Pkg, session Session) {
	if c.PackageEndFn == nil {
		return
	}
	c.PackageEndFn(pkg, session)
}

// FileRewritter implements RW
func (c *Rewriters) FileRewritter(f FileContext, session Session) bool {
	if c.FileFn == nil {
		return true
	}
	return c.FileFn(f, session)
}

// FileRewritterEnd implements RW
func (c *Rewriters) FileRewritterEnd(f FileContext, session Session) {
	if c.FileEndFn == nil {
		return
	}
	c.FileEndFn(f, session)
}

// FuncRewritter implements RW
func (c *Rewriters) FuncRewritter(fn FuncContext, session Session) {
	if c.FuncFn == nil {
		return
	}
	c.FuncFn(fn, session)
}

// FuncRewritterEnd implements RW
func (c *Rewriters) FuncRewritterEnd(fn FuncContext, session Session) {
	if c.FuncEndFn == nil {
		return
	}
	c.FuncEndFn(fn, session)
}

type stackVisitor struct {
	v       Visitor
	session Session

	// root is package, then file
	stack []ast.Node
}

// Visit implements ast.Visitor
func (c *stackVisitor) Visit(node ast.Node) (w ast.Visitor) {
	if node == nil {
		// back
		last := c.stack[len(c.stack)-1]
		c.stack = c.stack[:len(c.stack)-1]
		c.v.Visit(last, c.session)
		return
	}
	if !c.v.Visit(node, c.session) {
		return nil
	}
	c.stack = append(c.stack, node)
	return c
}

func VisitAll(pkgs func(func(pkg Pkg) bool), session Session, visitor Visitor) {
	st := &stackVisitor{
		v:       visitor,
		session: session,
	}
	// traverse all packages
	pkgs(func(p Pkg) bool {
		ast.Walk(st, p.ASTNode())
		if len(st.stack) != 0 {
			panic(fmt.Errorf("internal error, expect empty stack,actual:%d", len(st.stack)))
		}
		return true
	})
}

// RewritePackages
// func RewritePackages(pkgs func(func(pkg Pkg) bool), session Session, rw Rewritter) {
// 	// traverse all packages
// 	pkgs(func(p Pkg) bool {
// 		if !rw.PackageRewritter(p, session) {
// 			return true
// 		}
// 		p.RangeFiles(func(i int, f FileContext) bool {
// 			if !rw.FileRewritter(f, session) {
// 				return true
// 			}
// 			ast.Inspect(f.AST(), func(n ast.Node) bool {
// 				switch n := n.(type) {
// 				case *ast.File:
// 					return true
// 				case *ast.FuncDecl:
// 					fn := NewFunc(f, n)
// 					rw.FuncRewritter(fn, session)
// 					rw.FuncRewritterEnd(fn, session)
// 					return false
// 				default:
// 					// other types, we are not interested
// 					return false
// 				}
// 			})
// 			rw.FileRewritterEnd(f, session)
// 			return true
// 		})

// 		rw.PackageRewritterEnd(p, session)
// 		return true
// 	})
// }
