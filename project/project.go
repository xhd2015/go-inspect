package project

import (
	"crypto/sha256"
	"fmt"
	"go/ast"
	"path"

	"github.com/xhd2015/go-inspect/inspect"
	"github.com/xhd2015/go-inspect/inspect/util"
	"github.com/xhd2015/go-inspect/rewrite"
	"github.com/xhd2015/go-inspect/rewrite/session"
)

type sessionDirs struct {
	projectRoot string
	rewriteRoot string

	rewriteProjectRoot string // {rewriteRoot}/{projectRoot}/source

	rewriteProjectVendorRoot string // {rewriteRoot}/{projectRoot}/vendor
}

func (c *sessionDirs) RewriteRoot() string {
	return c.rewriteRoot
}
func (c *sessionDirs) ProjectRoot() string {
	return c.projectRoot
}

func (c *sessionDirs) RewriteProjectRoot() string {
	return c.rewriteProjectRoot
}
func (c *sessionDirs) RewriteProjectVendorRoot() string {
	return c.rewriteProjectVendorRoot
}

var _ session.Project = ((*project)(nil))

type project struct {
	g           inspect.Global
	mainPkg     inspect.Pkg
	opts        *loadOptions
	args        []string
	projectRoot string

	// TODO: move these fields to session
	rewriteRoot        string
	rewriteProjectRoot string

	// TODO: move to session
	genMap map[string]*rewrite.Content

	vendor bool

	// deprecated
	// use session.Data() instead
	ctxData map[interface{}]interface{}
}

// AllocExtraFileaAt implements Project
func (c *project) AllocExtraFileaAt(dir string, name string, suffix string) (fileName string) {
	return path.Join(dir, util.NextFileNameUnderDir(dir, name, suffix))
}

// AllocExtraPkgAt implements Project
func (c *project) AllocExtraPkgAt(dir string, name string) (fileName string) {
	return path.Join(dir, util.NextFileNameUnderDir(dir, name, ""))
}

// Options implements Project
func (c *project) Options() session.LoadOptions {
	return c.opts
}
func (c *project) Args() []string {
	return c.args
}

func (c *project) ProjectRoot() string {
	return c.projectRoot
}

// RewriteRoot implements Project
func (c *project) RewriteRoot() string {
	return c.rewriteRoot
}

// TargetDir implements Project
func (c *project) RewriteProjectRoot() string {
	return c.rewriteProjectRoot
}

func (c *project) IsVendor() bool {
	return c.vendor
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
	return path.Join(c.mainPkg.Dir(), util.NextFileNameUnderDir(c.mainPkg.Dir(), name, ""))
}

// AllocExtraPkg implements Helper
func (c *project) AllocExtraFile(name string, suffix string) (fileName string) {
	return path.Join(c.mainPkg.Dir(), util.NextFileNameUnderDir(c.mainPkg.Dir(), name, suffix))
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

// GetData implements Project
func (c *project) GetData(key interface{}) (value interface{}, ok bool) {
	value, ok = c.ctxData[key]
	return
}

// SetData implements Project
func (c *project) SetData(key interface{}, value interface{}) {
	c.ctxData[key] = value
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
