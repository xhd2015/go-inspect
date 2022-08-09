package cmdsupport

import (
	"fmt"
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
	api "github.com/xhd2015/go-inspect/inspect2"
	"github.com/xhd2015/go-inspect/sh"
)

type GenRewriteOptions struct {
	Verbose        bool
	VerboseCopy    bool
	VerboseRewrite bool
	// VerboseGomod   bool
	ProjectDir     string // the project dir
	RewriteOptions *inspect.RewriteOptions
	StubGenDir     string // relative path, default test/mock_gen
	SkipGenMock    bool

	PkgFilterOptions

	Force bool // force indicates no cache

	LoadOptions
}

type PkgFilterOptions struct {
	OnlyPackages map[string]bool
	Packages     map[string]bool
	Modules      map[string]bool
	AllowMissing bool
}

type GenRewriteResult struct {
	// original dir to new dir, original dir may contain @version, new dir replace @ with /
	// to be used as -trim when building
	MappedMod    map[string]string
	UseNewGOROOT string
}

var ignores = []string{"(.*/)?\\.git\\b", "(.*/)?node_modules\\b"}

/*
COST stats of serial process:
2022/07/02 21:56:38 COST load packages:4.522980981s
2022/07/02 21:56:38 COST load package -> filter package:131.63µs
2022/07/02 21:56:38 COST filter package:1.875944ms
2022/07/02 21:56:38 COST rewrite:229.670816ms
2022/07/02 21:56:48 COST copy:9.850795028s
2022/07/02 21:56:48 COST go mod:152.905388ms
2022/07/02 21:56:50 COST write content:2.157077768s
2022/07/02 21:56:50 COST GenRewrite:16.937557165s

shows that copy is the most time consuming point.
*/

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

type Rewritter interface {
	FilterPkgs(g api.Global) func(func(p api.Pkg, pkgFlag PkgFlag) bool)
	BeforeCopy(g api.Global, session api.Session)
	GenOverlay(g api.Global, session api.Session) map[string]*Content
}

type RewritterFuncs struct {
	FilterPkgsFn func(g api.Global) func(func(p api.Pkg, pkgFlag PkgFlag) bool)
	BeforeCopyFn func(g api.Global, session api.Session)
	GenOverlayFn func(g api.Global, session api.Session) map[string]*Content
}

func (c *RewritterFuncs) FilterPkgs(g api.Global) func(func(p api.Pkg, pkgFlag PkgFlag) bool) {
	if c.FilterPkgsFn == nil {
		return nil
	}
	return c.FilterPkgsFn(g)
}

func (c *RewritterFuncs) BeforeCopy(g api.Global, session api.Session) {
	if c.BeforeCopyFn == nil {
		return
	}
	c.BeforeCopyFn(g, session)
}
func (c *RewritterFuncs) GenOverlay(g api.Global, session api.Session) map[string]*Content {
	if c.GenOverlayFn == nil {
		return nil
	}
	return c.GenOverlayFn(g, session)
}

