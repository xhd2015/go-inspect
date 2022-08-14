package inspect

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"

	inspect "github.com/xhd2015/go-inspect/inspect"
	"github.com/xhd2015/go-inspect/inspect/util"
)

func NewMockRewritter(opts *RewriteOptions) inspect.Visitor {
	starterTypesMapping := make(map[types.Type]bool)
	starterTypes := make([]types.Type, 0)

	addType := func(t types.Type) {
		if starterTypesMapping[t] {
			return
		}
		starterTypesMapping[t] = true
		starterTypes = append(starterTypes, t)
	}

	type PkgPath = string

	details := make(map[PkgPath][]*RewriteFileDetail)
	fileDetailMap := make(map[inspect.FileContext]*RewriteFileDetail)
	return &inspect.Visitors{
		VisitFn: func(n ast.Node, session inspect.Session) bool {
			if p, ok := n.(*ast.Package); ok {
				pkg := session.Global().Registry().Pkg(p)
				return pkg.GoPkg().Types.Scope().Lookup(SKIP_MOCK_PKG) == nil
			}

			if astF, ok := n.(*ast.File); ok {
				if astF.Scope.Lookup(SKIP_MOCK_FILE) != nil {
					return false
				}
				f := session.Global().Registry().File(astF)

				// the token may be loaded from cached file
				// which means there is no change in the content
				// so just skip it.
				// "/Users/xhd2015/Library/Caches/go-build/b9/b922abe0d6b605b09d7d9c1439988dc01564a743e3bcfd403e491bb07a4a7f22-d"
				// the simplest workaround is to detect if it ends with ".go"
				// NOTE: there may exists both gofiles and cacehd files for one package
				// ignoring cached files does not affect correctness.(TODO: check if it is really a cachedd build file)
				if !f.IsGoFile() || f.IsTestGoFile() /*skip test file: x_test.go */ {
					return false
				}
				d := &RewriteFileDetail{}
				details[f.Pkg().Path()] = append(details[f.Pkg().Path()], d)
				fileDetailMap[f] = d
				return true
			}

			if astFunc, ok := n.(*ast.FuncDecl); ok {
				fn := session.Global().Registry().FuncDecl(astFunc)
				fr := session.FileRewrite(fn.File())

				fd := fileDetailMap[fn.File()]
				RewriteFuncMock(fn, opts, fr, addType, func(detail *rewriteFuncDetail) {
					fd.Funcs = append(fd.Funcs, detail)
				})
			}

			return true
		},
		VisitEndFn: func(n ast.Node, session inspect.Session) {
			if p, ok := n.(*ast.Package); ok {
				pkg := session.Global().Registry().Pkg(p)
				genMockStub(pkg.GoPkg(), session.PackageEdit(pkg, "mock_stub"), details[pkg.Path()])
			}
			if astF, ok := n.(*ast.File); ok {
				f := session.Global().Registry().File(astF)
				fr := session.FileRewrite(f)
				regCode := genRegCode(fileDetailMap[f].Funcs, f.Pkg().Path(), fr)
				fr.AddAnaymouseInit(regCode)
			}
		},
	}
}
func NewInitRewritter() inspect.Visitor {
	return &inspect.Visitors{
		VisitFn: func(n ast.Node, session inspect.Session) bool {
			switch n := n.(type) {
			case *ast.Package:
				pkg := session.Global().Registry().Pkg(n)
				if pkg.Module().IsStd() {
					return false
				}

				edit := session.PackageEdit(pkg, "init_print")
				fmtPkg := edit.MustImport("fmt", "fmt", "", nil)
				edit.AddAnaymouseInit(fmt.Sprintf(`var _ = func() bool {%s.Println("init",%q);return true;}()`, fmtPkg, pkg.Path()))
				return false
			}
			return true
		},
	}
}

