package session

import (
	"github.com/xhd2015/go-inspect/inspect"
	"github.com/xhd2015/go-vendor-pack/writefs/memfs"
)

// Session session represents a rewrite pass
type Session interface {
	Global() inspect.Global

	Project() Project

	Options() Options

	Data() Data
	Dirs() SessionDirs

	// files
	RewriteFS() *memfs.MemFS

	FileEditor

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
	RewriteMetaRoot() string
	RewriteMetaSubPath(subPath string) string
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

type FileEditor interface {
	// create a file in rewrite source root
	// e.g. {rewriteRoot}/{projectPath}/src/...
	SetRewriteFile(filePath string, content string) error
	// ReplaceFile modifies the original source
	ReplaceFile(filePath string, content string) error

	// file creation
	// NewFile create a file in rewritten root
	// without tracking any file
	// Deprecated: use SetRewriteFile instead
	// NewFile(filePath string, content string) error

	// ModifyFile modifes a file in rewritten root
	// with tracking
	// Deprecated
	// ModifyFile(filePath string, content string) error
	// DeriveFileFrom create a file with tracking
	// Deprecated
	// DeriveFileFrom(filePath string, srcPath string, content string) error
}
