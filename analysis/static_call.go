package analysis

import (
	"go/ast"
	"go/token"
	"path"
	"strings"

	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/pointer"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"

	"github.com/xhd2015/go-inspect/inspect"
	"github.com/xhd2015/go-inspect/inspect/load"
)

// StaticAnalyseResult see pointer.Result
type FuncID = int
type StaticAnalyseResult struct {
	Callgraph *CallGraph
	Warnings  []*Warning
}
type CallGraph struct {
	Root  FuncID
	Funcs map[FuncID]*Func // FuncID: 0-based
}
type Func struct {
	Pkg            string
	File           string
	ID             FuncID
	Pos            *FilePos // the beginning position
	End            *FilePos // the end
	Name           string   // optional name
	QuanlifiedName string   // <type>.<name>, can be used as key inside a package to retrieve function

	// call info
	In  []*Edge
	Out []*Edge
}

type Edge struct {
	Caller FuncID
	Site   *Pos // may be nil
	Callee FuncID
}
type Warning struct {
	Pos     *Pos
	Message string
}

type Pos struct {
	Pkg  string
	File string
	*FilePos
}
type FilePos struct {
	Offset int
	Line   int
	Column int
}

type PkgPath = string
type ShortFile = string

type StaticCallResult struct {
	Pkgs map[PkgPath]*StaticCallPkg // pkg
}
type StaticCallPkg struct {
	Files map[ShortFile]*StaticCallFile
}
type StaticCallFile struct {
	Funcs map[string]*StaticCallFunc
}
type StaticCallFunc struct {
	// only named functions
	Name string
}

func LoadCallGraph(args []string, opts *load.LoadOptions) (g inspect.Global, res *StaticAnalyseResult, err error) {
	g, err = load.LoadPackages(args, opts)
	if err != nil {
		return
	}

	pkgs := g.LoadInfo().StarterPkgs()
	var loadPkgs []*packages.Package
	for _, p := range pkgs {
		loadPkgs = append(loadPkgs, p.GoPkg())
	}

	// only load packages of the same project
	mod0 := g.LoadInfo().MainModule()
	g.RangePkg(func(pkg inspect.Pkg) bool {
		if pkg.Module() != mod0 {
			p := pkg.GoPkg()
			// unset these two fields tells
			// pointer analyse to skip them,
			// which saves lots of memory and
			// time to wait.
			p.Syntax = nil
			p.TypesInfo = nil
		}
		return true
	})
	res, err = FindCallGraph(g, loadPkgs)
	return
}

func FindCallGraph(g inspect.Global, mainPkgs []*packages.Package) (*StaticAnalyseResult, error) {
	prog, ssaPkgs := ssautil.AllPackages(mainPkgs, 0)
	// NOTE: Build() must be called to get full relationship
	// otherwise only main pkg will appear
	prog.Build()

	config := &pointer.Config{
		Mains:          ssaPkgs,
		BuildCallGraph: true,
	}

	ssaRes, err := pointer.Analyze(config)
	if err != nil {
		return nil, err
	}
	return BuildCallGraphFromSSAResult(g, ssaRes), nil
}

func BuildCallGraphFromSSAResult(g inspect.Global, ssaRes *pointer.Result) *StaticAnalyseResult {
	return &StaticAnalyseResult{
		Callgraph: buildCallGraph(g, ssaRes.CallGraph),
		// Warnings:  buildWarnings(g, ssaRes.Warnings), // don't build warnings because we know there are much of them due to delete of AST fles
	}
}

