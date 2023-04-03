package rewrite

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xhd2015/go-inspect/code/gen"
	"github.com/xhd2015/go-inspect/filecopy"
	"github.com/xhd2015/go-inspect/inspect"
	"github.com/xhd2015/go-inspect/inspect/load"
	"github.com/xhd2015/go-inspect/inspect/util"
	"github.com/xhd2015/go-inspect/sh"
)

type Controller interface {
	BeforeLoad(opts *BuildRewriteOptions)
	AfterLoad(g inspect.Global)
	// FilterPkgs, defaults to starter packages
	FilterPkgs(g inspect.Global) func(func(p inspect.Pkg, pkgFlag PkgFlag) bool)
	BeforeCopy(g inspect.Global, session inspect.Session)
	// GenOverlay generate overlay for src files.
	// Overlay is a rewritten content of the original file or just a generated content
	// without original file/dir.
	GenOverlay(g inspect.Global, session inspect.Session) map[string]*Content
}

type ControllerFuncs struct {
	BeforeLoadFn func(opts *BuildRewriteOptions)
	AfterLoadFn  func(g inspect.Global)
	FilterPkgsFn func(g inspect.Global) func(func(p inspect.Pkg, pkgFlag PkgFlag) bool)
	BeforeCopyFn func(g inspect.Global, session inspect.Session)
	GenOverlayFn func(g inspect.Global, session inspect.Session) map[string]*Content
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

func (c PkgFlag) IsExtra() bool {
	return c&BitExtra == 1
}
func (c PkgFlag) IsStarter() bool {
	return c&BitStarter == 1
}
func (c PkgFlag) IsStarterMod() bool {
	return c&BitStarterMod == 1
}

func (c *ControllerFuncs) BeforeLoad(opts *BuildRewriteOptions) {
	if c.BeforeLoadFn == nil {
		return
	}
	c.BeforeLoadFn(opts)
}

func (c *ControllerFuncs) AfterLoad(g inspect.Global) {
	if c.AfterLoadFn == nil {
		return
	}
	c.AfterLoadFn(g)
}

func (c *ControllerFuncs) FilterPkgs(g inspect.Global) func(func(p inspect.Pkg, pkgFlag PkgFlag) bool) {
	if c.FilterPkgsFn == nil {
		return nil
	}
	return c.FilterPkgsFn(g)
}

func (c *ControllerFuncs) BeforeCopy(g inspect.Global, session inspect.Session) {
	if c.BeforeCopyFn == nil {
		return
	}
	c.BeforeCopyFn(g, session)
}
func (c *ControllerFuncs) GenOverlay(g inspect.Global, session inspect.Session) map[string]*Content {
	if c.GenOverlayFn == nil {
		return nil
	}
	return c.GenOverlayFn(g, session)
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
func GenRewrite(args []string, rootDir string, ctrl Controller, rewritter inspect.Visitor, opts *BuildRewriteOptions) (res *GenRewriteResult, err error) {
	res = &GenRewriteResult{}
	if opts == nil {
		opts = &BuildRewriteOptions{}
	}
	verbose := opts.Verbose
	verboseCopy := opts.VerboseCopy
	verboseRewrite := opts.VerboseRewrite
	verboseCost := false

	if rootDir == "" {
		panic(fmt.Errorf("rootDir is empty"))
	}
	if opts.Verbose {
		log.Printf("rewrite root: %s", rootDir)
	}
	err = os.MkdirAll(rootDir, 0777)
	if err != nil {
		err = fmt.Errorf("error mkdir %s %v", rootDir, err)
		return
	}

	projectDir := opts.ProjectDir
	projectDir, err = util.ToAbsPath(projectDir)
	if err != nil {
		err = fmt.Errorf("get abs dir err:%v", err)
		return
	}
	ctrl.BeforeLoad(opts)

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
	ctrl.AfterLoad(g)

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
	pkgsFn := ctrl.FilterPkgs(g)
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

	// create a session, and rewrite
	session := inspect.NewSession(g)
	inspect.VisitAll(func(f func(pkg inspect.Pkg) bool) {
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
	var destUpdatedBySource map[string]bool
	doCopy := func() {
		if verbose {
			log.Printf("copying packages files into rewrite dir: total packages=%d", pkgCnt)
		}
		copyTime := time.Now()
		destUpdatedBySource = copyPackageFiles(pkgsFn, rootDir, extraPkgInVendor, hasStd, opts.Force, verboseCopy, verbose)
		copyEnd := time.Now()
		if verboseCost {
			log.Printf("COST copy:%v", copyEnd.Sub(copyTime))
		}
	}
	doCopy()

	// mod replace only work at module-level, so if at least
	// one package inside a module is modified, we need to
	// copy its module out.
	doMod := func() {
		// after copied, modify go.mod with replace absoluted
		if verbose {
			log.Printf("replacing go.mod with rewritten paths")
		}
		goModTime := time.Now()
		res.MappedMod = makeGomodReplaceAboslute(pkgsFn, rootDir, projectDir, verbose)
		goModEnd := time.Now()
		if verboseCost {
			log.Printf("COST go mod:%v", goModEnd.Sub(goModTime))
		}
	}

	// NOTE: only non-vendor needs to replace relative module path
	// with absolute path, because vendored packages are inside
	// vendor
	if !extraPkgInVendor {
		doMod()
	}

	writeContentTime := time.Now()

	ctrl.BeforeCopy(g, session)

	// NOTE: paths in the backMap
	// are absolute, i.e. ${REWRITE_ROOT}/${ORIG_DIR}/file.go
	// so it's possible to put anything in here
	// the upper layer package 'project' will
	// ensure these files are all rooted at ${REWRITE_ROOT},
	// including GOROOT/src rewritted ones.
	backMap := ctrl.GenOverlay(g, session)

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
	filecopy.SyncGenerated(
		func(fn func(path string)) {
			for path := range backMap {
				fn(path)
			}
		},
		func(name string) []byte {
			c, ok := backMap[name]
			if !ok {
				panic(fmt.Errorf("no such file:%v", name))
			}
			return c.Content
		},
		"", // already rooted
		func(filePath, destPath string, destFileInfo os.FileInfo) bool {
			// if ever updated by source, then we always need to update again.
			// NOTE: this only applies to rewritten file,mock file not influenced.
			if destUpdatedBySource[filePath] {
				// log.Printf("DEBUG update by source:%v", filePath)
				return true
			}
			backFile := backMap[filePath].SrcFile
			if backFile == "" {
				return true // should always copy if no back file
			}
			modTime, ferr := filecopy.GetNewestModTime(backFile)
			if ferr != nil {
				panic(ferr)
			}
			return !modTime.IsZero() && modTime.After(destFileInfo.ModTime())
		},
		filecopy.SyncRebaseOptions{
			Force:   opts.Force,
			Ignores: ignores,
			// ProcessDestPath: cleanFsGoPath, // not needed as we already did that
			OnUpdateStats: filecopy.NewLogger(func(format string, args ...interface{}) {
				log.Printf(format, args...)
			}, verboseRewrite, verbose, 200*time.Millisecond),
		},
	)

	writeContentEnd := time.Now()
	if verboseCost {
		log.Printf("COST write content:%v", writeContentEnd.Sub(writeContentTime))
	}

	if verboseCost {
		log.Printf("COST GenRewrite:%v", time.Since(loadPkgTime))
	}
	return
}

var ignores = []string{"(.*/)?\\.git\\b", "(.*/)?node_modules\\b"}

// copyPackageFiles copy starter packages(with all packages under the same module) and extra packages into rootDir, to bundle them together.
func copyPackageFiles(pkgs func(func(p inspect.Pkg, flag PkgFlag) bool), rootDir string, extraPkgInVendor bool, hasStd bool, force bool, verboseDetail bool, verboseOverall bool) (destUpdated map[string]bool) {
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
		Force:           force,
		DeleteNotFound:  true, // uncovered files are deleted
		ProcessDestPath: cleanGoFsPath,
		OnUpdateStats: filecopy.NewLogger(func(format string, args ...interface{}) {
			log.Printf(format, args...)
		}, verboseDetail, verboseOverall, 200*time.Millisecond),
		DidCopy: func(srcPath, destPath string) {
			destUpdatedM.Store(destPath, true)
			atomic.AddInt64(&size, 1)
		},
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
func makeGomodReplaceAboslute(pkgs func(func(pkg inspect.Pkg, flag PkgFlag) bool), rebaseDir string, projectDir string, verbose bool) (mappedMod map[string]string) {
	// if there is vendor/modules.txt, should also replace there
	replaceMap := make(map[string]string)

	goModEditReplace := func(oldpath string, newPath string) string {
		return fmt.Sprintf("go mod edit -replace=%s=%s", sh.Quote(oldpath), sh.Quote(newPath))
	}
	// premap: modPath -> ${rebaseDir}/${modDir}
	preMap := make(map[string]string)
	var preCmdList []string
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
			preCmdList = append(preCmdList, goModEditReplace(modPath, newPath))
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
		gomod, err := util.GetGoMod(dir)
		if err != nil {
			panic(err)
		}

		// replace with absolute paths
		var replaceList []string
		if len(gomod.Replace) > 0 {
			replaceList = make([]string, 0, len(gomod.Replace))
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
				replaceList = append(replaceList, goModEditReplace(oldv, newPath))

				// replace vendor/modules.txt without version will effectively replace all version
				// # PKG => PATH
				// # PKG VERSION => PATH
				replaceMap[rp.Old.Path] = newPath
			}
		}

		if len(replaceList) > 0 || len(preCmdList) > 0 {
			if verbose {
				log.Printf("make absolute replace in go.mod for %v", mod.OrigPath())
			}
			cmds := append([]string{
				fmt.Sprintf("cd %s", sh.Quote(dir)),
			}, replaceList...)
			cmds = append(cmds, preCmdList...)
			err = sh.RunBash(cmds, verbose)
			if err != nil {
				panic(err)
			}
		}
	}

	// because files are already copied, so we can check vendor/modules.txt locally
	vendorErr := appendVendorModulesIgnoreNonExist(path.Join(rebaseDir, projectDir, "vendor/modules.txt"), replaceMap)
	if vendorErr != nil {
		log.Printf("ERROR failed to update vendor/modules.txt: %v\n", vendorErr)
		panic(vendorErr)
	}
	return
}
func appendVendorModulesIgnoreNonExist(path string, replaceMap map[string]string) (err error) {
	if len(replaceMap) == 0 {
		return
	}
	bytes, err := ioutil.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
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

	err = ioutil.WriteFile(path, []byte(strings.Join(lines, "\n")), 0777)
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
