package inspect

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/xhd2015/go-inspect/code/edit"
	"github.com/xhd2015/go-inspect/code/gen"
	inspect "github.com/xhd2015/go-inspect/inspect"
	"github.com/xhd2015/go-inspect/inspect/util"
)

// this file provides source file rewrite and mock stub generation.
// the generated stub are copied into a sub-directory with the same
// package hierarchy.
// For example, package 'a/b/c' is put into 'test/mock_gen/a/c'.
// The same package name is still present. This time all file
// are merged into one.
// For unexported types and their dependency unexported types,
// an exported name will be made available to external packages.

const MOCK_PKG = "github.com/xhd2015/go-inspect/inspect/mock"
const SKIP_MOCK_PKG = "_SKIP_MOCK"
const SKIP_MOCK_FILE = "_SKIP_MOCK_THIS_FILE"

type RewriteOptions struct {
	// Filter tests whether the specific function should be rewritten
	Filter func(pkgPath string, fileName string, ownerName string, ownerIsPtr bool, funcName string) bool
}

type RewriteResult struct {
	Funcs map[string]map[string]string
	Error error // error if any
}

type ContentError struct {
	PkgPath string // a repeat of the map result
	Files   map[string]*FileContentError

	// exported types
	// function prototypes are based on
	MockContent      string
	MockContentError error // error if any
	// MockInfoCode type for mockable functions
	MockInfoCode  string
	MockInfoError error
}
type FileContentError struct {
	OrigFile string // a repeat of the key
	Content  string
	Error    error
}

type rewriteFuncDetail struct {
	File          string
	RewriteConfig *RewriteConfig

	// original no re-packaged
	// Args    string // including receiver
	// Results string

	ArgsRewritter    func(r inspect.AstNodeRewritter, hook func(node ast.Node, c []byte) []byte) string // re-packaged
	ResultsRewritter func(r inspect.AstNodeRewritter, hook func(node ast.Node, c []byte) []byte) string // re-packaged
}

type RewriteFileDetail struct {
	File     *ast.File
	FilePath string
	Funcs    []*rewriteFuncDetail

	// name -> EXPORTED name
	AllExportNames map[string]string
	// pkgs imported by function types
	// this info is possibly used by external generator(plus the package itself will be imported by external generator.)
	// pkg path -> name/alias
	ImportPkgByTypes map[string]*NameAlias

	// the content getter
	GetContentByPos func(start, end token.Pos) []byte
}
type NameAlias struct {
	Name  string
	Alias string
	Use   string // the effective appearance
}

// name -> EXPORTED name
func exportTypes(starterTypes []types.Type, pkgPath string, astFile *ast.File) (allExportNames map[string]string, importPkgByTypes map[string]*NameAlias) {
	needExportNames := make(map[string]string)
	allExportNames = make(map[string]string)
	importPkgByTypes = make(map[string]*NameAlias)
	TraverseTypes(starterTypes, func(t types.Type) bool {
		n, ok := t.(*types.Named)
		if !ok {
			return true //continue
		}
		pkg := n.Obj().Pkg()
		if pkg != nil { // int,error's pkg is nil
			impPkgPath := pkg.Path()
			if impPkgPath == pkgPath {
				name := n.Obj().Name()
				expName := name
				if !util.IsExportedName(name) {
					expName = EXPORT_PREFIX + name
					needExportNames[name] = expName
				}
				allExportNames[name] = expName
			} else {
				// find the correct name used
				if _, ok := importPkgByTypes[impPkgPath]; !ok {
					name := pkg.Name()
					alias, _ := getFileImport(astFile, impPkgPath)
					use := alias
					if alias == "" {
						use = name
					}
					importPkgByTypes[impPkgPath] = &NameAlias{
						Name:  name,
						Alias: alias,
						Use:   use,
					}
				}
			}
		}
		return false
	})
	return
}

