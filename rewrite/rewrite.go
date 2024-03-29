package rewrite

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xhd2015/go-inspect/rewrite/session/session_impl"
	"github.com/xhd2015/go-inspect/rewrite/source_import"
	"github.com/xhd2015/go-vendor-pack/go_cmd"
	"github.com/xhd2015/go-vendor-pack/go_cmd/model"
	"github.com/xhd2015/go-vendor-pack/writefs"

	"github.com/xhd2015/go-inspect/code/gen"
	"github.com/xhd2015/go-inspect/filecopy"
	"github.com/xhd2015/go-inspect/inspect"
	"github.com/xhd2015/go-inspect/inspect/load"
	"github.com/xhd2015/go-inspect/inspect/util"
	session_pkg "github.com/xhd2015/go-inspect/rewrite/session"
)

type Controller interface {
	BeforeLoad(opts *BuildRewriteOptions, session session_pkg.Session)
	InitSession(g inspect.Global, session session_pkg.Session)
	AfterLoad(g inspect.Global, session session_pkg.Session)
	// FilterPkgs, defaults to starter packages
	FilterPkgs(g inspect.Global, session session_pkg.Session) func(func(p inspect.Pkg, pkgFlag PkgFlag) bool)
	BeforeCopy(g inspect.Global, session session_pkg.Session)
	// GenOverlay generate overlay for src files.
	// Overlay is a rewritten content of the original file or just a generated content
	// without original file/dir.
	GenOverlay(g inspect.Global, session session_pkg.Session)
}

type ControllerFuncs struct {
	BeforeLoadFn  func(opts *BuildRewriteOptions, session session_pkg.Session)
	InitSessionFn func(g inspect.Global, session session_pkg.Session)
	AfterLoadFn   func(g inspect.Global, session session_pkg.Session)
	FilterPkgsFn  func(g inspect.Global, session session_pkg.Session) func(func(p inspect.Pkg, pkgFlag PkgFlag) bool)
	BeforeCopyFn  func(g inspect.Global, session session_pkg.Session)
	GenOverlayFn  func(g inspect.Global, session session_pkg.Session)
}
type Content struct {
	SrcFile string
	Content []byte
}

// Why this PkgFlag?
type PkgFlag int

const (
	BitExtra = 1 << iota
	BitStarter
	BitStarterMod
)

// IsExtra controls whether a package should be:
// - rewritten
// - copied to rewrite root
// - replaced in go.mod
// TODO: make this clear
func (c PkgFlag) IsExtra() bool {
	return c&BitExtra == 1
}
func (c PkgFlag) IsStarter() bool {
	return c&BitStarter == 1
}
func (c PkgFlag) IsStarterMod() bool {
	return c&BitStarterMod == 1
}

func (c *ControllerFuncs) BeforeLoad(opts *BuildRewriteOptions, session session_pkg.Session) {
	if c.BeforeLoadFn == nil {
		return
	}
	c.BeforeLoadFn(opts, session)
}
func (c *ControllerFuncs) InitSession(g inspect.Global, session session_pkg.Session) {
	if c.InitSessionFn == nil {
		return
	}
	c.InitSessionFn(g, session)
}

func (c *ControllerFuncs) AfterLoad(g inspect.Global, session session_pkg.Session) {
	if c.AfterLoadFn == nil {
		return
	}
	c.AfterLoadFn(g, session)
}

func (c *ControllerFuncs) FilterPkgs(g inspect.Global, session session_pkg.Session) func(func(p inspect.Pkg, pkgFlag PkgFlag) bool) {
	if c.FilterPkgsFn == nil {
		return nil
	}
	return c.FilterPkgsFn(g, session)
}

func (c *ControllerFuncs) BeforeCopy(g inspect.Global, session session_pkg.Session) {
	if c.BeforeCopyFn == nil {
		return
	}
	c.BeforeCopyFn(g, session)
}
func (c *ControllerFuncs) GenOverlay(g inspect.Global, session session_pkg.Session) {
	if c.GenOverlayFn == nil {
		return
	}
	c.GenOverlayFn(g, session)
}

