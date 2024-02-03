package session_impl

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/xhd2015/go-inspect/inspect"
	"github.com/xhd2015/go-inspect/inspect/util"
	source_import_internal "github.com/xhd2015/go-inspect/rewrite/internal/source_import"
	sessionpkg "github.com/xhd2015/go-inspect/rewrite/session"
	"github.com/xhd2015/go-inspect/rewrite/source_import"
	"github.com/xhd2015/go-vendor-pack/writefs/memfs"
)

type session struct {
	g       inspect.Global
	project sessionpkg.Project
	data    *sessionData

	dirs      sessionpkg.SessionDirs
	rewriteFS *memfs.MemFS

	opts sessionpkg.Options

	source_import_internal.SourceImportRegistryRetriever

	fileEditMap    util.SyncMap
	fileRewriteMap util.SyncMap
	pkgEditMap     util.SyncMap
}

var _ sessionpkg.Session = ((*session)(nil))

func NewSession(g inspect.Global, opts sessionpkg.Options, memfsRoot string) sessionpkg.Session {
	return &session{
		g:                             g,
		data:                          &sessionData{},
		opts:                          opts,
		rewriteFS:                     memfs.NewAtFs(memfsRoot),
		SourceImportRegistryRetriever: source_import.NewRegistry(),
	}
}

func OnSessionOpts(s sessionpkg.Session, opts sessionpkg.Options) {
	if s, ok := s.(*session); ok {
		s.opts = opts
	}
}

func OnSessionDirs(s sessionpkg.Session, dirs sessionpkg.SessionDirs) {
	if s, ok := s.(*session); ok {
		s.dirs = dirs
	}
}

func OnSessionGlobal(s sessionpkg.Session, g inspect.Global) {
	if s, ok := s.(*session); ok {
		s.g = g
	}
}

func OnSessionProject(s sessionpkg.Session, project sessionpkg.Project) {
	if s, ok := s.(*session); ok {
		s.project = project
	}
}

type sessionData struct {
	m sync.Map
}

var _ sessionpkg.Data = (*sessionData)(nil)

func (c *sessionData) GetOK(key interface{}) (val interface{}, ok bool) {
	return c.m.Load(key)
}

func (c *sessionData) Get(key interface{}) interface{} {
	val, _ := c.m.Load(key)
	return val
}

func (c *sessionData) Set(key interface{}, val interface{}) {
	c.m.Store(key, val)
}

func (c *sessionData) Del(key interface{}) {
	c.m.Delete(key)
}

type fileEntry struct {
	f    inspect.FileContext
	edit sessionpkg.GoRewriteEdit
}
type pkgEntry struct {
	pkg      inspect.Pkg
	kind     string
	realName string // no ".go" suffix
	edit     sessionpkg.GoNewEdit
}

// Global implements Session
func (c *session) Global() inspect.Global {
	return c.g
}
func (c *session) Project() sessionpkg.Project {
	return c.project
}

func (c *session) Data() sessionpkg.Data {
	return c.data
}

// Options implements Session.
func (c *session) Options() sessionpkg.Options {
	return c.opts
}

func (c *session) Dirs() sessionpkg.SessionDirs {
	return c.dirs
}

func (c *session) RewriteFS() *memfs.MemFS {
	return c.rewriteFS
}

// FileEdit implements Session
func (c *session) FileEdit(f inspect.FileContext) sessionpkg.GoRewriteEdit {
	absPath := f.AbsPath()
	v := c.fileEditMap.LoadOrCompute(absPath, func() interface{} {
		return &fileEntry{f: f, edit: NewGoRewrite(f)}
	})
	return v.(*fileEntry).edit
}

// FileRewrite implements Session
func (c *session) FileRewrite(f inspect.FileContext) sessionpkg.GoRewriteEdit {
	absPath := f.AbsPath()
	v := c.fileRewriteMap.LoadOrCompute(absPath, func() interface{} {
		return &fileEntry{f: f, edit: NewGoRewrite(f)}
	})
	return v.(*fileEntry).edit
}

// PackageEdit implements Session
func (c *session) PackageEdit(p inspect.Pkg, kind string) sessionpkg.GoNewEdit {
	pkgPath := p.Path()
	key := fmt.Sprintf("%s:%s", pkgPath, kind)
	v := c.pkgEditMap.LoadOrCompute(key, func() interface{} {
		realName := util.NextName(func(s string) bool {
			foundGo := false
			p.RangeFiles(func(i int, f inspect.FileContext) bool {
				foundGo = foundGo || filepath.Base(f.AbsPath()) == s+".go"
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
func (c *session) Gen(callback sessionpkg.EditCallback) {
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

func (c *session) SetRewriteFile(filePath string, content string) error {
	p := CleanGoFsPath(path.Join(c.dirs.RewriteRoot(), filePath))
	return c.setFile(p, content)
}

func (c *session) ReplaceFile(filePath string, content string) error {
	p := CleanGoFsPath(path.Join(c.dirs.ProjectRoot(), filePath))
	return c.setFile(p, content)
}

func (c *session) setFile(filePath string, content string) error {
	err := c.rewriteFS.MkdirAll(filepath.Dir(filePath), 0755)
	if err != nil {
		return err
	}
	w, err := c.rewriteFS.OpenFileWrite(filePath)
	if err != nil {
		return err
	}
	defer w.Close()
	_, err = w.Write([]byte(content))
	if err != nil {
		return err
	}
	return nil
}

// go's replace cannot have '@' character, so we replace it with ver_
// this is used for files to be copied into tmp dir, and will appear on replace verb.
func CleanGoFsPath(s string) string {
	// example:
	// /Users/xhd2015/Projects/gopath/pkg/mod/google.golang.org/grpc@v1.47.0/xds
	return strings.ReplaceAll(s, "@", "/")
}