func genRegCode(funcDetails []*rewriteFuncDetail, pkgPath string, impEditor inspect.ImportListContext) string {
	getMockImpName := func() string {
		return impEditor.MustImport(MOCK_PKG, "mock", "_mock", nil /* no forbidden */)
	}
	getReflectImpName := func() string {
		return impEditor.MustImport("reflect", "reflect", "", nil /* no forbidden */)
	}

	// gen register mock call
	regT := gen.NewTemplateBuilder()
	regT.Block(
		"var _ = func() bool {",
		fmt.Sprintf("    pkgPath := %q", pkgPath),
	)

	getFields := func(args FieldList) string {
		list := make([]string, 0, len(args))
		for _, arg := range args {
			list = append(list,
				fmt.Sprintf("%s.NewTypeInfo(%q, %s.TypeOf((*%s)(nil)).Elem())",
					getMockImpName(),
					arg.Name,
					getReflectImpName(),
					deEllipsisTypeStr(arg),
				))
		}
		return strings.Join(list, ",")
	}

	for _, fd := range funcDetails {
		fdT := gen.NewTemplateBuilder()
		fdT.Block(
			"__MOCKP__.RegisterMockStub(pkgPath, __OWNER_TYPE_NAMEQ__,__OWNER_TYPE__,__FUNC_NAMEQ__, []__MOCKP__.TypeInfo{__ARGS__},[]__MOCKP__.TypeInfo{__RESULTS__},__ARGCTX__,__RESERR__)",
		)

		ownerType := "nil"
		if fd.RewriteConfig.Recv != nil {
			ownerType = fmt.Sprintf("%s.TypeOf((*%s)(nil)).Elem()", getReflectImpName(), deEllipsisTypeStr(fd.RewriteConfig.Recv))
		}

		fdRegCode := fdT.Format(gen.VarMap{
			"__MOCKP__":            fd.RewriteConfig.SupportPkgRef,
			"__OWNER_TYPE_NAMEQ__": strconv.Quote(fd.RewriteConfig.Owner),
			"__OWNER_TYPE__":       ownerType,
			"__FUNC_NAMEQ__":       strconv.Quote(fd.RewriteConfig.FuncName),
			"__ARGS__":             getFields(fd.RewriteConfig.Args),
			"__RESULTS__":          getFields(fd.RewriteConfig.Results),
			"__ARGCTX__":           strconv.FormatBool(fd.RewriteConfig.FirstArgIsCtx),
			"__RESERR__":           strconv.FormatBool(fd.RewriteConfig.LastResIsError),
		})

		regT.Block(gen.Indent("    ", fdRegCode))
	}
	regT.Block(
		"    return true",
		"} ()",
	)
	return regT.Format(nil)
}

func joinRecvArgs(recvCode string, args string, recvOrigName string, needEmptyName bool, argLen int) string {
	recvCode = strings.TrimPrefix(recvCode, "(")
	recvCode = strings.TrimSuffix(recvCode, ")")
	comma := ""
	if argLen > 0 {
		comma = ","
	}
	if recvOrigName == "" && needEmptyName { // no name,given _
		recvCode = "_ " + recvCode
	}

	return "(" + recvCode + comma + strings.TrimPrefix(args, "(")
}

// name -> EXPORTED name
func exportUnexported(f *ast.File, fset *token.FileSet, needExportNames map[string]string, buf *edit.Buffer) {
	for _, decl := range f.Decls {
		if gdecl, ok := decl.(*ast.GenDecl); ok && gdecl.Tok == token.TYPE {
			for _, spec := range gdecl.Specs {
				if tspec, ok := spec.(*ast.TypeSpec); ok {
					exportedName := needExportNames[tspec.Name.Name]
					if exportedName != "" {
						buf.Insert(util.OffsetOf(fset, gdecl.End()), fmt.Sprintf(";type %s = %s", exportedName, tspec.Name.Name))
					}
				}
			}
		}
	}
}

type NamedType struct {
	t *types.Named
}

func NewNamedType(t *types.Named) *NamedType {
	return &NamedType{t}
}