func GetTmpRewriteRoot(name string) string {
	return path.Join(os.TempDir(), name)
}
func GetRewriteRoot(root string, name string) string {
	return path.Join(root, name)
}

// go's replace cannot have '@' character, so we replace it with ver_
// this is used for files to be copied into tmp dir, and will appear on replace verb.
func CleanGoFsPath(s string) string {
	// example:
	// /Users/xhd2015/Projects/gopath/pkg/mod/google.golang.org/grpc@v1.47.0/xds
	return strings.ReplaceAll(s, "@", "/")
}

// TODO: vendor mod should behave differently
// `ctrl` is responsible for filtering packages, and generate file map
// `rewritter` is responsible for do the actual rewriting work
// There are two phases for calling filecopy, but for 2 different operations: one for file copying the original code, and another for generating content.
//
// the first phase of filecopy.SyncRebase:
// packages specified by args, and by ctrl.FilterPkgs with Extra bit set. These packages are collected and collasped to their modules, as we must make all packages under the same module one place. In short, modifying single package results in the whole enclosing module to be copied.
//
// the second phase of filecopy.SyncGenerated:
// overlay, which is for generated file.
func GenRewrite(args []string, rewriteRoot string, ctrl Controller, rewritter Visitor, opts *BuildRewriteOptions) (res *GenRewriteResult, err error) {
	res = &GenRewriteResult{}
	if opts == nil {
		opts = &BuildRewriteOptions{}
	}
	verbose := opts.Verbose
	verboseCopy := opts.VerboseCopy
	verboseRewrite := opts.VerboseRewrite
	// verboseCost := false
	verboseCost := true

	if rewriteRoot == "" {
		panic(fmt.Errorf("rewriteRoot is empty"))
	}
	if opts.Verbose {
		log.Printf("rewrite root: %s", rewriteRoot)
	}
	err = os.MkdirAll(rewriteRoot, 0777)
	if err != nil {
		err = fmt.Errorf("error mkdir %s %v", rewriteRoot, err)
		return
	}

	projectDir := opts.ProjectDir
	projectDir, err = util.ToAbsPath(projectDir)
	if err != nil {
		err = fmt.Errorf("get abs dir err:%v", err)
		return
	}
	// src-shadow
	memfsDir := filepath.Join(filepath.Dir(rewriteRoot), filepath.Base(rewriteRoot)+"-shadow")
	// create a session, and rewrite
	session := session_impl.NewSession(nil /* filled later*/, nil /*filled later: this is a workaround*/, memfsDir)

	ctrl.BeforeLoad(opts, session)

	loadPkgTime := time.Now()

	g, err := load.LoadPackages(args, &load.LoadOptions{
		ProjectDir: projectDir,
		ForTest:    opts.ForTest,
		BuildFlags: opts.GoFlags,
	})
	if err != nil {
		err = fmt.Errorf("loading packages err: %v", err)
		return
	}

	// hack: this is a workaround
	session_impl.OnSessionGlobal(session, g)

	// give everyone a chance to init necessary data
	// logic in this phase should be as lightweight as possible
	ctrl.InitSession(g, session)

	// starts to do heavy logic
	ctrl.AfterLoad(g, session)

	loadPkgEnd := time.Now()
	if verboseCost {
		log.Printf("COST load packages:%v", loadPkgEnd.Sub(loadPkgTime))
	}

	// ensure that starterPkgs have exactly one module
	mainMod := g.LoadInfo().MainModule()
	modPath, modDir := mainMod.Path(), mainMod.Dir()
	if verbose {
		log.Printf("current module: %s , dir %s", modPath, modDir)
	}

	// validate
	starterPkgs := g.LoadInfo().StarterPkgs()
	if len(starterPkgs) == 0 {
		err = fmt.Errorf("no packages loaded")
		return
	}

	// filter pkgs
	pkgsFn := ctrl.FilterPkgs(g, session)
	if pkgsFn == nil {
		// choose starterPkgs
		pkgsFn = func(f func(p inspect.Pkg, pkgFlag PkgFlag) bool) {
			for _, p := range g.LoadInfo().StarterPkgs() {
				if !f(p, BitStarterMod|BitStarter) {
					return
				}
			}
		}
	}

	// expand to all packages under the same module that depended by starter packages
	// rewrite
	rewriteTime := time.Now()
	if verboseCost {
		log.Printf("COST load package -> rewrite package:%v", rewriteTime.Sub(loadPkgEnd))
	}

	VisitAll(func(f func(pkg inspect.Pkg) bool) {
		pkgsFn(func(p inspect.Pkg, pkgFlag PkgFlag) bool {
			return f(p)
		})
	}, session, rewritter)
	rewriteEnd := time.Now()
	if verboseCost {
		log.Printf("COST rewrite:%v", rewriteEnd.Sub(rewriteTime))
	}

	// TODO: move vendor detection to Global
	extraPkgInVendor := false

	// hasStd indicates whether the standard
	// lib is rewritten, for example: runtime
	hasStd := opts.RewriteStd
	hasExtra := false

	pkgCnt := 0
	pkgsFn(func(p inspect.Pkg, pkgFlag PkgFlag) bool {
		pkgCnt++
		hasStd = hasStd || p.Module().IsStd()
		if !pkgFlag.IsExtra() {
			return true
		}
		hasExtra = true
		// checking vendor
		if !extraPkgInVendor {
			dir := p.Module().Dir()
			if dir == "" {
				// has module, but no dir
				// check if any file is inside vendor
				if util.IsVendor(modDir, p.GoPkg().GoFiles[0]) /*empty GoFiles are filtered*/ {
					extraPkgInVendor = true
					return true // break the loop
				}
			}
		}
		return true
	})

	if hasStd {
		res.UseNewGOROOT = g.GOROOT()
	}

	if verbose {
		if hasExtra {
			log.Printf("extra packages in vendor:%v", extraPkgInVendor)
		}
	}

	// copy files
	var destUpdatedBySource map[string]bool // todo: clear
	doCopy := func() {
		if verbose {
			log.Printf("copying packages files into rewrite dir: total packages=%d", pkgCnt)
		}
		copyTime := time.Now()
		destUpdatedBySource = copyPackageFiles(pkgsFn, session.RewriteFS(), rewriteRoot, extraPkgInVendor, hasStd, opts.Force, verboseCopy, verbose)
		copyEnd := time.Now()
		if verboseCost {
			log.Printf("COST copy:%v", copyEnd.Sub(copyTime))
		}
	}
	_ = destUpdatedBySource
	doCopy()

	// NOTE: only non-vendor needs to replace relative module path
	// with absolute path, because vendored packages are inside
	// vendor
	if !extraPkgInVendor {
		// TODO: edit go mod in JSON and format back
		// mod replace only work at module-level, so if at least
		// one package inside a module is modified, we need to
		// copy its module out.
		doMod := func() {
			// after copied, modify go.mod with replace absoluted
			if verbose {
				log.Printf("replacing go.mod with rewritten paths")
			}
			goModTime := time.Now()
			res.MappedMod = makeGomodReplaceAboslute(session.RewriteFS(), pkgsFn, rewriteRoot, projectDir, verbose)
			goModEnd := time.Now()
			if verboseCost {
				log.Printf("COST go mod:%v", goModEnd.Sub(goModTime))
			}
		}
		doMod()
	}

	ctrl.BeforeCopy(g, session)

	// import source first
	// RewriteFS fill override order(least order higher priority):
	//   original source -> source import -> overlay -> rewrite each file
	source_import.OnSessionGenOverlay(session)

	// NOTE: paths in the backMap
	// are absolute, i.e. ${REWRITE_ROOT}/${ORIG_DIR}/file.go
	// so it's possible to put anything in here
	// the upper layer package 'project' will
	// ensure these files are all rooted at ${REWRITE_ROOT},
	// including GOROOT/src rewritted ones.
	ctrl.GenOverlay(g, session)

	rewriteFS := session.RewriteFS()
	// var disableDigest bool

	// it seems that go cache is happy with content overridding
	// disableDigest = true

	// TODO: make file path relative to rewrite root

	var updatedDigestMap sync.Map // map[string]string
	var savedDigestMap map[string]string

	srcMD5File := session.Dirs().RewriteMetaSubPath("src-md5.json")
	if !opts.Force {
		// with Force = true
		// ShouldCopyFile is not actually called, so
		// digest will not be updated,
		// that leaves a un synchronized version of digest
		// so we simply clear it
		var srcMD5Content []byte
		srcMD5Content, err = ioutil.ReadFile(srcMD5File)
		if err != nil && !writefs.IsNotExist(err) {
			err = fmt.Errorf("read %s: %w", filepath.Base(srcMD5File), err)
			return
		}
		if len(srcMD5Content) > 0 {
			jsonErr := json.Unmarshal(srcMD5Content, &savedDigestMap)
			if jsonErr != nil {
				log.Printf("WARN bad src-md5.json ignored: %v", err)
			}
		}
	}

	// DEBUG
	// for file, content := range backMap {
	// 	if strings.Contains(file, "dialect.go") {
	// 		b := content
	// 		_ = content
	// 		a := b
	// 		b = a
	// 	}
	// }
	// in this copy config, srcPath is the same with destPath
	// the extra info is looked up in a back map

	// var changedFiles int64
	copyBegin := time.Now()
	err = filecopy.SyncFS(
		rewriteFS,
		[]string{rewriteRoot},
		"", // target dir already rooted
		filecopy.SyncRebaseOptions{
			Ignores:        ignores,
			DeleteNotFound: true,
			Force:          opts.Force,
			// ProcessDestPath: cleanFsGoPath, // not needed as we already did that
			OnUpdateStats: filecopy.NewLogger(func(format string, args ...interface{}) {
				log.Printf(format, args...)
			}, verboseRewrite, verbose, 200*time.Millisecond),
			DidCopy: func(srcPath, destPath string) {
			},
			ShouldCopyFile: func(srcPath string, destPath string, srcFileInfo filecopy.FileInfo, destFileInfo os.FileInfo) (bool, error) { /*isSourceNewer, i.e. true=needCopy*/
				f, err := rewriteFS.OpenFileRead(destPath)
				if err != nil {
					return false, err
				}
				defer f.Close()

				// TODO: may give an option
				// do discard digest in CI environment
				h := md5.New()
				_, err = io.Copy(h, f)
				if err != nil {
					return false, err
				}

				curDigest := hex.EncodeToString(h.Sum(nil))
				savedDigest := savedDigestMap[destPath]
				if savedDigest == "" || savedDigest != curDigest {
					// if atomic.AddInt64(&changedFiles, 1) < 10 {
					// log.Printf("DEBUG write %s, digest:%s -> %s", destPath, savedDigest, curDigest)
					// }

					updatedDigestMap.Store(destPath, curDigest)
					return true, nil
				}
				return false, nil
			},
		},
	)
	copyEnd := time.Now()
	if verboseCost {
		log.Printf("COST copy files to build root: %v", copyEnd.Sub(copyBegin))
		// log.Printf("COST copy files to build root: %v, %d changed", copyEnd.Sub(copyBegin), atomic.LoadInt64(&changedFiles))
	}

	// clear, to help GC reclaim spaces
	// runtime.SetFinalizer(session, func(s session_pkg.Session) {
	// 	log.Printf("DEBUG session end")
	// })

	// runtime.SetFinalizer(rewriteFS, func(*memfs.MemFS) {
	// 	log.Printf("DEBUG rewriteFS end")
	// })
	session = nil
	rewriteFS = nil
	// log.Printf("DEBUG GC start")
	// log.Printf("DEBUG GC finished")

	if err != nil {
		return
	}
	updatedDigestMap.Range(func(file, digest interface{}) bool {
		if savedDigestMap == nil {
			savedDigestMap = make(map[string]string)
		}
		savedDigestMap[file.(string)] = digest.(string)
		return true
	})
	// update dest digest(when force is true, all digest are cleared)
	newDigestData, err := json.Marshal(savedDigestMap)
	if err != nil {
		err = fmt.Errorf("marhsal digest map: %w", err)
		return
	}
	err = ioutil.WriteFile(srcMD5File, newDigestData, 0755)
	if err != nil {
		err = fmt.Errorf("write digest map: %w", err)
		return
	}

	if verboseCost {
		log.Printf("COST load->rewrite->copy:%v", time.Since(loadPkgTime))
	}
	return
}

