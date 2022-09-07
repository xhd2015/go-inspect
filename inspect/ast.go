package inspect

import (
	"fmt"
	"go/ast"
	"go/types"
	"io/ioutil"
	"strings"
	"sync"

	"github.com/hashicorp/hcl/hcl/strconv"

	"github.com/xhd2015/go-inspect/inspect/util"
)

// Node defines basic abstraction
// known implementations:
//    Pkg,File,Func
type Node interface {
	ASTNode() ast.Node
}

type FileContext interface {
	Global() Global
	Pkg() Pkg
	AbsPath() string // abs path of the file

	EditImports(edit Edit) ImportListContext

	AST() *ast.File
	ASTNode() ast.Node

	// IsGoFile ends with .go
	IsGoFile() bool

	// IsTestGoFile ends with _test.go
	IsTestGoFile() bool
}

type FuncContext interface {
	File() FileContext

	Name() string           // func name
	QuanlifiedName() string // <type>.<name> or *<type>.<name>

	AST() *ast.FuncDecl
	ASTNode() ast.Node

	Recv() FieldContext
	ParseRecvInfo() (ptr bool, typeName string)
	Type() FuncType

	RangeVars(fn func(i int, f FieldVar) bool)

	Body() interface{} // TODO
}

type FuncType interface {
	Pkg() Pkg
	AST() *ast.FuncType
	ASTNode() ast.Node
	Args() FieldListContext
	Results() FieldListContext
}

type FieldContext interface {
	Pkg() Pkg
	AST() *ast.Field
	TypeExpr() Expr    // type
	ASTNode() ast.Node // the ASTNode only refers to

	VarLen() int
	RangeVars(fn func(i int, f FieldVar) bool)
}

type FieldVar interface {
	Pkg() Pkg
	Name() string

	TypeExpr() Expr

	// Rename change name in edit Session
	// names must not be empty
	Rename(name string, edit Edit)
}

type FieldListContext interface {
	AST() *ast.FieldList
	ASTNode() ast.Node

	Len() int
	Index(i int) FieldContext

	// Range(fn func(i int, f FieldContext) bool)
	RangeVars(fn func(i int, f FieldVar) bool)
}

type TypeDecl interface {
	Pkg() Pkg // quick access

	File() FileContext
}

type TypeContext interface {
	GoType() types.Type
	GetNamedOrPtr() (ptr bool, n *types.Named)
	RefInvisible() bool
}

type Expr interface {
	Pkg() Pkg

	AST() ast.Expr
	ASTNode() ast.Node

	// RefersToQualified reports whether this expr referes
	// to a qualified ident after unwrapping alias(if any)
	RefersToQualified(pkgPath string, name string) bool

	// ResolveType try to resolve this expr to a declared type
	ResolveType() TypeContext
}

type fileContent struct {
	file string
	once sync.Once

	content string
}

func (c *fileContent) getContent() string {
	c.once.Do(func() {
		content, err := ioutil.ReadFile(c.file)
		if err != nil {
			panic(fmt.Errorf("reading file %s:%v", c.file, err))
		}
		c.content = string(content)
	})
	return c.content
}

type file struct {
	pkg Pkg
	ast *ast.File
}

var _ FileContext = ((*file)(nil))

func NewFile(pkg Pkg, ast *ast.File) FileContext {
	return &file{
		pkg: pkg,
		ast: ast,
	}
}

// Global implements FileContext
func (c *file) Global() Global {
	return c.pkg.Global()
}

// AbsPath implements FileContext
func (c *file) AbsPath() string {
	fset := c.pkg.Global().FileSet()
	// fileName is always absolute
	return fset.File(c.ast.Pos()).Name()
}

// AST implements FileContext
func (c *file) AST() *ast.File {
	return c.ast
}

// ASTNode implements FileContext
func (c *file) ASTNode() ast.Node {
	return c.ast
}

// EditImports implements FileContext
func (c *file) EditImports(edit Edit) ImportListContext {
	first := true
	insertPos := c.ast.Name.End()
	addImport := func(use, pkg string) {
		st := "import " + util.FormatImport(use, pkg) + ";"
		if first {
			st = ";" + st
			first = false
		}
		edit.Insert(insertPos, st)
	}
	return NewImportList_X(func(fn func(pkg string, name string, alias string)) {
		for _, imp := range c.ast.Imports {
			alias := ""
			if imp.Name != nil {
				alias = imp.Name.Name
			}
			pkgPath, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				panic(fmt.Errorf("parse package %s error:%v", pkgPath, err))
			}
			pkg := c.pkg.Global().GetPkg(pkgPath)
			if pkg == nil {
				panic(fmt.Errorf("package %s not found", pkgPath))
			}
			fn(pkgPath, pkg.Name(), alias)
		}
	}, addImport)
}

// Pkg implements FileContext
func (c *file) Pkg() Pkg {
	return c.pkg
}

// IsGoFile implements FileContext
func (c *file) IsGoFile() bool {
	return strings.HasSuffix(c.AbsPath(), ".go")
}

