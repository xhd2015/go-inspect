package analysis

import (
	"fmt"
	"go/ast"
	"log"
	"strings"

	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/pointer"
	"golang.org/x/tools/go/ssa/ssautil"

	"github.com/xhd2015/go-inspect/inspect"
	"github.com/xhd2015/go-inspect/inspect/load"
)

type Matcher interface {
	Match(g inspect.Global, decl *ast.FuncDecl /*parent decl*/, lit *ast.FuncLit /*func decl,or func literal*/, n ast.Node) bool

	Include(g inspect.Global, n ast.Node) bool
}

type Matchers struct {
	MatchFunc   func(g inspect.Global, decl *ast.FuncDecl /*parent decl*/, lit *ast.FuncLit /*func decl,or func literal*/, n ast.Node) bool
	IncludeFunc func(g inspect.Global, n ast.Node) bool
}

// Include implements Matcher
func (c *Matchers) Include(g inspect.Global, n ast.Node) bool {
	if c.IncludeFunc == nil {
		return true
	}
	return c.IncludeFunc(g, n)
}

// Match implements Matcher
func (c *Matchers) Match(g inspect.Global, decl *ast.FuncDecl, lit *ast.FuncLit, n ast.Node) bool {
	if c.MatchFunc == nil {
		return false
	}
	return c.MatchFunc(g, decl, lit, n)
}

var _ Matcher = ((*Matchers)(nil))

type Opts struct {
	Include func(n ast.Node) bool
}
type MatchFunc func(g inspect.Global, decl *ast.FuncDecl /*parent decl*/, lit *ast.FuncLit /*func decl,or func literal*/) bool

func LoadMatch(args []string, m Matcher, opts *load.LoadOptions) (g inspect.Global, nodeMap map[ast.Node]bool, err error) {
	g, err = load.LoadPackages(args, opts)
	if err != nil {
		return
	}

	pkgs := g.LoadInfo().StarterPkgs()
	var loadPkgs []*packages.Package
	for _, p := range pkgs {
		loadPkgs = append(loadPkgs, p.GoPkg())
	}

	nodeMap, err = FindMatch(g, loadPkgs, m)
	return
}

func FindMatch(g inspect.Global, loadPkgs []*packages.Package, m Matcher) (nodeMap map[ast.Node]bool, err error) {
	prog, ssaPkgs := ssautil.AllPackages(loadPkgs, 0)
	// NOTE: Build() must be called to get full relationship
	// otherwise only main pkg will appear
	prog.Build()

	config := &pointer.Config{
		Mains:          ssaPkgs,
		BuildCallGraph: true,
	}

	res, err := pointer.Analyze(config)
	if err != nil {
		return
	}

	// each function has a unique position
	// but pkgName + fileName + fileOffset is must more stable.
	type fnInfo struct {
		pkg  inspect.Pkg
		f    inspect.FileContext
		decl *ast.FuncDecl
		lit  *ast.FuncLit
	}
	var fns []*fnInfo

	// find entry functions by matching
	// pkg,file,func, and body
	g.RangePkg(func(pkg inspect.Pkg) bool {
		pkg.RangeFiles(func(i int, f inspect.FileContext) bool {
			// FuncLit or FuncDecl
			decls := f.AST().Decls
			var fdec *ast.FuncDecl

			tr := func(n ast.Node) bool {
				var lit *ast.FuncLit
				switch n := n.(type) {
				case *ast.FuncDecl:
					// proceed
				case *ast.FuncLit:
					lit = n

				default:
					return true
				}
				if m.Match(g, fdec, lit, n) {
					fns = append(fns, &fnInfo{
						pkg:  pkg,
						f:    f,
						decl: fdec,
						lit:  lit,
					})
				}
				return true
			}

			for _, decl := range decls {
				var ok bool
				fdec, ok = decl.(*ast.FuncDecl) // make parent
				if !ok {
					continue
				}
				ast.Inspect(decl, tr)
			}

			// traverse lit
			fdec = nil
			for _, decl := range decls {
				_, ok := decl.(*ast.FuncDecl)
				if ok {
					continue
				}
				ast.Inspect(decl, tr)
			}
			return true
		})
		return true
	})

	_ = prog

	// dependencyMap contains direct and indirect dependencies
	// in flatten style.
	dependencyMap := make(map[*callgraph.Node]map[*callgraph.Node]bool)

	origAstNode := func(n ast.Node) ast.Node {
		if n == nil {
			return nil
		}
		return g.Registry().GetNodeByPos(n.Pos(), n.End())
	}

	// TODO: handle cyclic reference
	astToSSA := make(map[ast.Node]*callgraph.Node)
	ssaToAST := make(map[*callgraph.Node]ast.Node)
	var flatten func(n *callgraph.Node) map[*callgraph.Node]bool
	flatten = func(n *callgraph.Node) map[*callgraph.Node]bool {
		// skip
		if n.Func.Synthetic != "" {
			return nil
		}
		if found, ok := dependencyMap[n]; ok {
			return found
		}
		res := make(map[*callgraph.Node]bool, len(n.Out))
		dependencyMap[n] = res // prevent infinite loop
		for _, out := range n.Out {
			if out.Callee.Func.Synthetic == "" {
				res[out.Callee] = true
			}

			for k, v := range flatten(out.Callee) {
				if k.Func.Synthetic == "" {
					res[k] = v
				}
			}
		}

		ast := origAstNode(n.Func.Syntax())
		if ast == nil {
			panic(fmt.Errorf("unexpected nil ast"))
		}
		astToSSA[ast] = n
		ssaToAST[n] = ast
		return res
	}
	debug := false
	for _, n := range res.CallGraph.Nodes {
		flatten(n)

		// TODO: comment out
		if debug {
			ast := origAstNode(n.Func.Syntax())
			if ast == nil {
				continue
			}

			f := g.Registry().FileOf(ast)
			c := g.CodeSlice(ast.Pos(), ast.End())
			if !strings.Contains(c, "Run") {
				continue
			}

			a := c
			if !strings.Contains(f.AbsPath(), "analysis/testdata") {
				continue
			}

			a = c
			_ = a
		}
	}

	// for each entry, mark their direct and indirect depedencies as match
	nodeMap = make(map[ast.Node]bool)
	for _, fn := range fns {
		// buggy:
		// var node ast.Node = fn.lit
		// if node == nil {
		// 	node = fn.decl
		// }

		var node ast.Node
		if fn.lit != nil {
			node = fn.lit
		} else {
			node = fn.decl
		}

		nodeMap[node] = true
		ssaNode := astToSSA[node]
		if ssaNode == nil {
			// ssa node not found, that is ok
			// because only toplevel named functions are built
			// err = fmt.Errorf("ssa node not found:%+v", node)
			log.Printf("ssa node not found:%v", g.CodeSlice(node.Pos(), node.End()))
			continue
		}
		for n := range dependencyMap[ssaNode] {
			astNode := ssaToAST[n]
			if astNode == nil {
				err = fmt.Errorf("ast node not found:%+v", n)
				continue
			}
			nodeMap[astNode] = true
		}
	}
	// filter
	for n := range nodeMap {
		if !m.Include(g, n) {
			delete(nodeMap, n)
		}
	}

	return
}