func filterPackage(g api.Global, verbose bool, opts *PkgFilterOptions) func(func(p api.Pkg, pkgFlag PkgFlag) bool) {
	if opts == nil {
		opts = &PkgFilterOptions{}
	}
	verboseCost := false

	// init rewrite opts
	onlyPkgs := opts.OnlyPackages
	wantsExtraPkgs := opts.Packages
	wantsExtrPkgsByMod := opts.Modules
	allowMissing := opts.AllowMissing

	var needPkg func(pkgPath string) bool
	var needMod func(modPath string) bool
	if len(onlyPkgs) == 0 {
		if len(wantsExtraPkgs) > 0 {
			needPkg = func(pkgPath string) bool {
				return wantsExtraPkgs[pkgPath]
			}
		}
		if len(wantsExtrPkgsByMod) > 0 {
			needMod = func(modPath string) bool {
				return wantsExtrPkgsByMod[modPath]
			}
		}
	} else {
		needPkg = func(pkgPath string) bool {
			return onlyPkgs[pkgPath]
		}
	}

	filterPkg := func(needPkg func(pkgPath string) bool, needMod func(modPath string) bool) (modPkgs []api.Pkg, allPkgs []api.Pkg, extraPkgs []api.Pkg) {
		if len(onlyPkgs) == 0 {
			modPkgs, extraPkgs = inspect.GetSameModulePackagesAndPkgsGiven(g.LoadInfo(), needPkg, needMod)
		} else {
			var oldModPkgs []api.Pkg
			oldModPkgs, extraPkgs = inspect.GetSameModulePackagesAndPkgsGiven(g.LoadInfo(), needPkg, needMod)
			for _, p := range oldModPkgs {
				if onlyPkgs[p.Path()] {
					modPkgs = append(modPkgs, p)
				}
			}
		}

		allPkgs = make([]api.Pkg, 0, len(modPkgs)+len(extraPkgs))
		allPkgs = append(allPkgs, modPkgs...)
		for _, p := range extraPkgs {
			if len(p.GoPkg().GoFiles) == 0 {
				continue
			}
			allPkgs = append(allPkgs, p)
		}
		return
	}
	filterPkgStart := time.Now()
	modPkgs, allPkgs, extraPkgs := filterPkg(needPkg, needMod)
	if verboseCost {
		log.Printf("COST filter package:%v", time.Since(filterPkgStart))
	}

	// make pkg map
	pkgMap := make(map[string]api.Pkg, len(allPkgs))
	for _, pkg := range allPkgs {
		pkgMap[pkg.Path()] = pkg
	}

	if verbose {
		log.Printf("found %d packages", len(allPkgs))
	}

	// check if wanted pkgs are all found
	var missingExtra []string
	for extraPkg := range wantsExtraPkgs {
		if pkgMap[extraPkg] == nil {
			missingExtra = append(missingExtra, extraPkg)
		}
	}
	if len(missingExtra) > 0 {
		if !allowMissing {
			panic(fmt.Errorf("packages not found:%v", missingExtra))
		}
		log.Printf("WARNING: not found packages will be skipped:%v", missingExtra)
	}

	return func(f func(p api.Pkg, pkgFlag PkgFlag) bool) {
		for _, p := range modPkgs {
			if !f(p, BitStarterMod) {
				return
			}
		}
		for _, p := range extraPkgs {
			if !f(p, BitExtra) {
				return
			}
		}
	}
}
func genFileMap(g api.Global, session api.Session, rootDir string, opts *GenRewriteOptions) map[string]*Content {
	contents := make(map[string]*inspect.ContentError)
	session.Gen(&api.EditCallbackFn{
		Rewrites: func(f api.FileContext, content string) bool {
			pcontent := contents[f.Pkg().Path()]
			if pcontent == nil {
				pcontent = &inspect.ContentError{
					PkgPath: f.Pkg().Path(),
					Files:   make(map[string]*inspect.FileContentError),
				}
				contents[f.Pkg().Path()] = pcontent
			}
			pcontent.Files[f.AbsPath()] = &inspect.FileContentError{
				OrigFile: f.AbsPath(),
				Content:  content,
			}
			return true
		},
		Pkg: func(p api.Pkg, kind, realName, content string) bool {
			if kind == "mock_stub" {
				if contents[p.Path()] == nil {
					contents[p.Path()] = &inspect.ContentError{}
				}
				contents[p.Path()].MockContent = content
			}
			return true
		},
	})

	destFsPath := func(origFsPath string) string {
		return path.Join(rootDir, origFsPath)
	}

	mainModPath := g.LoadInfo().MainModule().Path()
	// return relative directory
	stubFsRelDir := func(pkgModPath, pkgPath string) string {
		rel := ""
		if pkgModPath == mainModPath {
			rel = inspect.GetRelativePath(pkgModPath, pkgPath)
		} else {
			rel = path.Join("ext", pkgPath)
		}
		return rel
	}

	projectDir := g.LoadInfo().Root()

	verbose := opts.Verbose
	verboseRewrite := opts.VerboseRewrite
	skipGenMock := opts.SkipGenMock
	needGenMock := !skipGenMock  // gen mock inside test/mock_stub
	needMockRegistering := false // gen mock registering info,for debug info. TODO: may use another option
	needAnyMockStub := needGenMock || needMockRegistering

	stubGenDir := opts.StubGenDir
	stubInitEntryDir := "" // ${stubGenDir}/${xxxName}_init
	if needAnyMockStub && stubGenDir == "" {
		if needGenMock {
			stubGenDir = "test/mock_gen"
		} else {
			// try mock_gen, mock_gen1,...
			stubGenDir = util.NextFileNameUnderDir(projectDir, "mock_gen", "")
			stubInitEntryDir = stubGenDir + "/" + stubGenDir + "_init"
		}
	}
	if needAnyMockStub && stubInitEntryDir == "" {
		// try mock_gen_init, mock_gen_init1,...
		genName := util.NextFileNameUnderDir(projectDir, "mock_gen_init", "")
		stubInitEntryDir = stubGenDir + "/" + genName
	}

	stubRelGenDir := ""
	// make absolute
	if !path.IsAbs(stubGenDir) {
		stubRelGenDir = stubGenDir
		stubGenDir = path.Join(projectDir, stubGenDir)
	}

	// overwrite new content
	nrewriteFile := 0
	nmock := 0

	var mockPkgList []string
	backMap := make(map[string]*Content)
	for _, pkgRes := range contents {
		pkgPath := pkgRes.PkgPath

		pkg := g.GetPkg(pkgPath)
		if pkg == nil {
			panic(fmt.Errorf("pkg not found:%v", pkgPath))
		}

		// generate rewritting files
		for _, fileRes := range pkgRes.Files {
			if fileRes.OrigFile == "" {
				panic(fmt.Errorf("orig file not found:%v", pkgPath))
			}
			if pkgRes.MockContentError != nil {
				continue
			}
			nrewriteFile++
			backMap[cleanGoFsPath(destFsPath(fileRes.OrigFile))] = &Content{
				SrcFile: fileRes.OrigFile,
				Content: []byte(fileRes.Content),
			}
		}
		// generate mock stubs
		if needAnyMockStub && pkgRes.MockContentError == nil && pkgRes.MockContent != "" {
			// relative to current module
			rel := stubFsRelDir(pkg.Module().Path(), pkgPath)
			genDir := path.Join(stubGenDir, rel)
			genFile := path.Join(genDir, "mock.go")
			if verboseRewrite {
				log.Printf("generate mock file %s", genFile)
			}

			pkgDir := pkg.Dir()

			mockContent := []byte(pkgRes.MockContent)

			if needGenMock {
				backMap[genFile] = &Content{
					SrcFile: pkgDir,
					Content: mockContent,
				}
			}

			// TODO: may skip this for 'go test'
			if needMockRegistering {
				genRewriteFile := destFsPath(genFile)
				backMap[genRewriteFile] = &Content{
					SrcFile: pkgDir,
					Content: mockContent,
				}
				rdir := ""
				if stubGenDir != "" {
					rdir = "/" + strings.TrimPrefix(stubRelGenDir, "/")
				}
				mockPkgList = append(mockPkgList, pkg.Module().Path()+rdir+"/"+rel)
				nmock++
			}
		}
	}
	if verbose {
		log.Printf("rewritten files:%d, generate mock files:%d", nrewriteFile, nmock)
	}
	return backMap
}
func GenRewrite(args []string, rootDir string, opts *GenRewriteOptions) (res *GenRewriteResult) {
	if true {
		// My test
		return GenRewriteInit(args, rootDir, opts)
	}
	if opts == nil {
		opts = &GenRewriteOptions{}
	}

	rw := &RewritterFuncs{
		FilterPkgsFn: func(g api.Global) func(func(p api.Pkg, pkgFlag PkgFlag) bool) {
			return filterPackage(g, opts.Verbose, &opts.PkgFilterOptions)
		},
		GenOverlayFn: func(g api.Global, session api.Session) map[string]*Content {
			return genFileMap(g, session, rootDir, opts)
		},
		BeforeCopyFn: func(g api.Global, session api.Session) {
			// 1: create build info
			{
				// create a mock_build_info.go aside with original project files,
				// to register build infos
				pkg0 := g.LoadInfo().StarterPkgs()[0]
				newEdit := session.PackageEdit(pkg0, "mock_build_info")
				newEdit.SetPackageName(pkg0.Name())

				newEdit.AddCode(fmt.Sprintf("package %s\n\nimport _mock %q\nfunc init(){\n    _mock.SetBuildInfo(&_mock.BuildInfo{MainModule: %q})\n}", pkg0.Name(), inspect.MOCK_PKG, pkg0.Module().OrigPath()))
			}

			if false /*needMockRegistering: need mock init*/ {

				// addMockRegisterContent := func(stubInitEntryDir string, mockPkgList []string) {
				// 	// an entry init.go to import all registering types
				// 	stubGenCode := genImportListContent(stubInitEntryDir, mockPkgList)
				// 	backMap[destFsPath(path.Join(modDir, stubInitEntryDir, "init.go"))] = &Content{
				// 		bytes: []byte(stubGenCode),
				// 	}

				// 	// create a mock_init.go aside with original project files, to import the entry file above
				// 	starterName := util.NextFileNameUnderDir(starterPkg0Dir, "mock_init", ".go")
				// 	backMap[destFsPath(path.Join(starterPkg0Dir, starterName))] = &Content{
				// 		bytes: []byte(fmt.Sprintf("package %s\nimport _ %q", starterPkg0.Name, modPath+"/"+stubInitEntryDir)),
				// 	}
				// }
				// addMockRegisterContent(stubInitEntryDir, mockPkgList)
			}
		},
	}

	pkgRw := inspect.NewMockRewritter(opts.RewriteOptions)
	return GenRewriteV2(args, rootDir, rw, pkgRw, opts)
}