// IsTestGoFile implements FileContext
func (c *file) IsTestGoFile() bool {
	return strings.HasSuffix(c.AbsPath(), "_test.go")
}

type funcImpl struct {
	file FileContext
	ast  *ast.FuncDecl
}

func NewFunc(file FileContext, ast *ast.FuncDecl) FuncContext {
	return &funcImpl{
		file: file,
		ast:  ast,
	}
}

var _ FuncContext = ((*funcImpl)(nil))

// AST implements FuncContext
func (c *funcImpl) AST() *ast.FuncDecl {
	return c.ast
}

// ASTNode implements FuncContext
func (c *funcImpl) ASTNode() ast.Node {
	return c.ast
}
func (c *funcImpl) Name() string {
	return c.ast.Name.Name
}
func (c *funcImpl) QuanlifiedName() string {
	rcv := c.Recv()
	if rcv == nil {
		return c.Name()
	}
	ptr, n := rcv.TypeExpr().ResolveType().GetNamedOrPtr()
	if ptr {
		return "*" + n.Obj().Name() + "." + c.Name()
	}
	return n.Obj().Name() + "." + c.Name()
}

// Body implements FuncContext
func (c *funcImpl) Body() interface{} {
	return c.ast.Body
}

// File implements FuncContext
func (c *funcImpl) File() FileContext {
	return c.file
}

// Recv implements FuncContext
func (c *funcImpl) Recv() FieldContext {
	if c.ast.Recv != nil && len(c.ast.Recv.List) > 0 {
		return NewField(c.file.Pkg(), c.ast.Recv.List[0])
	}
	return nil
}
func (c *funcImpl) ParseRecvInfo() (ptr bool, typeName string) {
	if c.Recv() == nil {
		return
	}
	if c.Recv().VarLen() != 1 {
		panic(fmt.Errorf("method must have exactly 1 recv,found:%d", c.Recv().VarLen()))
	}
	c.Recv().RangeVars(func(i int, f FieldVar) bool {
		var name *types.Named
		ptr, name = f.TypeExpr().ResolveType().GetNamedOrPtr()
		if name != nil {
			typeName = name.Obj().Name()
		}
		return true
	})
	return
}

// Type implements FuncContext
func (c *funcImpl) Type() FuncType {
	return NewFuncType(c.file.Pkg(), c.ast.Type)
}
func (c *funcImpl) RangeVars(fn func(i int, f FieldVar) bool) {
	idx := -1
	done := false
	tf := func(_ int, f FieldVar) bool {
		idx++
		done = fn(idx, f)
		return done
	}
	recv := c.Recv()
	if recv != nil {
		recv.RangeVars(tf)
		if done {
			return
		}
	}
	c.Type().Args().RangeVars(tf)
	if done {
		return
	}
	c.Type().Results().RangeVars(tf)
}

type funcType struct {
	pkg Pkg
	ast *ast.FuncType
}

func NewFuncType(pkg Pkg, node *ast.FuncType) FuncType {
	return &funcType{
		pkg: pkg,
		ast: node,
	}
}

var _ FuncType = ((*funcType)(nil))

// Pkg implements FuncType
func (c *funcType) Pkg() Pkg {
	return c.pkg
}

// AST implements FuncType
func (c *funcType) AST() *ast.FuncType {
	return c.ast
}

// ASTNode implements FuncType
func (c *funcType) ASTNode() ast.Node {
	return c.ast
}

// Args implements FuncType
func (c *funcType) Args() FieldListContext {
	return NewFieldList(c.pkg, c.ast.Params)
}

// Results implements FuncType
func (c *funcType) Results() FieldListContext {
	return NewFieldList(c.pkg, c.ast.Results)
}

type field struct {
	pkg Pkg
	ast *ast.Field
}

func NewField(pkg Pkg, ast *ast.Field) FieldContext {
	return &field{
		pkg: pkg,
		ast: ast,
	}
}

var _ FieldContext = ((*field)(nil))

// Pkg implements FieldContext
func (c *field) Pkg() Pkg {
	return c.pkg
}

func (c *field) AST() *ast.Field {
	return c.ast
}

// ASTNode implements FieldContext
func (c *field) ASTNode() ast.Node {
	return c.ast
}

// Name implements FieldContext
func (c *field) Names() []string {
	if len(c.ast.Names) > 0 {
		names := make([]string, 0, len(c.ast.Names))
		for _, idt := range c.ast.Names {
			names = append(names, idt.Name)
		}
		return names
	}
	return nil
}

// func (c *Field) Rename(fset *token.FileSet, buf *edit.Buffer) {
// 	// always rewrite if: origName does not exist, or has been renamed
// 	if c.OrigName == "" || c.OrigName == "_" || c.OrigName != c.Name {
// 		if c.NameNode == nil {
// 			buf.Insert(OffsetOf(fset, c.TypeExpr.Pos()), c.Name+" ")
// 		} else {
// 			buf.Replace(OffsetOf(fset, c.NameNode.Pos()), OffsetOf(fset, c.NameNode.End()), c.Name)
// 		}
// 	}
// }

