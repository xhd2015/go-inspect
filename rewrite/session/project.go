package session

import (
	"go/ast"

	"github.com/xhd2015/go-inspect/inspect"
)

type Project interface {
	Global() inspect.Global
	MainPkg() inspect.Pkg

	// Options return the options to be
	// used, guranteed to be not nil
	Options() LoadOptions
	Args() []string

	// AllocExtraPkg under main
	AllocExtraPkg(name string) (pkgName string)

	// AllocExtraFile under main
	AllocExtraFile(name string, suffix string) (fileName string)

	// AllocExtraFile under main
	AllocExtraPkgAt(dir string, name string) (fileName string)
	AllocExtraFileaAt(dir string, name string, suffix string) (fileName string)

	// file creation
	// NewFile create a file in rewritten root
	// without tracking any file
	NewFile(filePath string, content string)
	// ModifyFile modifes a file in rewritten root
	// with tracking
	ModifyFile(filePath string, content string)
	// ReplaceFile modifies the original source
	ReplaceFile(filePath string, content string)

	// DeriveFileFrom create a file with tracking
	DeriveFileFrom(filePath string, srcPath string, content string)

	ProjectRoot() string
	RewriteRoot() string
	RewriteProjectRoot() string

	IsVendor() bool

	// static tool
	HasImportPkg(f *ast.File, pkgNameQuoted string) bool
	ShortHash(s string) string
	ShortHashFile(f inspect.FileContext) string

	// Deprecated use session.Data() instead
	// SetData makes project serve as a context
	SetData(key interface{}, value interface{})
	GetData(key interface{}) (value interface{}, ok bool)
}

// LoadOptions are options that only
// related to load info that cannot be changed
// This is to make the build process reproducible,
// and is visible to user
type LoadOptions interface {
	Verbose() bool

	// GoFlags are common to load and build
	GoFlags() []string

	// BuildFlags only apply to build
	BuildFlags() []string
}
