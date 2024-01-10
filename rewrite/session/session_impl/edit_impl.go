package session_impl

import (
	"go/token"

	"github.com/xhd2015/go-inspect/code/edit"
	"github.com/xhd2015/go-inspect/code/gen"
	"github.com/xhd2015/go-inspect/inspect"
	"github.com/xhd2015/go-inspect/inspect/util"
	sessionpkg "github.com/xhd2015/go-inspect/rewrite/session"
)

type editImpl struct {
	buf  *edit.Buffer
	fset *token.FileSet
}

var _ sessionpkg.Edit = ((*editImpl)(nil))

func NewEdit(fset *token.FileSet, content string) sessionpkg.Edit {
	return &editImpl{
		fset: fset,
		buf:  edit.NewBuffer([]byte(content)),
	}
}

// Delete implements Edit
func (c *editImpl) Delete(start token.Pos, end token.Pos) {
	c.buf.Delete(util.OffsetOf(c.fset, start), util.OffsetOf(c.fset, end))
}

// Insert implements Edit
func (c *editImpl) Insert(start token.Pos, content string) {
	c.buf.Insert(util.OffsetOf(c.fset, start), content)
}

// Replace implements Edit
func (c *editImpl) Replace(start token.Pos, end token.Pos, content string) {
	c.buf.Replace(util.OffsetOf(c.fset, start), util.OffsetOf(c.fset, end), content)
}

func (c *editImpl) String() string {
	return c.buf.String()
}

type posInfo struct {
	pos    token.Pos
	offset int
}

func NewPos(pos token.Pos, offset int) *posInfo {
	return &posInfo{
		pos:    pos,
		offset: offset,
	}
}

func (c *posInfo) Pos() token.Pos {
	return c.pos + token.Pos(c.offset)
}
func (c *posInfo) Advance(off int) {
	c.offset += off
}

type goRewriteEdit struct {
	sessionpkg.Edit
	inspect.ImportListContext

	anonymousPos *posInfo
}

var _ sessionpkg.GoRewriteEdit = ((*goRewriteEdit)(nil))

func NewGoRewrite(f inspect.FileContext) sessionpkg.GoRewriteEdit {
	g := f.Pkg().Global()
	edit := NewEdit(g.FileSet(), g.Code(f))
	return &goRewriteEdit{
		Edit:              edit,
		ImportListContext: f.EditImports(edit),
		anonymousPos:      NewPos(f.AST().End(), 0),
	}
}

// AddAnaymouseInit implements GoRewriteEdit
func (c *goRewriteEdit) AddAnaymouseInit(code string) {
	c.Edit.Insert(c.anonymousPos.Pos(), code)
	c.anonymousPos.Advance(len(code))
}
func (c *goRewriteEdit) Append(code string) {
	c.Edit.Insert(c.anonymousPos.pos, code)
	// c.anonymousPos.Advance(len(code))
}

type imp struct {
	alias   string
	pkgPath string
}

func (c *imp) Format() string {
	return util.FormatImport(c.alias, c.pkgPath)
}

type goNewEdit struct {
	inspect.ImportListContext
	imports []*imp

	pkgName string

	// pre code, like comment
	headCodes          []string
	codes              []string
	anonymousInitCodes []string
}

var _ sessionpkg.GoNewEdit = ((*goNewEdit)(nil))

func NewGoEdit() sessionpkg.GoNewEdit {
	c := &goNewEdit{}
	c.ImportListContext = inspect.NewImportList_X(nil, func(use, pkg string) {
		c.imports = append(c.imports, &imp{
			alias:   use,
			pkgPath: pkg,
		})
	})
	return c
}

// SetPackageName implements GoNewEdit
func (c *goNewEdit) SetPackageName(name string) {
	c.pkgName = name
}

// AddAnaymouseInit implements GoNewEdit
func (c *goNewEdit) AddAnaymouseInit(code string) {
	c.anonymousInitCodes = append(c.anonymousInitCodes, code)
}

// before package
func (c *goNewEdit) AddHeadCode(code string) {
	c.headCodes = append(c.headCodes, code)
}

// after import
func (c *goNewEdit) AddCode(code string) {
	c.codes = append(c.codes, code)
}

func (c *goNewEdit) String() string {
	t := gen.NewTemplateBuilder()
	if len(c.headCodes) > 0 {
		t.Block(
			c.headCodes,
			"",
		)
	}
	t.Block(
		"package " + c.pkgName,
	)
	if len(c.imports) > 0 {
		t.Block(
			"", // new line
			"import (",
		)
		for _, imp := range c.imports {
			t.Block("    " + imp.Format())
		}
		t.Block(")")
	}

	if len(c.codes) > 0 {
		t.Block(
			"",
			c.codes,
		)
	}

	if len(c.anonymousInitCodes) > 0 {
		// var _ = func() bool { ;return true;}
		t.Block(
			"",
			c.anonymousInitCodes,
		)
	}
	return t.Format(nil)
}