func GenRewriteInit(args []string, rootDir string, opts *GenRewriteOptions) (res *GenRewriteResult) {
	if opts == nil {
		opts = &GenRewriteOptions{}
	}

	rw := &RewritterFuncs{
		FilterPkgsFn: func(g api.Global) func(func(p api.Pkg, pkgFlag PkgFlag) bool) {
			mainMod := g.LoadInfo().MainModule()
			return func(f func(p api.Pkg, pkgFlag PkgFlag) bool) {
				g.LoadInfo().RangePkgs(func(p api.Pkg) bool {
					var flag PkgFlag
					if p.Module() == mainMod {
						flag |= BitStarterMod
					} else {
						flag |= BitExtra
					}
					return f(p, flag)
				})
			}
		},
		GenOverlayFn: func(g api.Global, session api.Session) map[string]*Content {
			m := make(map[string]*Content)
			destFsPath := func(origFsPath string) string {
				return path.Join(rootDir, origFsPath)
			}
			session.Gen(&api.EditCallbackFn{
				Pkg: func(p api.Pkg, kind, realName, content string) bool {
					absFile := path.Join(p.Dir(), realName+".go")
					m[cleanGoFsPath(destFsPath(absFile))] = &Content{
						SrcFile: p.Dir(),
						Content: []byte(content),
					}
					return true
				},
			})
			return m
		},
	}

	pkgRw := inspect.NewInitRewritter()
	return GenRewriteV2(args, rootDir, rw, pkgRw, opts)
}