var ignores = []string{"(.*/)?\\.git\\b", "(.*/)?node_modules\\b"}

// copyPackageFiles copy starter packages(with all packages under the same module) and extra packages into rootDir, to bundle them together.
func copyPackageFiles(pkgs func(func(p inspect.Pkg, flag PkgFlag) bool), fs writefs.FS, rootDir string, extraPkgInVendor bool, hasStd bool, force bool, verboseDetail bool, verboseOverall bool) (destUpdated map[string]bool) {
	var dirList []string
	fileIgnores := append([]string(nil), ignores...)

	// in test mode, go loads 3 types package under the same dir:
	// 1.normal package
	// 2.bridge package, which contains module
	// 3.test package, which does not contain module

	// copy go files
	moduleDirs := make(map[string]bool)
	pkgs(func(p inspect.Pkg, flag PkgFlag) bool {
		if flag.IsExtra() && extraPkgInVendor {
			// if extra, and extra in vendor, don't copy

			// TODO may ignore vendor in the other branch
			// NOTE: not ignoring vendor
			// ignores = append(ignores, "vendor")
			return true
		}
		// std packages are processed as a whole
		if p.IsTest() || p.Module().IsStd() {
			return true
		}
		moduleDirs[p.Module().Dir()] = true
		return true
	})

	dirList = make([]string, 0, len(moduleDirs))
	for modDir := range moduleDirs {
		dirList = append(dirList, modDir)
	}
	if hasStd {
		// TODO: what if GOROOT is /usr/local/bin?
		// it also has /usr/local/go/src
		dirList = append(dirList, util.GetGOROOT())
	}
	// copy other pkgs (deprecated, this only copies package files, but we need to module if any package is modfied.may be used in the future when overlay is supported)
	// for _, p := range extraPkgs {
	// 	if p.Module == nil {
	// 		panic(fmt.Errorf("package has no module:%v", p.PkgPath))
	// 	}
	// 	dirList = append(dirList, inspect.GetFsPathOfPkg(p.Module, p.PkgPath))
	// }

	var destUpdatedM sync.Map

	size := int64(0)
	err := filecopy.SyncRebase(dirList, rootDir, filecopy.SyncRebaseOptions{
		Ignores:         fileIgnores,
		Force:           true, // always set to true to force read all files into memory
		DeleteNotFound:  true, // uncovered files are deleted
		ProcessDestPath: cleanGoFsPath,
		OnUpdateStats: filecopy.NewLogger(func(format string, args ...interface{}) {
			log.Printf(format, args...)
		}, verboseDetail, verboseOverall, 200*time.Millisecond),
		DidCopy: func(srcPath, destPath string) {
			destUpdatedM.Store(destPath, true)
			atomic.AddInt64(&size, 1)
		},
		FS: fs,
	})

	destUpdated = make(map[string]bool, atomic.LoadInt64(&size))
	destUpdatedM.Range(func(destPath, value interface{}) bool {
		destUpdated[destPath.(string)] = true
		return true
	})

	// err := CopyDirs(dirList, rootDir, CopyOpts{
	// 	Verbose:     verbose,
	// 	IgnoreNames: ignores,
	// 	ProcessDest: cleanGoFsPath,
	// })
	if err != nil {
		panic(err)
	}
	return
}

