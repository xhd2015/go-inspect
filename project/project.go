package project

import (
	"crypto/sha256"
	"fmt"
	"go/ast"
	"path"
	"path/filepath"

	"github.com/xhd2015/go-inspect/inspect"
	"github.com/xhd2015/go-inspect/inspect/util"
	"github.com/xhd2015/go-inspect/rewrite/session"
)

type sessionDirs struct {
	projectRoot string

	rewriteMetaRoot string // {temp}/{projectRoot_md5}

	rewriteRoot string // {temp}/{projectRoot_md5}/src

	rewriteProjectRoot string // {temp}/{projectRoot_md5}/src/{projectRoot}

	rewriteProjectVendorRoot string // {temp}/{projectRoot_md5}/vendor
}

var _ session.SessionDirs = (*sessionDirs)(nil)

func (c *sessionDirs) ProjectRoot() string {
	return c.projectRoot
}

// meta root
func (c *sessionDirs) RewriteMetaRoot() string {
	return c.rewriteMetaRoot
}
func (c *sessionDirs) RewriteMetaSubPath(subPath string) string {
	return filepath.Join(c.rewriteMetaRoot, subPath)
}

func (c *sessionDirs) RewriteRoot() string {
	return c.rewriteRoot
}

// source root
func (c *sessionDirs) RewriteProjectRoot() string {
	return c.rewriteProjectRoot
}

// vendor root
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

	vendor bool
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
