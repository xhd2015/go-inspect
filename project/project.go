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
	// AllocExtraPkg under main
	AllocExtraPkg(name string) (pkgName string)

	// AllocExtraFile under main
	AllocExtraFile(name string, suffix string) (fileName string)

	// file creation
	NewFile(filePath string, content string)
	ModifyFile(filePath string, content string)
	ReplaceFile(filePath string, content string)
	DeriveFileFrom(filePath string, srcPath string, content string)

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
	projectRoot        string
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

// ModifyFile implements Session
func (c *project) ModifyFile(filePath string, content string) {
	c.genMap[rewrite.CleanGoFsPath(path.Join(c.rewriteRoot, filePath))] = &rewrite.Content{
		SrcFile: path.Join(c.projectRoot, filePath),
		Content: []byte(content),
	}
}

// ModifyFile implements Session
func (c *project) ReplaceFile(filePath string, content string) {
	c.genMap[rewrite.CleanGoFsPath(path.Join(c.projectRoot, filePath))] = &rewrite.Content{
		Content: []byte(content),
	}
}

// NewFileAsFrom implements Session
func (c *project) DeriveFileFrom(filePath string, srcPath string, content string) {
	c.genMap[rewrite.CleanGoFsPath(path.Join(c.rewriteRoot, filePath))] = &rewrite.Content{
		SrcFile: path.Join(c.projectRoot, srcPath),
		Content: []byte(content),
	}
}

// AllocExtraPkg implements Helper
func (c *project) AllocExtraPkg(name string) (pkgName string) {
	return path.Join(c.mainPkg.Dir(),util.NextFileNameUnderDir(c.mainPkg.Dir(), name, ""))
}

// AllocExtraPkg implements Helper
func (c *project) AllocExtraFile(name string, suffix string) (fileName string) {
	return path.Join(c.mainPkg.Dir(),util.NextFileNameUnderDir(c.mainPkg.Dir(), name, suffix))
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
