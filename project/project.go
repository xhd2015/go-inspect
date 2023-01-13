package project

import (
	"crypto/sha256"
	"fmt"
	"go/ast"
	"path"

	"github.com/xhd2015/go-inspect/inspect"
	"github.com/xhd2015/go-inspect/inspect/util"
	"github.com/xhd2015/go-inspect/rewrite"
)

type Project interface {
	Global() inspect.Global
	MainPkg() inspect.Pkg
	AllocExtraPkg(name string) (pkgName string)
	NewFile(filePath string, content string)

	RewriteRoot() string
	RewriteProjectRoot() string

	// static tool
	HasImportPkg(f *ast.File, pkgNameQuoted string) bool
	ShortHash(s string) string
	ShortHashFile(f inspect.FileContext) string
}

var _ Project = ((*project)(nil))

type project struct {
	g                  inspect.Global
	mainPkg            inspect.Pkg
	rewriteRoot        string
	rewriteProjectRoot string
	genMap             map[string]*rewrite.Content
}

// RewriteRoot implements Project
func (c *project) RewriteRoot() string {
	return c.rewriteRoot
}

// TargetDir implements Project
func (c *project) RewriteProjectRoot() string {
	return c.rewriteProjectRoot
}

// MainPkg implements Session
func (c *project) MainPkg() inspect.Pkg {
	return c.mainPkg
}

// Global implements Session
func (c *project) Global() inspect.Global {
	return c.g
}

// NewFile implements Session
func (c *project) NewFile(filePath string, content string) {
	c.genMap[rewrite.CleanGoFsPath(path.Join(c.rewriteRoot, filePath))] = &rewrite.Content{
		Content: []byte(content),
	}
}

// AllocExtraPkg implements Helper
func (c *project) AllocExtraPkg(name string) (pkgName string) {
	return util.NextFileNameUnderDir(c.mainPkg.Dir(), name, "")
}
func (c *project) HasImportPkg(f *ast.File, pkgNameQuoted string) bool {
	return HasImportPkg(f, pkgNameQuoted)
}
func (c *project) ShortHash(s string) string {
	return ShortHash(s)
}

// ShortHashFile implements Session
func (*project) ShortHashFile(f inspect.FileContext) string {
	return ShortHashFile(f)
}

func ShortHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	h := fmt.Sprintf("%x", sum[:6])

	return fmt.Sprintf("%x", h)
}
func ShortHashFile(f inspect.FileContext) string {
	return ShortHash(f.Pkg().Path() + "/" + path.Base(f.AbsPath()))
}

func HasImportPkg(f *ast.File, pkgNameQuoted string) bool {
	for _, imp := range f.Imports {
		if imp.Path != nil && imp.Path.Value == pkgNameQuoted {
			return true
		}
	}
	return false
}
