package inspect

import (
	"fmt"
	"go/token"
	"path/filepath"

	"github.com/xhd2015/go-inspect/code/edit"
	"github.com/xhd2015/go-inspect/code/gen"
	"github.com/xhd2015/go-inspect/inspect/util"
)

// Session session represents a rewrite pass
type Session interface {
	Global() Global
	// FileRewrite
	// rewrite of the source file
	FileRewrite(f FileContext) GoRewriteEdit

	// FileEdit edit in place
	FileEdit(f FileContext) GoRewriteEdit

	// kind: apart from the original files, newly selected files
	// call with the same pkgPath and kind returns the same result
	PackageEdit(p Pkg, kind string) GoNewEdit

	// Gen generates contents
	Gen(callback EditCallback)
}

type EditCallback interface {
	OnEdit(f FileContext, content string) bool
	OnRewrites(f FileContext, content string) bool
	OnPkg(p Pkg, kind string, realName string, content string) bool
}
type EditCallbackFn struct {
	Edits    func(f FileContext, content string) bool
	Rewrites func(f FileContext, content string) bool
	Pkg      func(p Pkg, kind string, realName string, content string) bool
}

func (c *EditCallbackFn) OnEdit(f FileContext, content string) bool {
	if c.Edits == nil {
		return true
	}
	return c.Edits(f, content)
}
func (c *EditCallbackFn) OnRewrites(f FileContext, content string) bool {
	if c.Rewrites == nil {
		return true
	}
	return c.Rewrites(f, content)
}
func (c *EditCallbackFn) OnPkg(p Pkg, kind string, realName string, content string) bool {
	if c.Pkg == nil {
		return true
	}
	return c.Pkg(p, kind, realName, content)
}

// Edit represents
type Edit interface {
	Insert(start token.Pos, content string)
	Delete(start token.Pos, end token.Pos)
	Replace(start token.Pos, end token.Pos, newContent string)

	String() string
}

type GoRewriteEdit interface {
	Edit

	// MustImport import `pkgPath` with `name`, returns the actual name used.
	MustImport(pkgPath string, name string, suggestAlias string, forbidden func(name string) bool) string

	// can always work
	AddAnaymouseInit(code string)

	Append(code string)
}

type GoNewEdit interface {
	SetPackageName(name string)

	MustImport(pkgPath string, name string, suggestAlias string, forbidden func(name string) bool) string

	// before package
	AddHeadCode(code string)

	// after import
	AddCode(code string)

	// CodeBuilder() *gen.TemplateBuilder

	// can always work
	AddAnaymouseInit(code string)

	// the code
	String() string
}

type session struct {
	g Global

	fileEditMap    util.SyncMap
	fileRewriteMap util.SyncMap
	pkgEditMap     util.SyncMap
}

var _ Session = ((*session)(nil))

func NewSession(g Global) Session {
	return &session{
		g: g,
	}
}

type fileEntry struct {
	f    FileContext
	edit GoRewriteEdit
}
type pkgEntry struct {
	pkg      Pkg
	kind     string
	realName string // no ".go" suffix
	edit     GoNewEdit
}

// Global implements Session
func (c *session) Global() Global {
	return c.g
}

// FileEdit implements Session
func (c *session) FileEdit(f FileContext) GoRewriteEdit {
	absPath := f.AbsPath()
	v := c.fileEditMap.LoadOrCompute(absPath, func() interface{} {
		return &fileEntry{f: f, edit: NewGoRewrite(f)}
	})
	return v.(*fileEntry).edit
}

// FileRewrite implements Session
func (c *session) FileRewrite(f FileContext) GoRewriteEdit {
	absPath := f.AbsPath()
	v := c.fileRewriteMap.LoadOrCompute(absPath, func() interface{} {
		return &fileEntry{f: f, edit: NewGoRewrite(f)}
	})
	return v.(*fileEntry).edit
}

// PackageEdit implements Session
func (c *session) PackageEdit(p Pkg, kind string) GoNewEdit {
	pkgPath := p.Path()
	key := fmt.Sprintf("%s:%s", pkgPath, kind)
	v := c.pkgEditMap.LoadOrCompute(key, func() interface{} {
		realName := util.NextName(func(s string) bool {
			foundGo := false
			p.RangeFiles(func(i int, f FileContext) bool {
				foundGo = foundGo || filepath.Base(f.AbsPath()) == kind+".go"
				return !foundGo
			})
			return !foundGo
		}, kind)
		edit := NewGoEdit()
		edit.SetPackageName(p.Name())
		return &pkgEntry{pkg: p, kind: kind, realName: realName, edit: edit}
	})
	return v.(*pkgEntry).edit
}
func (c *session) Gen(callback EditCallback) {
	loop := true

	c.fileEditMap.RangeComputed(func(key, value interface{}) bool {
		e := value.(*fileEntry)
		loop = loop && callback.OnEdit(e.f, e.edit.String())
		return loop
	})
	if !loop {
		return
	}

	c.fileRewriteMap.RangeComputed(func(key, value interface{}) bool {
		e := value.(*fileEntry)
		loop = loop && callback.OnRewrites(e.f, e.edit.String())
		return loop
	})
	if !loop {
		return
	}

	c.pkgEditMap.RangeComputed(func(key, value interface{}) bool {
		e := value.(*pkgEntry)
		loop = loop && callback.OnPkg(e.pkg, e.kind, e.realName, e.edit.String())
		return loop
	})
	if !loop {
		return
	}
}

type editImpl struct {
	buf  *edit.Buffer
	fset *token.FileSet
}

var _ Edit = ((*editImpl)(nil))

func NewEdit(fset *token.FileSet, content string) Edit {
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
	Edit
	ImportListContext

	anonymousPos *posInfo
}

var _ GoRewriteEdit = ((*goRewriteEdit)(nil))

func NewGoRewrite(f FileContext) GoRewriteEdit {
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
	ImportListContext
	imports []*imp

	pkgName string

	// pre code, like comment
	headCodes          []string
	codes              []string
	anonymousInitCodes []string
}

var _ GoNewEdit = ((*goNewEdit)(nil))

func NewGoEdit() GoNewEdit {
	c := &goNewEdit{}
	c.ImportListContext = NewImportList_X(nil, func(use, pkg string) {
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
