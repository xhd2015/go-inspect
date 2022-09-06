package cmd

import (
	"fmt"
	"log"
	"path"
	"strings"
	"time"

	"github.com/xhd2015/go-inspect/inspect"
	"github.com/xhd2015/go-inspect/inspect/util"
	inspect_old "github.com/xhd2015/go-inspect/inspect_old"
	"github.com/xhd2015/go-inspect/rewrite"
)

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

func filterPackage(g inspect.Global, verbose bool, opts *PkgFilterOptions) func(func(p inspect.Pkg, pkgFlag PkgFlag) bool) {
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

	filterPkg := func(needPkg func(pkgPath string) bool, needMod func(modPath string) bool) (modPkgs []inspect.Pkg, allPkgs []inspect.Pkg, extraPkgs []inspect.Pkg) {
		if len(onlyPkgs) == 0 {
			modPkgs, extraPkgs = inspect_old.GetSameModulePackagesAndPkgsGiven(g.LoadInfo(), needPkg, needMod)
		} else {
			var oldModPkgs []inspect.Pkg
			oldModPkgs, extraPkgs = inspect_old.GetSameModulePackagesAndPkgsGiven(g.LoadInfo(), needPkg, needMod)
			for _, p := range oldModPkgs {
				if onlyPkgs[p.Path()] {
					modPkgs = append(modPkgs, p)
				}
			}
		}

		allPkgs = make([]inspect.Pkg, 0, len(modPkgs)+len(extraPkgs))
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
	pkgMap := make(map[string]inspect.Pkg, len(allPkgs))
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

	return func(f func(p inspect.Pkg, pkgFlag PkgFlag) bool) {
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
func genFileMap(g inspect.Global, session inspect.Session, rootDir string, opts *GenRewriteOptions) map[string]*Content {
	contents := make(map[string]*inspect_old.ContentError)
	session.Gen(&inspect.EditCallbackFn{
		Rewrites: func(f inspect.FileContext, content string) bool {
			pcontent := contents[f.Pkg().Path()]
			if pcontent == nil {
				pcontent = &inspect_old.ContentError{
					PkgPath: f.Pkg().Path(),
					Files:   make(map[string]*inspect_old.FileContentError),
				}
				contents[f.Pkg().Path()] = pcontent
			}
			pcontent.Files[f.AbsPath()] = &inspect_old.FileContentError{
				OrigFile: f.AbsPath(),
				Content:  content,
			}
			return true
		},
		Pkg: func(p inspect.Pkg, kind, realName, content string) bool {
			if kind == "mock_stub" {
				if contents[p.Path()] == nil {
					contents[p.Path()] = &inspect_old.ContentError{}
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
			rel = inspect_old.GetRelativePath(pkgModPath, pkgPath)
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
			backMap[rewrite.CleanGoFsPath(destFsPath(fileRes.OrigFile))] = &rewrite.Content{
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
func GenRewrite(args []string, rootDir string, opts *GenRewriteOptions) (res *GenRewriteResult, err error) {
	if true {
		// My test
		return GenRewriteInit(args, rootDir, opts)
	}
	if opts == nil {
		opts = &GenRewriteOptions{}
	}

	ctrl := &rewrite.ControllerFuncs{
		FilterPkgsFn: func(g inspect.Global) func(func(p inspect.Pkg, pkgFlag PkgFlag) bool) {
			return filterPackage(g, opts.Verbose, &opts.PkgFilterOptions)
		},
		GenOverlayFn: func(g inspect.Global, session inspect.Session) map[string]*Content {
			return genFileMap(g, session, rootDir, opts)
		},
		BeforeCopyFn: func(g inspect.Global, session inspect.Session) {
			// 1: create build info
			{
				// create a mock_build_info.go aside with original project files,
				// to register build infos
				pkg0 := g.LoadInfo().StarterPkgs()[0]
				newEdit := session.PackageEdit(pkg0, "mock_build_info")
				newEdit.SetPackageName(pkg0.Name())

				newEdit.AddCode(fmt.Sprintf("package %s\n\nimport _mock %q\nfunc init(){\n    _mock.SetBuildInfo(&_mock.BuildInfo{MainModule: %q})\n}", pkg0.Name(), inspect_old.MOCK_PKG, pkg0.Module().OrigPath()))
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

	rewritter := inspect_old.NewMockRewritter(opts.RewriteOptions)
	return rewrite.GenRewrite(args, rootDir, ctrl, rewritter, opts)
}

func GenRewriteInit(args []string, rootDir string, opts *GenRewriteOptions) (res *GenRewriteResult, err error) {
	if opts == nil {
		opts = &GenRewriteOptions{}
	}

	ctrl := &rewrite.ControllerFuncs{
		FilterPkgsFn: func(g inspect.Global) func(func(p inspect.Pkg, pkgFlag PkgFlag) bool) {
			mainMod := g.LoadInfo().MainModule()
			return func(f func(p inspect.Pkg, pkgFlag PkgFlag) bool) {
				g.LoadInfo().RangePkgs(func(p inspect.Pkg) bool {
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
		GenOverlayFn: func(g inspect.Global, session inspect.Session) map[string]*Content {
			m := make(map[string]*Content)
			destFsPath := func(origFsPath string) string {
				return path.Join(rootDir, origFsPath)
			}
			session.Gen(&inspect.EditCallbackFn{
				Pkg: func(p inspect.Pkg, kind, realName, content string) bool {
					absFile := path.Join(p.Dir(), realName+".go")
					m[rewrite.CleanGoFsPath(destFsPath(absFile))] = &Content{
						SrcFile: p.Dir(),
						Content: []byte(content),
					}
					return true
				},
			})
			return m
		},
	}

	rewritter := inspect_old.NewInitRewritter()
	return rewrite.GenRewrite(args, rootDir, ctrl, rewritter, opts)
}