// go mod's replace, find relative paths and replace them with absolute path
func makeGomodReplaceAboslute(fs writefs.FS, pkgs func(func(pkg inspect.Pkg, flag PkgFlag) bool), rebaseDir string, projectDir string, verbose bool) (mappedMod map[string]string) {
	// if there is vendor/modules.txt, should also replace there
	replaceMap := make(map[string]string)

	type goModReplace struct {
		oldPath string
		newPath string
	}
	// goModEditReplace := func(oldpath string, newPath string) string {
	// 	return fmt.Sprintf("go mod edit -replace=%s=%s", sh.Quote(oldpath), sh.Quote(newPath))
	// }
	// premap: modPath -> ${rebaseDir}/${modDir}
	preMap := make(map[string]string)
	var preReplaceList []goModReplace
	mappedMod = make(map[string]string)

	// get modules(for mods, actually only 1 module, i.e. the current module will be processed)
	mods := make([]inspect.Module, 0, 1)
	modMap := make(map[string]bool, 1)
	pkgs(func(p inspect.Pkg, flag PkgFlag) bool {
		mod := p.Module()
		if mod == nil {
			if flag.IsExtra() {
				panic(fmt.Errorf("cannot replace non-module package:%v", p.Path()))
			}
			return true
		}
		// must use original path
		// some replace have:
		//   path: x/y/z
		//   replace.path: ../y/z
		modPath := mod.OrigPath()
		modDir := mod.Dir()
		// extra pkg
		if flag.IsExtra() {
			if mod.IsStd() {
				// std modules are replaced via golabl env: GOROOT=xxx
				return true
			}
			if preMap[modPath] != "" {
				return true
			}

			// dir always absolute
			cleanDir := cleanGoFsPath(modDir)
			newPath := path.Join(rebaseDir, cleanDir)
			preMap[modPath] = newPath
			// preReplaceList = append(preReplaceList, goModEditReplace(modPath, newPath))
			preReplaceList = append(preReplaceList, goModReplace{oldPath: modPath, newPath: newPath})
			replaceMap[modPath] = newPath

			mappedMod[modDir] = cleanDir
			return true
		}

		// normal pkg
		if modMap[modPath] {
			return true
		}

		modMap[modPath] = true
		mods = append(mods, mod)
		return true
	})
	for _, mod := range mods {
		dir := mod.Dir()
		origDir := dir
		// rebase to rootDir
		if rebaseDir != "" {
			dir = path.Join(rebaseDir, dir)
		}
		goModFile := filepath.Join(dir, "go.mod")

		var gomod *model.GoMod
		var err error

		if _, ok := fs.(writefs.SysFS); ok {
			gomod, err = go_cmd.ParseGoMod(dir)
		} else {
			var content []byte
			content, err = writefs.ReadFile(fs, goModFile)
			if err != nil {
				panic(err)
			}
			gomod, err = go_cmd.ParseGoModContent(string(content))
		}
		if err != nil {
			panic(err)
		}

		// replace with absolute paths
		var replaceList []goModReplace
		if len(gomod.Replace) > 0 {
			replaceList = make([]goModReplace, 0, len(gomod.Replace))
		}

		for _, rp := range gomod.Replace {
			newPath := preMap[rp.Old.Path]
			// skip replace made by us
			if newPath != "" {
				continue
			}
			if strings.HasPrefix(rp.New.Path, "./") || strings.HasPrefix(rp.New.Path, "../") {
				oldv := rp.Old.Path
				if rp.Old.Version != "" {
					oldv += "@" + rp.Old.Version
				}
				newPath := path.Join(origDir, rp.New.Path)
				// replaceList = append(replaceList, goModEditReplace(oldv, newPath))
				replaceList = append(replaceList, goModReplace{oldPath: oldv, newPath: newPath})

				// replace vendor/modules.txt without version will effectively replace all version
				// # PKG => PATH
				// # PKG VERSION => PATH
				replaceMap[rp.Old.Path] = newPath
			}
		}

		if len(replaceList) > 0 || len(preReplaceList) > 0 {
			if verbose {
				log.Printf("make absolute replace in go.mod for %v", mod.OrigPath())
			}
			doCmds := func(goModFile string) error {
				for _, replace := range replaceList {
					err := go_cmd.GoModReplace(goModFile, replace.oldPath, replace.newPath)
					if err != nil {
						return fmt.Errorf("replace %s->%s: %w", replace.oldPath, replace.newPath, err)
					}
				}
				for _, replace := range preReplaceList {
					err := go_cmd.GoModReplace(goModFile, replace.oldPath, replace.newPath)
					if err != nil {
						return fmt.Errorf("pre replace %s->%s: %w", replace.oldPath, replace.newPath, err)
					}
				}
				return nil
			}

			var cmdErr error
			if _, ok := fs.(writefs.SysFS); ok {
				cmdErr = doCmds(goModFile)
			} else {
				// read and update
				content, err := writefs.ReadFile(fs, goModFile)
				if err != nil {
					panic(err)
				}
				newGoMod, err := go_cmd.GoModEdit(string(content), doCmds)
				if err != nil {
					panic(err)
				}
				cmdErr = writefs.WriteFile(fs, goModFile, []byte(newGoMod))
			}
			if cmdErr != nil {
				panic(cmdErr)
			}
		}
	}

	// because files are already copied, so we can check vendor/modules.txt locally
	vendorErr := appendVendorModulesIgnoreNonExist(fs, path.Join(rebaseDir, projectDir, "vendor/modules.txt"), replaceMap)
	if vendorErr != nil {
		log.Printf("ERROR failed to update vendor/modules.txt: %v\n", vendorErr)
		panic(vendorErr)
	}
	return
}
func appendVendorModulesIgnoreNonExist(fs writefs.FS, path string, replaceMap map[string]string) (err error) {
	if len(replaceMap) == 0 {
		return
	}
	bytes, err := writefs.ReadFile(fs, path)
	if err != nil {
		if writefs.IsNotExist(err) {
			// if the file does not exist, we can safely ignore updating it
			err = nil
		}
		return
	}

	// replace lines
	lines := strings.Split(string(bytes), "\n")
	for i, line := range lines {
		if !strings.HasPrefix(line, "# ") {
			continue
		}
		lineTail := line[2:]
		idx := strings.Index(lineTail, "=>")
		if idx < 0 {
			continue
		}
		// it could have a version
		pkgVersion := strings.SplitN(strings.TrimSpace(lineTail[:idx]), " ", 2)
		var pkg string
		var version string
		if len(pkgVersion) > 0 {
			pkg = pkgVersion[0]
		}
		if len(pkgVersion) > 1 {
			version = pkgVersion[1]
		}
		replace := replaceMap[pkg]
		if replace == "" {
			continue
		}
		newLine := "# " + pkg
		if version != "" {
			newLine += " " + version
		}
		newLine += " => " + replace
		lines[i] = newLine
	}

	err = writefs.WriteFile(fs, path, []byte(strings.Join(lines, "\n")))
	return
}

// genImportListContent
// Deprecated: mock are registered in original package,not in a standalone import file
func genImportListContent(stubInitEntryDir string, mockPkgList []string) string {
	stubGen := gen.NewTemplateBuilder().Block(
		fmt.Sprintf("package %s", path.Base(stubInitEntryDir)),
		"",
		"import (",
	)
	for _, mokcPkg := range mockPkgList {
		stubGen.Block(fmt.Sprintf(`    _ %q`, mokcPkg))
	}
	stubGen.Block(
		")",
	)
	return stubGen.Format(nil)
}

// go's replace cannot have '@' character, so we replace it with ver_
// this is used for files to be copied into tmp dir, and will appear on replace verb.
func cleanGoFsPath(s string) string {
	// example:
	// /Users/xhd2015/Projects/gopath/pkg/mod/google.golang.org/grpc@v1.47.0/xds
	return strings.ReplaceAll(s, "@", "/")
}