func buildCallGraph(g inspect.Global, ssaGraph *callgraph.Graph) *CallGraph {
	funcs := make(map[FuncID]*Func, len(ssaGraph.Nodes))

	ssaMap := make(map[*ssa.Function]FuncID, len(ssaGraph.Nodes))

	// first build a id->func map
	funcID := FuncID(0)
	for ssaFunc := range ssaGraph.Nodes {
		fn := buildFunc(g, ssaFunc)
		if fn == nil {
			continue
		}
		fn.ID = funcID
		funcs[funcID] = fn
		ssaMap[ssaFunc] = funcID
		funcID++
	}

	// then build call graph
	for ssaFunc, ssaNode := range ssaGraph.Nodes {
		funcID, ok := ssaMap[ssaFunc]
		if !ok {
			continue
		}
		fn := funcs[funcID]
		fn.In = buildEdges(g, ssaNode.In, ssaMap)
		fn.Out = buildEdges(g, ssaNode.Out, ssaMap)
	}

	return &CallGraph{
		Root:  ssaMap[ssaGraph.Root.Func],
		Funcs: funcs,
	}
}
func buildFunc(g inspect.Global, ssaFunc *ssa.Function) *Func {
	if ssaFunc.Synthetic != "" {
		return nil
	}
	// the node is created by ssa, only position reserved
	ssaAstNode := ssaFunc.Syntax()
	if ssaAstNode == nil {
		return nil
	}

	// the astNode of ssaFunc is either a declared function, or a function lit
	// e.g.: *ast.FuncLit, *ast.FuncDecl
	astNode := g.Registry().GetNodeByPos(ssaAstNode.Pos(), ssaAstNode.End())
	if astNode == nil {
		return nil
	}

	fpos := getPosition(g, astNode.Pos())

	// only load current project
	if !strings.HasPrefix(fpos.Filename, g.LoadInfo().MainModule().Dir()) {
		return nil
	}
	endPos := getPosition(g, astNode.End())

	var fnName string
	var qname string
	declNode, _ := astNode.(*ast.FuncDecl)
	if declNode != nil {
		fctx := g.Registry().FuncDecl(declNode)
		fnName = fctx.Name()
		qname = fctx.QuanlifiedName()
	}

	pkg, fileName := getFileInfo(g, fpos)
	return &Func{
		Pkg:            pkg,
		File:           fileName,
		Pos:            getFilePos(fpos),
		End:            getFilePos(endPos),
		Name:           fnName,
		QuanlifiedName: qname,
	}
}
func buildEdges(g inspect.Global, ssaEdges []*callgraph.Edge, ssaMap map[*ssa.Function]FuncID) []*Edge {
	edges := make([]*Edge, 0, len(ssaEdges))
	for _, edge := range ssaEdges {
		edges = append(edges, buildEdge(g, edge, ssaMap))
	}
	return edges
}
func buildEdge(g inspect.Global, edge *callgraph.Edge, ssaMap map[*ssa.Function]FuncID) *Edge {
	var site *Pos
	if edge.Site != nil {
		site = buildPos(g, getPosition(g, edge.Site.Pos()))
	}
	return &Edge{
		Caller: ssaMap[edge.Caller.Func],
		Site:   site,
		Callee: ssaMap[edge.Callee.Func],
	}
}

func buildWarnings(g inspect.Global, warnings []pointer.Warning) []*Warning {
	resWarnings := make([]*Warning, 0, len(warnings))
	for _, warning := range warnings {
		newWarning := buildWarning(g, &warning)
		if newWarning == nil {
			continue
		}
		resWarnings = append(resWarnings, newWarning)
	}
	return resWarnings
}
func buildWarning(g inspect.Global, warning *pointer.Warning) *Warning {
	fpos := getPosition(g, warning.Pos)
	if !fpos.IsValid() {
		return nil
	}
	absFile := fpos.Filename
	// only related to current module
	if !strings.HasPrefix(absFile, g.LoadInfo().MainModule().Dir()) {
		return nil
	}
	return &Warning{
		Pos:     buildPos(g, fpos),
		Message: warning.Message,
	}
}

func getPosition(g inspect.Global, pos token.Pos) *token.Position {
	fpos := g.FileSet().Position(pos)
	return &fpos
}
func buildPos(g inspect.Global, fpos *token.Position) *Pos {
	if !fpos.IsValid() {
		return nil
	}
	pkg, fileName := getFileInfo(g, fpos)

	return &Pos{
		Pkg:     pkg,
		File:    fileName,
		FilePos: getFilePos(fpos),
	}
}

func getFilePos(fpos *token.Position) *FilePos {
	if !fpos.IsValid() {
		return nil
	}
	return &FilePos{
		Offset: fpos.Offset,
		Line:   fpos.Line,
		Column: fpos.Column,
	}
}
func getFileInfo(g inspect.Global, fpos *token.Position) (pkg string, fileName string) {
	if !fpos.IsValid() {
		return
	}
	absFile := fpos.Filename

	mainMod := g.LoadInfo().MainModule()
	mainDir := mainMod.Dir()
	if !strings.HasPrefix(absFile, mainDir) {
		return
	}
	subFile := strings.TrimPrefix(absFile[len(mainDir):], "/")
	dirName, baseName := path.Split(subFile)

	pkg = strings.TrimSuffix(mainMod.Path()+"/"+dirName, "/")
	fileName = baseName
	return
}