func initRewriteConfig(pkg *packages.Package, decl *ast.FuncDecl, skipNonCtx bool) *RewriteConfig {
	pkgPath := pkg.PkgPath
	rc := &RewriteConfig{
		Names:         make(map[string]bool),
		SupportPkgRef: "_mock",
		VarPrefix:     "_mock",
		Pkg:           pkgPath,
		FuncName:      decl.Name.Name,
	}
	rc.Exported = util.IsExportedName(rc.FuncName)

	firstIsCtx := false
	params := decl.Type.Params
	if params != nil && len(params.List) > 0 {
		firstParam := params.List[0]
		var firstName string
		if len(firstParam.Names) > 0 {
			firstName = firstParam.Names[0].Name
		}

		if util.TokenHasQualifiedName(pkg, firstParam.Type, "context", "Context") {
			firstIsCtx = true
			if firstName == "" || firstName == "_" {
				// not give
				rc.CtxName = "ctx"
			} else {
				rc.CtxName = firstName
			}
		}
	}
	if !firstIsCtx && skipNonCtx {
		return nil
	}

	// at least one res has names
	rc.ResultsNameGen = !hasName(decl.Type.Results)

	lastIsErr := false
	if decl.Type.Results != nil && len(decl.Type.Results.List) > 0 {
		lastRes := decl.Type.Results.List[len(decl.Type.Results.List)-1]
		var lastName string
		if len(lastRes.Names) > 0 {
			lastName = lastRes.Names[len(lastRes.Names)-1].Name
		}
		retType := pkg.TypesInfo.TypeOf(lastRes.Type)

		if util.HasQualifiedName(retType, "", "error") {
			lastIsErr = true
			if lastName == "" || lastName == "_" {
				rc.ErrName = "err"
			} else {
				rc.ErrName = lastName
			}
		}
	}

	typeInfo := make(map[types.Type]*Type)

	rc.FirstArgIsCtx = firstIsCtx
	rc.LastResIsError = lastIsErr
	if decl.Type.Params != nil {
		rc.FullArgs = parseTypes(decl.Type.Params.List, pkg, "unused_", typeInfo)
	}

	// return may be empty
	if decl.Type.Results != nil {
		rc.FullResults = parseTypes(decl.Type.Results.List, pkg, "Resp_", typeInfo)
	}

	rc.Args = rc.FullArgs
	rc.Results = rc.FullResults
	if firstIsCtx {
		rc.FullArgs[0].Name = rc.CtxName
		rc.Args = rc.FullArgs[1:]
	}
	if lastIsErr {
		rc.FullResults[len(rc.Results)-1].Name = rc.ErrName
		rc.Results = rc.Results[:len(rc.Results)-1]
	}

	// recv
	rc.Recv = parseRecv(decl, pkg, typeInfo)
	if rc.Recv != nil {
		rc.OwnerPtr, rc.Owner = rc.Recv.Type.Ptr, rc.Recv.Type.Name
		rc.AllFields = append(rc.AllFields, rc.Recv)
	}

	rc.AllFields = append(rc.AllFields, rc.FullArgs...)
	rc.AllFields = append(rc.AllFields, rc.FullResults...)

	rc.NewFuncName = "_mock" + rc.GetFullName()

	// find existing names
	// if any fieldName conflicts with an existing name,
	// we should rename it.
	// for example :
	//     func (dao *impl) Find(filter dao.Filter)
	usedSelector := make(map[string]bool)
	ast.Inspect(decl.Type, func(n ast.Node) bool {
		if x, ok := n.(*ast.SelectorExpr); ok {
			if id, ok := x.X.(*ast.Ident); ok {
				usedSelector[id.Name] = true
			}
		}
		return true
	})

	// unique names
	allVisible := true
	for _, field := range rc.AllFields {
		allVisible = allVisible && field.Type.Visible
		// will not modify original names as they are already validated
		if field.OrigName != "" && field.OrigName != "_" && !usedSelector[field.OrigName] {
			rc.Names[field.Name] = true
			field.ExportedName = ToExported(field.Name)
			continue
		}

		field.Name = util.NextName(func(k string) bool {
			if rc.Names[k] || usedSelector[k] {
				return false
			}
			rc.Names[k] = true
			return true
		}, field.Name)
		field.ExportedName = ToExported(field.Name)
	}

	// CtxName ,ErrName,
	if rc.FirstArgIsCtx {
		rc.CtxName = rc.FullArgs[0].Name
	}
	if rc.LastResIsError {
		rc.ErrName = rc.FullResults[len(rc.FullResults)-1].Name
	}

	return rc
}