func RewriteFuncMock(fn inspect.FuncContext, opts *RewriteOptions, edit inspect.GoRewriteEdit, addType func(t types.Type), addFuncDetail func(*rewriteFuncDetail)) {
	if fn.Body() == nil {
		return // external linked functions have no body
	}
	node := fn.AST()
	fileName := fn.File().AbsPath()
	pkg := fn.File().Pkg().GoPkg()
	g := fn.File().Pkg().Global()

	getContentByPos := func(start, end token.Pos) []byte {
		return []byte(g.CodeSlice(start, end))
	}

	ownerIsPtr, ownerType := fn.ParseRecvInfo()
	funcName := fn.Name()

	// package level init function cannot be mocked
	// because go allows init be defined multiple times in a file,and across files
	if ownerType == "" && funcName == "init" {
		return
	}

	if opts != nil && opts.Filter != nil && !opts.Filter(pkg.PkgPath, fileName, ownerType, ownerIsPtr, funcName) {
		return
	}
	// special case, if the function returns ctx,
	// we do not mock it as such function violatiles
	// ctx-function pair relation.
	//
	// if node.Type.Results != nil && len(node.Type.Results.List) > 0 {
	// 	for _, res := range node.Type.Results.List {
	// 		if TokenHasQualifiedName(pkg, res.Type, "context", "Context") {
	// 			return
	// 		}
	// 	}
	// }
	hasCtx := false
	fn.Type().Results().RangeVars(func(i int, f inspect.FieldVar) bool {
		if f.TypeExpr().RefersToQualified("context", "Context") {
			hasCtx = true
			return false
		}
		return true
	})
	if hasCtx {
		return
	}

	rc := initRewriteConfig(pkg, node, false /*skip no ctx*/)
	if rc == nil {
		// no ctx
		return
	}

	rc.SupportPkgRef = edit.MustImport(MOCK_PKG, "mock", "_mock", func(name string) bool {
		for _, f := range rc.AllFields {
			if f.Name == name {
				return true
			}
		}
		return false
	})

	// init all type exprs
	for _, f := range rc.AllFields {
		f.TypeExprString = g.CodeSlice(f.TypeExpr.Pos(), f.TypeExpr.End())
	}
	rc.Init()

	// rewrite names
	// rc.AllFields.RenameFields(fset, buf)
	fn.RangeVars(func(i int, f inspect.FieldVar) bool {
		f.Rename(rc.AllFields[i].Name, edit)
		return true
	})

	// get code of original result
	// var originalResults string
	// if node.Type.Results != nil && len(node.Type.Results.List) > 0 {
	// 	if node.Type.Results.Opening == token.NoPos {
	// 		// add ()
	// 		buf.Insert(OffsetOf(fset, node.Type.Results.Pos())-1, "(")
	// 		buf.Insert(OffsetOf(fset, node.Type.Results.End()), ")")
	// 	}
	// 	originalResults = string(getContent(fset, content, node.Type.Results.Pos(), node.Type.Results.End()))
	// }
	var originalResults string
	if fn.Type().Results().Len() > 0 {
		resNode := fn.Type().Results().AST()
		if resNode.Opening == token.NoPos {
			edit.Insert(resNode.Pos()-1, "(")
			edit.Insert(resNode.End(), ")")
		}
	}

	recvCode := ""
	// if node.Recv != nil {
	// 	recvCode = string(getContent(fset, content, node.Recv.Pos(), node.Recv.End()))
	// }
	if fn.Recv() != nil {
		recvCode = g.Code(fn.Recv())
	}

	// fix mixed names:
	// because we are combining recv+args,
	// so if either has no name, we must given a name _
	// var args string
	args := g.CodeSlice(node.Type.Params.Pos(), node.Type.Params.End())
	if rc.Recv != nil && len(rc.FullArgs) > 0 &&
		(rc.Recv.OrigName == "" && rc.FullArgs[0].OrigName != "" ||
			rc.Recv.OrigName != "" && rc.FullArgs[0].OrigName == "") {
		if rc.FullArgs[0].OrigName == "" {

			// args has no name, but recv has name
			typePrefixMap := make(map[ast.Node]string, 1)
			for _, arg := range rc.FullArgs {
				if arg.OrigName == "" {
					typePrefixMap[arg.TypeExpr] = "_ "
				}
			}
			hook := func(node ast.Node, c string) string {
				prefix, ok := typePrefixMap[node]
				if !ok {
					return c
				}
				return prefix + c
			}
			args = g.RewriteAstNode(node.Type.Params, nil, hook)
		}
		args = joinRecvArgs(recvCode, args, rc.Recv.OrigName, true, len(node.Type.Params.List))
	} else {
		if node.Recv != nil {
			args = joinRecvArgs(recvCode, args, rc.Recv.OrigName, false, len(node.Type.Params.List))
		}
	}

	// generate patch content and insert
	newCode := rc.Gen(false /*pretty*/)
	patchContent := fmt.Sprintf(`%s}; func %s%s%s{`, newCode, rc.NewFuncName, util.StripNewline(args), util.StripNewline(originalResults))

	// buf.Insert(OffsetOf(fset, node.Body.Lbrace)+1, patchContent)
	edit.Insert(node.Body.Lbrace+1, patchContent)

	// make rewriteDetails
	// funcDetails = append(funcDetails,
	addFuncDetail(
		&rewriteFuncDetail{
			File:          fileName,
			RewriteConfig: rc,

			ArgsRewritter: func(r inspect.AstNodeRewritter, hook func(node ast.Node, c []byte) []byte) (argsRepkg string) {
				argsRepkg = string(inspect.RewriteAstNodeTextHooked(node.Type.Params, getContentByPos, r, hook))
				if node.Recv != nil {
					// for exported name, should add package prefix for it.
					// unexported type has interface{}
					recvCode = string(inspect.RewriteAstNodeTextHooked(node.Recv, getContentByPos, r, hook))

					argsRepkg = joinRecvArgs(recvCode, argsRepkg, rc.Recv.OrigName, false, len(node.Type.Params.List))
				}
				return
			},
			ResultsRewritter: func(r inspect.AstNodeRewritter, hook func(node ast.Node, c []byte) []byte) (resultsRepkg string) {
				if node.Type.Results != nil && len(node.Type.Results.List) > 0 {
					resultsRepkg = string(inspect.RewriteAstNodeTextHooked(node.Type.Results, getContentByPos, r, hook))
				}
				return
			},
		})

	// add starter types
	// starter types are entrance for export
	for _, arg := range rc.AllFields {
		addType(arg.Type.ResolvedType)
	}
}
