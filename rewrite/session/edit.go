package session

import (
	"github.com/xhd2015/go-inspect/inspect"
	"github.com/xhd2015/go-inspect/rewrite/edit"
)

type Edit = edit.Edit

type EditCallback interface {
	OnEdit(f inspect.FileContext, content string) bool
	OnRewrites(f inspect.FileContext, content string) bool
	OnPkg(p inspect.Pkg, kind string, realName string, content string) bool
}
type EditCallbackFn struct {
	Edits    func(f inspect.FileContext, content string) bool
	Rewrites func(f inspect.FileContext, content string) bool
	Pkg      func(p inspect.Pkg, kind string, realName string, content string) bool
}

func (c *EditCallbackFn) OnEdit(f inspect.FileContext, content string) bool {
	if c.Edits == nil {
		return true
	}
	return c.Edits(f, content)
}
func (c *EditCallbackFn) OnRewrites(f inspect.FileContext, content string) bool {
	if c.Rewrites == nil {
		return true
	}
	return c.Rewrites(f, content)
}
func (c *EditCallbackFn) OnPkg(p inspect.Pkg, kind string, realName string, content string) bool {
	if c.Pkg == nil {
		return true
	}
	return c.Pkg(p, kind, realName, content)
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