func (c *RewriteConfig) GetFullName() string {
	if c.Recv != nil {
		return c.Owner + "_" + c.FuncName
	}
	return c.FuncName
}

func parseRecv(decl *ast.FuncDecl, pkg *packages.Package, typeInfo map[types.Type]*Type) *Field {
	if decl.Recv != nil && len(decl.Recv.List) > 0 {
		if len(decl.Recv.List) != 1 {
			panic(fmt.Errorf("multiple receiver found:%s", decl.Name.Name))
		}
		return parseTypes(decl.Recv.List, pkg, "unused_Recv", typeInfo)[0]
	}
	return nil
}

const EXPORT_PREFIX = "MExport_"

func parseTypes(list []*ast.Field, pkg *packages.Package, genPrefix string, typeInfo map[types.Type]*Type) []*Field {
	fields := make([]*Field, 0, len(list))

	forEachName(list, func(i int, nameNode *ast.Ident, name string, t ast.Expr) {
		fName := name
		origName := fName
		if fName == "_" || fName == "" {
			fName = fmt.Sprintf("%s%d", genPrefix, i)
		}
		rtype := resolveType(pkg, t)
		tIsPtr, tName := typeName(pkg, rtype)

		_, ellipsis := t.(*ast.Ellipsis)

		typeInfoCache := typeInfo[rtype]
		if typeInfoCache == nil {
			exported := util.IsExportedName(tName)
			exportedName := tName
			if !exported {
				exportedName = EXPORT_PREFIX + tName
			}
			foundInvisible := RefInvisible(rtype)
			typeInfoCache = &Type{
				Ptr:          tIsPtr,
				Name:         tName,
				Exported:     exported,
				ExportedName: exportedName, // TODO: fix for error, MExport_error is not correct
				ResolvedType: rtype,
				Visible:      !foundInvisible,
			}
			typeInfo[rtype] = typeInfoCache
		}

		fields = append(fields, &Field{
			Name:     fName,
			OrigName: origName,
			NameNode: nameNode,
			Type:     typeInfoCache,
			TypeExpr: t,
			Ellipsis: ellipsis,
		})
	})
	return fields
}

func hasName(fields *ast.FieldList) bool {
	if fields != nil && len(fields.List) > 0 {
		for _, res := range fields.List {
			for _, x := range res.Names {
				if x.Name != "" {
					return true
				}
			}
		}
	}
	return false
}

func formatAssign(dst []string, colon bool, src string) string {
	if len(dst) == 0 {
		return src
	}
	eq := "="
	if colon {
		eq = ":="
	}
	return fmt.Sprintf("%s %s %s", strings.Join(dst, " "), eq, src)
}

// names must be pre-assigned
// the rewritter will change all _ names to generated names, like '_unusedReq_${i}', '_unusedResp_${i}`
// these names will not appear to mock json.
type RewriteConfig struct {
	Names         map[string]bool // declared names(unique)
	SupportPkgRef string          // _mock
	VarPrefix     string          // _mock
	Pkg           string
	Owner         string // the owner type name (always inside Pkg)
	OwnerPtr      bool   // is owner type a pointer type?
	Exported      bool   // is name exported?
	FuncName      string
	NewFuncName   string
	// HasCtx always be true
	CtxName        string // if "", has no ctx. if "_", should adjust outside this config
	ErrName        string // if "", has no error.
	ResultsNameGen bool   // Results names generated ?
	FirstArgIsCtx  bool
	LastResIsError bool
	Recv           *Field
	FullArgs       FieldList // args including first ctx
	FullResults    FieldList // results includeing last error
	AllFields      FieldList // all fields, including recv(if any),args,results
	Args           FieldList // args excluding first ctx
	Results        FieldList // results excluding last error
}