// TypeExpr implements FieldContext
func (f *field) TypeExpr() Expr {
	return NewExpr(f.pkg, f.ast.Type)
}

func (c *field) VarLen() int {
	n := len(c.ast.Names)
	if n == 0 {
		return 1
	}
	return n
}

// RangeVars implements FieldContext
func (c *field) RangeVars(fn func(i int, f FieldVar) bool) {
	if len(c.ast.Names) > 0 {
		for i := range c.ast.Names {
			if !fn(i, NewFieldVar(i, c)) {
				break
			}
		}
	} else {
		fn(0, NewFieldVar(0, c))
	}
}

type fieldVar struct {
	idx int
	f   FieldContext
}

func NewFieldVar(idx int, f FieldContext) FieldVar {
	if idx >= f.VarLen() {
		panic(fmt.Errorf("out of range: %d >= %d", idx, f.VarLen()))
	}
	return fieldVar{
		idx: idx,
		f:   f,
	}
}

var _ FieldVar = fieldVar{}

// Pkg implements FieldVar
func (c fieldVar) Pkg() Pkg {
	return c.f.Pkg()
}

// Name implements FieldVar
func (c fieldVar) Name() string {
	names := c.f.AST().Names
	if len(names) == 0 {
		return ""
	}
	return names[c.idx].Name
}

// TypeExpr implements FieldVar
func (c fieldVar) TypeExpr() Expr {
	return c.f.TypeExpr()
}

// Rename implements FieldVar
func (c fieldVar) Rename(name string, edit Edit) {
	if name == "" {
		panic(fmt.Errorf("empty name at %d", c.idx))
	}
	ast := c.f.AST()

	if len(ast.Names) == 0 {
		edit.Insert(ast.Type.Pos(), name+"  ")
	} else {
		edit.Replace(ast.Names[c.idx].Pos(), ast.Names[c.idx].End(), name)
	}
}

type fieldList struct {
	pkg Pkg
	ast *ast.FieldList
}

var _ FieldListContext = ((*fieldList)(nil))

func NewFieldList(pkg Pkg, ast *ast.FieldList) FieldListContext {
	return &fieldList{
		pkg: pkg,
		ast: ast,
	}
}

// ASTNode implements FieldListContext
func (c *fieldList) AST() *ast.FieldList {
	return c.ast
}

// ASTNode implements FieldListContext
func (c *fieldList) ASTNode() ast.Node {
	return c.ast
}

// Index implements FieldListContext
func (c *fieldList) Index(i int) FieldContext {
	return NewField(c.pkg, c.ast.List[i])
}

// Len implements FieldListContext
func (c *fieldList) Len() int {
	return len(c.ast.List)
}

// RangeVars implements FieldListContext
func (c *fieldList) RangeVars(fn func(i int, f FieldVar) bool) {
	idx := -1
	done := false
	for i, _ := range c.ast.List {
		c.Index(i).RangeVars(func(_ int, f FieldVar) bool {
			idx++
			done = fn(idx, f)
			return done
		})
		if done {
			return
		}
	}
}

type typeContext struct {
	goType types.Type
}

func NewType(goType types.Type) TypeContext {
	return &typeContext{
		goType: goType,
	}
}

var _ TypeContext = ((*typeContext)(nil))

// GoType implements TypeContext
func (c *typeContext) GoType() types.Type {
	return c.goType
}

func (c *typeContext) GetNamedOrPtr() (ptr bool, n *types.Named) {
	t := c.goType
	if x, ok := t.(*types.Pointer); ok {
		ptr = true
		t = x.Elem()
	}
	if named, ok := t.(*types.Named); ok {
		n = named
	}
	return
}
func (c *typeContext) RefInvisible() bool {
	return RefInvisible(c.goType)
}

// func (c *typeContext) TypeName() (ptr bool, name string) {
// 	ptr, named := c.GetNamedOrPtr(c.goType)
// 	if named != nil {
// 		name = named.Obj().Name()
// 	}
// 	return
// }

type expr struct {
	pkg Pkg
	ast ast.Expr
}

func NewExpr(pkg Pkg, ast ast.Expr) Expr {
	return &expr{
		pkg: pkg,
		ast: ast,
	}
}

var _ Expr = ((*expr)(nil))

// AST implements Expr
func (c *expr) AST() ast.Expr {
	return c.ast
}

// ASTNode implements Expr
func (c *expr) ASTNode() ast.Node {
	return c.ast
}

// Pkg implements Expr
func (c *expr) Pkg() Pkg {
	return c.Pkg()
}

// RefersToQualified implements Expr
func (c *expr) RefersToQualified(pkgPath string, name string) bool {
	// TODO: also check alias
	return util.TokenHasQualifiedName(c.pkg.GoPkg(), c.ast, pkgPath, name)
}

// ResolveType implements Expr
func (c *expr) ResolveType() TypeContext {
	return NewType(c.pkg.GoPkg().TypesInfo.TypeOf(c.ast))
}