// TODO: vendor mod should behave differently
func GenRewriteV2(args []string, rootDir string, rw Rewritter, pkgRw api.Visitor, opts *GenRewriteOptions) (res *GenRewriteResult) {
	res = &GenRewriteResult{}
	if opts == nil {
		opts = &GenRewriteOptions{}
	}
	verbose := opts.Verbose
	verboseCopy := opts.VerboseCopy
	verboseRewrite := opts.VerboseRewrite
	verboseCost := false
	force := opts.Force

	if rootDir == "" {
		panic(fmt.Errorf("rootDir is empty"))
	}
	if verbose {
		log.Printf("rewrite root: %s", rootDir)
	}
	err := os.MkdirAll(rootDir, 0777)
	if err != nil {
		panic(fmt.Errorf("error mkdir %s %v", rootDir, err))
	}

	projectDir := opts.ProjectDir
	projectDir, err = util.ToAbsPath(projectDir)
	if err != nil {
		panic(fmt.Errorf("get abs dir err:%v", err))
	}

	loadPkgTime := time.Now()

	g, err := load.LoadPackages(args, &load.LoadOptions{
		ProjectDir: projectDir,
		ForTest:    opts.ForTest,
		BuildFlags: opts.LoadArgs,
	})
	if err != nil {
		panic(err)
	}

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
		panic(fmt.Errorf("no packages loaded."))
	}

	// filter pkgs
	pkgsFn := rw.FilterPkgs(g)
	if pkgsFn == nil {
		// choose starterPkgs
		pkgsFn = func(f func(p api.Pkg, pkgFlag PkgFlag) bool) {
			for _, p := range g.LoadInfo().StarterPkgs() {
				if !f(p, BitStarterMod|BitStarter) {
					return
				}
			}
		}
	}

	rewriteOpts := opts.RewriteOptions
	if rewriteOpts == nil {
		rewriteOpts = &inspect.RewriteOptions{}
	}

	// expand to all packages under the same module that depended by starter packages
	// rewrite
	rewriteTime := time.Now()
	if verboseCost {
		log.Printf("COST load package -> rewrite package:%v", rewriteTime.Sub(loadPkgEnd))
	}

	// create a session, and rewrite
	session := api.NewSession(g)
	api.VisitAll(func(f func(pkg api.Pkg) bool) {
		pkgsFn(func(p api.Pkg, pkgFlag PkgFlag) bool {
			return f(p)
		})
	}, session, pkgRw)
	rewriteEnd := time.Now()
	if verboseCost {
		log.Printf("COST rewrite:%v", rewriteEnd.Sub(rewriteTime))
	}

	// TODO: move vendor detection to Global
	extraPkgInInVendor := false
	hasStd := false
	hasExtra := false

	pkgCnt := 0
	pkgsFn(func(p api.Pkg, pkgFlag PkgFlag) bool {
		pkgCnt++
		hasStd = hasStd || p.Module().IsStd()
		if !pkgFlag.IsExtra() {
			return true
		}
		hasExtra = true
		// checking vendor
		if !extraPkgInInVendor {
			dir := p.Module().Dir()
			if dir == "" {
				// has module, but no dir
				// check if any file is inside vendor
				if util.IsVendor(modDir, p.GoPkg().GoFiles[0]) /*empty GoFiles are filtered*/ {
					extraPkgInInVendor = true
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
			log.Printf("extra packages in vendor:%v", extraPkgInInVendor)
		}
	}

	// copy files
	var destUpdatedBySource map[string]bool
	doCopy := func() {
		if verbose {
			log.Printf("copying packages files into rewrite dir: total packages=%d", pkgCnt)
		}
		copyTime := time.Now()
		destUpdatedBySource = copyPackageFiles(pkgsFn, rootDir, extraPkgInInVendor, hasStd, force, verboseCopy, verbose)
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
		res.MappedMod = makeGomodReplaceAboslute(pkgsFn, rootDir, verbose)
		goModEnd := time.Now()
		if verboseCost {
			log.Printf("COST go mod:%v", goModEnd.Sub(goModTime))
		}
	}
	if !extraPkgInInVendor {
		doMod()
	}

	writeContentTime := time.Now()

	rw.BeforeCopy(g, session)
	backMap := rw.GenOverlay(g, session)

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
			Force:   force,
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

// copyPackageFiles copy starter packages(with all packages under the same module) and extra packages into rootDir, to bundle them together.
func copyPackageFiles(pkgs func(func(p api.Pkg, flag PkgFlag) bool), rootDir string, extraPkgInVendor bool, hasStd bool, force bool, verboseDetail bool, verboseOverall bool) (destUpdated map[string]bool) {
	var dirList []string
	fileIgnores := append([]string(nil), ignores...)

	// in test mode, go loads 3 types package under the same dir:
	// 1.normal package
	// 2.bridge package, which contains module
	// 3.test package, which does not contain module

	// copy go files
	moduleDirs := make(map[string]bool)
	pkgs(func(p api.Pkg, flag PkgFlag) bool {
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
func makeGomodReplaceAboslute(pkgs func(func(pkg api.Pkg, flag PkgFlag) bool), rebaseDir string, verbose bool) (mappedMod map[string]string) {
	goModEditReplace := func(oldpath string, newPath string) string {
		return fmt.Sprintf("go mod edit -replace=%s=%s", Quote(oldpath), Quote(newPath))
	}
	// premap: modPath -> ${rebaseDir}/${modDir}
	preMap := make(map[string]string)
	var preCmdList []string
	mappedMod = make(map[string]string)

	// get modules(for mods, actually only 1 module, i.e. the current module will be processed)
	mods := make([]api.Module, 0, 1)
	modMap := make(map[string]bool, 1)
	pkgs(func(p api.Pkg, flag PkgFlag) bool {
		mod := p.Module()
		if mod == nil {
			if flag.IsExtra() {
				panic(fmt.Errorf("cannot replace non-module package:%v", p.Path()))
			}
			return true
		}
		// extra pkg
		if flag.IsExtra() {
			if mod.IsStd() {
				// std modules are replaced via golabl env: GOROOT=xxx
				return true
			}
			if preMap[mod.Path()] != "" {
				return true
			}
			cleanDir := cleanGoFsPath(mod.Dir())
			newPath := path.Join(rebaseDir, cleanDir)
			preMap[mod.Path()] = newPath
			preCmdList = append(preCmdList, goModEditReplace(mod.Path(), newPath))

			mappedMod[mod.Dir()] = cleanDir
			return true
		}

		// normal pkg
		if modMap[mod.Path()] {
			return true
		}

		modMap[mod.Path()] = true
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
		gomod, err := inspect.GetGoMod(dir)
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
				replaceList = append(replaceList, goModEditReplace(oldv, path.Join(origDir, rp.New.Path)))
			}
		}

		if len(replaceList) > 0 || len(preCmdList) > 0 {
			if verbose {
				log.Printf("make absolute replace in go.mod for %v", mod.Path())
			}
			cmds := append([]string{
				fmt.Sprintf("cd %s", Quote(dir)),
			}, replaceList...)
			cmds = append(cmds, preCmdList...)
			err = sh.RunBash(cmds, verbose)
			if err != nil {
				panic(err)
			}
		}
	}
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