type Signature struct {
	Args           string
	ArgRecvMayIntf string // version where Recv changed to interface{}
	Results        string
}

func (c *Signature) String() string {
	return fmt.Sprintf("func%s%s", c.Args, c.Results)
}

// not needed
// func (c *Signature) StringIntf() string {
// 	return fmt.Sprintf("func%s%s", c.ArgRecvMayIntf, c.Results)
// }

type FieldList []*Field

func (c FieldList) ForEachField(ignoreFirst, ignoreLast bool, fn func(f *Field)) {
	for i, f := range c {
		if ignoreFirst && i == 0 {
			continue
		}
		if ignoreLast && i == len(c)-1 {
			continue
		}
		fn(f)
	}
}

// are all fields visible to outside? meaning it does not reference unexported names.
// this function can be added to generated mock only when `AllTypesVisible` is true.
// even when its name is unexported.
func (c FieldList) AllTypesVisible() bool {
	for _, f := range c {
		if !f.Type.Visible {
			return false
		}
	}
	return true
}

func (c FieldList) FillFieldTypeExpr(fset *token.FileSet, content []byte) {
	for _, f := range c {
		f.TypeExprString = string(getContent(fset, content, f.TypeExpr.Pos(), f.TypeExpr.End()))
	}
}

func forEachName(list []*ast.Field, fn func(i int, nameNode *ast.Ident, name string, t ast.Expr)) {
	i := 0
	for _, e := range list {
		if len(e.Names) > 0 {
			for _, n := range e.Names {
				fn(i, n, n.Name, e.Type)
				i++
			}
		} else {
			fn(i, nil, "", e.Type)
			i++
		}
	}
}

type Field struct {
	Name         string // original name or generated name
	ExportedName string // exported version of Name
	NameNode     *ast.Ident
	// Type     *Type
	OrigName string // original name

	Type           *Type
	TypeExpr       ast.Expr // the type expr,indicate the position. maybe *ast.Indent, *ast.SelectorExpr or recursively
	TypeExprString string
	Ellipsis       bool // when true, TypeExpr is *ast.Ellipsis, and ResolvedType is slice type(unnamed)
}

type Type struct {
	Ptr          bool
	Name         string
	Exported     bool   // true if original Name is exported
	ExportedName string // if !Exported, the generated name

	ResolvedType types.Type
	Visible      bool // visible to outside? either is an exported name, or name from another non-internal package, or contains names of such. TODO: add internal detection.
}

func (c *RewriteConfig) Init() {
	if c.SupportPkgRef == "" {
		c.SupportPkgRef = "_mock"
	}
	if c.VarPrefix == "" {
		c.VarPrefix = "_mock"
	}
}

func (c *RewriteConfig) Validate() {
	// if c.CtxName == "" {
	// 	panic(fmt.Errorf("no ctx var, pkg:%v, owner:%v, func:%v", c.Pkg, c.Owner, c.FuncName))
	// }
	if c.CtxName == "_" {
		panic(fmt.Errorf("var ctx must not be _"))
	}
	if c.NewFuncName == "" {
		panic(fmt.Errorf("NewFuncName must not be empty"))
	}
	for i, arg := range c.Args {
		if arg.Name == "" {
			panic(fmt.Errorf("arg name %d is empty", i))
		}
	}
	for i, res := range c.Results {
		if res.Name == "" {
			panic(fmt.Errorf("results %d is empty", i))
		}
	}
}

func deEllipsisTypeStr(f *Field) string {
	if !f.Ellipsis {
		return f.TypeExprString
	}
	return "[]" + strings.TrimPrefix(f.TypeExprString, "...") // hack: replace ... with []
}

