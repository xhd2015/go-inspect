package session

import (
	"github.com/xhd2015/go-inspect/inspect"
)

// Session session represents a rewrite pass
type Session interface {
	Global() inspect.Global

	Project() Project

	Options() Options

	Data() Data
	Dirs() SessionDirs

	SourceImportRegistry

	// FileRewrite
	// rewrite of the source file
	FileRewrite(f inspect.FileContext) GoRewriteEdit

	// FileEdit edit in place
	FileEdit(f inspect.FileContext) GoRewriteEdit

	// kind: apart from the original files, newly selected files
	// call with the same pkgPath and kind returns the same result
	PackageEdit(p inspect.Pkg, kind string) GoNewEdit

	// Gen generates contents
	Gen(callback EditCallback)
}

type SessionDirs interface {
	ProjectRoot() string
	RewriteRoot() string
	RewriteProjectRoot() string
	RewriteProjectVendorRoot() string
}

type Data interface {
	GetOK(key interface{}) (val interface{}, ok bool)
	Get(key interface{}) interface{}
	Set(key interface{}, val interface{})
	Del(key interface{})
}
