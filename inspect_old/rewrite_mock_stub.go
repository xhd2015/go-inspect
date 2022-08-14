package inspect

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"github.com/xhd2015/go-inspect/code/gen"
	inspect "github.com/xhd2015/go-inspect/inspect"
	"golang.org/x/tools/go/packages"
)

func genMockStub(p *packages.Package, edit inspect.GoNewEdit, fileDetails []*RewriteFileDetail) {
	preMap := map[string]bool{
		"Setup":         true,
		"M":             true,
		SKIP_MOCK_FILE:  true,
		SKIP_MOCK_PKG:   true,
		"FULL_PKG_NAME": true, // TODO: may add go keywords.
	}

	forbidden := func(name string) bool {
		return preMap[name]
	}

	var links gen.Statements
	var defs gen.Statements

	type decAndLink struct {
		decl *gen.Statements
		link *gen.Statements
	}
	defByOwner := make(map[string]*decAndLink)

	// var rePkg AstNodeRewritter
	codeWillBeCommented := false
	var interfacedIdent map[ast.Node]bool
	// rePkg, and also given a name
	rePkg := func(node ast.Node, getNodeText func(start token.Pos, end token.Pos) []byte) ([]byte, bool) {
		_, ok := interfacedIdent[node]
		if ok {
			return []byte("interface{}"), true
		}
		if idt, ok := node.(*ast.Ident); ok {
			ref := p.TypesInfo.Uses[idt]
			// is it a Type declared in current package?
			if t, ok := ref.(*types.TypeName); ok {
				realPkg := t.Pkg()  // may be dot import
				if realPkg != nil { // string will have no pkg
					refPkgName := realPkg.Name()
					if !codeWillBeCommented {
						refPkgName = edit.MustImport(realPkg.Path(), realPkg.Name(), "", forbidden)
					}
					return []byte(fmt.Sprintf("%s.%s", refPkgName, idt.Name)), true
				}
			}
		} else if sel, ok := node.(*ast.SelectorExpr); ok {
			// external pkg
			// debugShow(p.TypesInfo, sel)
			if idt, ok := sel.X.(*ast.Ident); ok {
				ref := p.TypesInfo.Uses[idt]
				if pkgName, ok := ref.(*types.PkgName); ok {
					extPkgName := pkgName.Name()
					if !codeWillBeCommented {
						extPkgName = edit.MustImport(pkgName.Imported().Path(), pkgName.Imported().Name(), pkgName.Name(), forbidden)
					}
					return []byte(fmt.Sprintf("%s.%s", extPkgName, sel.Sel.Name)), true
				}
			}
		}
		return nil, false
	}

	noOwnerDef := &decAndLink{decl: &gen.Statements{}, link: &gen.Statements{}}
	defs.Append(noOwnerDef.decl)
	links.Append(noOwnerDef.link)
	defByOwner[""] = noOwnerDef
	hasRefX := false
	for _, fd := range fileDetails {
		for _, d := range fd.Funcs {
			rc := d.RewriteConfig

			oname := ""
			if rc.Owner != "" {
				oname = rc.Owner
				if !rc.Recv.Type.Exported {
					oname = "M_" + oname
				}
			}

			// don't add mock stub for invisible functions
			// because the user cannot easily reference invisible
			// functions
			// we add a comment here
			defSt, ok := defByOwner[rc.Owner]
			if !ok {
				// create for new owner
				defSt = &decAndLink{decl: &gen.Statements{}, link: &gen.Statements{}}
				if rc.Owner != "" {
					defs.Append(
						fmt.Sprintf("    %s struct{", oname),
						gen.Indent("        ", defSt.decl),
						"    }",
					)
					links.Append(
						fmt.Sprintf(`    "%s":map[string]interface{}{`, oname),
						gen.Indent("         ", defSt.link),
						"    },",
					)
				} else {
					defs.Append(gen.Indent("    ", defSt.decl))
					links.Append(gen.Indent("    ", defSt.link))
				}
				// always append impl
				defByOwner[rc.Owner] = defSt
			}

			refFuncName := rc.FuncName
			if !rc.Exported {
				refFuncName = "M_" + refFuncName
			}
			codeWillBeCommented = !rc.FullArgs.AllTypesVisible() || !rc.FullResults.AllTypesVisible()
			interfacedIdent = map[ast.Node]bool(nil)

			var renameHook func(node ast.Node, c []byte) []byte
			if rc.Recv != nil && len(rc.FullArgs) > 0 &&
				(rc.Recv.OrigName == "" && rc.FullArgs[0].OrigName != "" || rc.Recv.OrigName != "" && rc.FullArgs[0].OrigName == "") {
				prefixMap := make(map[ast.Node][]byte, 1)
				// args has no name, but recv has name
				for _, arg := range rc.FullArgs {
					if arg.OrigName == "" {
						prefixMap[arg.TypeExpr] = []byte("_ ")
					}
				}
				if rc.Recv.OrigName == "" {
					prefixMap[rc.Recv.TypeExpr] = []byte("_ ")
				}
				renameHook = func(node ast.Node, c []byte) []byte {
					prefix, ok := prefixMap[node]
					if !ok {
						return c
					}
					x := append([]byte(nil), prefix...)
					return append(x, c...)
				}
			}
			if rc.Recv != nil && !rc.Recv.Type.Exported {
				if interfacedIdent == nil {
					interfacedIdent = make(map[ast.Node]bool, 1)
				}
				interfacedIdent[rc.Recv.TypeExpr] = true
			}

			args := d.ArgsRewritter(rePkg, inspect.CombineHooks(renameHook))
			results := d.ResultsRewritter(rePkg, nil)
			if codeWillBeCommented {
				// add decl statements
				list := strings.Split(fmt.Sprintf("%s func%s%s", refFuncName, args, results), "\n")
				list[len(list)-1] += fmt.Sprintf("// NOTE: %s contains invisible types", refFuncName)
				defSt.decl.Append(
					gen.Indent("//     ", list),
				)
			} else {
				defSt.decl.Append(fmt.Sprintf("    %s func%s%s", refFuncName, args, results))

				// don't add link for unexported type
				if rc.Exported && (rc.Recv == nil || rc.Recv.Type.Exported) {
					usePkgName := edit.MustImport(p.PkgPath, p.Name, "", forbidden)
					xref := ""
					ref := ""
					if rc.Recv != nil {
						// unused code, but leave here for future optimization
						if !rc.Recv.Type.Exported {
							ref = fmt.Sprintf("((*%s)(nil)).%s", rc.Recv.Type.Name /*use internal name*/, rc.FuncName)
						} else {
							ref = fmt.Sprintf("((*%s.%s)(nil)).%s", usePkgName, rc.Recv.Type.Name, rc.FuncName)
						}
						xref = fmt.Sprintf("e.%s.%s", oname, refFuncName)
					} else {
						ref = fmt.Sprintf("%s.%s", usePkgName, rc.FuncName)
						xref = fmt.Sprintf("e.%s", refFuncName)
					}
					hasRefX = true
					defSt.link.Append(fmt.Sprintf(`    "%s": Pair{%s,%s},`, refFuncName, xref, ref))
				}
			}
		}
	}

	// should traverse all types from args and results, finding referenced types:
	// - types in the same package
	//   -- exported: just delcare a name reference
	//   -- unexported: make an exported alias, and declare that
	// - types from another package
	//   -- must be exported: just import the package and name
	// - types from internal package
	// name conflictions may be processed later.
	var typeAlias []string
	if false /*need alias type*/ {
		typeAliased := make(map[string]bool)
		for _, fd := range fileDetails {
			for name, exportName := range fd.AllExportNames {
				if !typeAliased[name] {
					typeAliased[name] = true
					// TODO
					// imps.ImportOrUseNext(p.PkgPath, "", p.Name)
					typeAlias = append(typeAlias, fmt.Sprintf("type %s = %s.%s", name, p.Name, exportName))
				}
			}
		}
	}

	// import predefined packages in the end
	// we try to not rename packages.
	ctxName := edit.MustImport("context", "context", "", forbidden)
	// reflectName := imps.ImportOrUseNext("reflect", "", "reflect")
	mockName := edit.MustImport(MOCK_PKG, "mock", "_mock", forbidden)

	varMap := gen.VarMap{
		"__PKG_NAME__": p.Name,
		"__FULL_PKG__": p.PkgPath,
		"__CTXP__":     ctxName,
		// "__REFLECTP__": reflectName,
		"__MOCKP__": mockName,
	}

	edit.AddHeadCode(`// Code generated by go-mock; DO NOT EDIT.`)
	edit.SetPackageName(p.Name)

	// example
	// type M interface {
	// 	context.Context
	// 	A(a interface{}) M
	// 	B(b interface{}) M
	// }
	// type A interface {
	// 	M(ctx context.Context) M
	// }
	// var a A
	// ctx = a.M(nil).A(nil).B(nil)
	t := gen.NewTemplateBuilder()
	t.Block(
		fmt.Sprintf(`const %s = true`, SKIP_MOCK_PKG),
		`const FULL_PKG_NAME = "__FULL_PKG__"`,
		"",
		// usage:
		// ctx := mock_xx.Setup(ctx,func(ctx,t){
		//	     t.X = X
		//       t.Y = Y
		// })
		"func Setup(ctx __CTXP__.Context,setup func(m *M)) __CTXP__.Context {",
		"    m:=M{}",
		"    setup(&m)",
		`    return __MOCKP__.WithMockSetup(ctx,FULL_PKG_NAME,m)`,
		"}",
		"",
		typeAlias,
		"",
		"type M struct {",
		defs,
		"}",
		"",
		"/* prodives quick link */",
		// pre-grouped
		gen.Group(
			"var _ = func() { type Pair [2]interface{};",
			gen.If(hasRefX).Then("e:=M{};"),
			"_ = map[string]interface{}{",
		),
		links,
		"}}",
		"",
	)
	code := t.Format(varMap)
	edit.AddCode(code)

	return
}