func (c *RewriteConfig) Gen(pretty bool) string {
	c.Validate()

	resNames := make([]string, 0, len(c.Results))
	for _, res := range c.Results {
		resNames = append(resNames, res.Name)
	}

	makeStructDefs := func(c FieldList) string {
		reqDefList := make([]string, 0, len(c))
		for _, f := range c {
			reqDefList = append(reqDefList, fmt.Sprintf("%v %s `json:%v`", f.ExportedName, deEllipsisTypeStr(f), strconv.Quote(f.Name)))
		}

		structDefs := strings.Join(reqDefList, ";")
		if len(structDefs) > 0 {
			structDefs = structDefs + ";"
		}
		return structDefs
	}

	getFieldAssigns := func(fields []*Field) string {
		assignList := make([]string, 0, len(fields))
		for _, arg := range fields {
			assignList = append(assignList, fmt.Sprintf("%v: %v", arg.ExportedName, arg.Name))
		}
		reqDefs := strings.Join(assignList, ",")
		return reqDefs
	}
	getResFields := func(fields []*Field, base string) string {
		assignList := make([]string, 0, len(fields))
		for _, arg := range fields {
			assignList = append(assignList, fmt.Sprintf("%s.%s", base, arg.ExportedName))
		}
		reqDefs := strings.Join(assignList, ",")
		return reqDefs
	}
	recvVar := "nil"
	if c.Recv != nil {
		recvVar = c.Recv.Name
	}

	varMap := gen.VarMap{
		"__V__":               c.VarPrefix,
		"__P__":               c.SupportPkgRef,
		"__RECV_VAR__":        recvVar,
		"__PKG_NAME_Q__":      strconv.Quote(c.Pkg),
		"__OWNER_NAME_Q__":    strconv.Quote(c.Owner),
		"__OWNER_IS_PTR__":    strconv.FormatBool(c.OwnerPtr),
		"__FUNC_NAME_Q__":     strconv.Quote(c.FuncName),
		"__NEW_FUNC__":        c.NewFuncName,
		"__ERR_NAME__":        c.ErrName,
		"__REQ_DEFS__":        makeStructDefs(c.Args),
		"__RESP_DEFS__":       makeStructDefs(c.Results),
		"__RES_NAMES__":       strings.Join(resNames, ","),
		"__REQ_DEF_ASSIGN__":  getFieldAssigns(c.Args),
		"__MOCK_RES_FIELDS__": getResFields(c.Results, fmt.Sprintf("%sresp", c.VarPrefix)),
		"__HAS_RECV__":        strconv.FormatBool(c.Recv != nil),
		"__FIRST_IS_CTX__":    strconv.FormatBool(c.FirstArgIsCtx),
		"__LAST_IS_ERR__":     strconv.FormatBool(c.LastResIsError),
	}
	t := gen.NewTemplateBuilder()
	t.Block(
		"var __V__req = struct{__REQ_DEFS__}{__REQ_DEF_ASSIGN__}",
		"var __V__resp struct{__RESP_DEFS__}",
		// func TrapFunc(ctx context.Context, stubInfo *StubInfo, inst interface{}, req interface{}, resp interface{}, oldFunc interface{}, hasRecv bool, firstIsCtx bool, lastIsErr bool) error
		gen.Group(
			gen.If(c.ErrName != "").Then("__ERR_NAME__ = "),
			gen.Group(
				"__P__.TrapFunc(",
				gen.If(c.CtxName != "").Then(c.CtxName).Else("nil"), ",",
				"&__P__.StubInfo{PkgName:__PKG_NAME_Q__,Owner:__OWNER_NAME_Q__,OwnerPtr:__OWNER_IS_PTR__,Name:__FUNC_NAME_Q__}, __RECV_VAR__, &__V__req, &__V__resp,__NEW_FUNC__,__HAS_RECV__,__FIRST_IS_CTX__,__LAST_IS_ERR__)",
			),
		),
		gen.If(len(c.Results) > 0).Then(
			"__RES_NAMES__ = __MOCK_RES_FIELDS__",
		),
		gen.If(len(c.Results) > 0 || c.ErrName != "").Then(
			"return",
		),
	)

	t.Pretty(pretty)

	// generate rule is that,  when no pretty, shoud
	// add ';' after each statement, unless that statements ends with '{'
	return t.Format(varMap)
}
