package project

import (
	"fmt"
	"go/ast"
	"path"

	"github.com/xhd2015/go-inspect/inspect"
	"github.com/xhd2015/go-inspect/inspect/util"
	"github.com/xhd2015/go-inspect/rewrite"
)

type BuildOpts struct {
	ProjectDir string
	Verbose    bool
	Force      bool
	Debug      bool
	Output     string
	ForTest    bool
	GoFlags    []string // passed to go build
}

type RewriteOpts struct {
	BuildOpts *BuildOpts

	RewriteName string // default: code-lens-agent

	// predefined code sets for generated content
	PreCode map[string]string

	Init           func(proj Project)
	GenOverlay     func(proj Project, session inspect.Session)
	RewritePackage func(proj Project, pkg inspect.Pkg, session inspect.Session) bool
	RewriteFile    func(proj Project, f inspect.FileContext, session inspect.Session)
}

// Rewrite always rewrite same module, though it can be
// extended to rewrite other modules
func Rewrite(loadArgs []string, opts *RewriteOpts) {
	if opts == nil {
		opts = &RewriteOpts{}
	}

	buildOpts := opts.BuildOpts
	if buildOpts == nil {
		buildOpts = &BuildOpts{}
	}
	rewriteName := opts.RewriteName
	if rewriteName == "" {
		rewriteName = "code-lens-agent"
	}
	rewriteRoot := rewrite.GetTmpRewriteRoot(rewriteName)

	projectAbsDir, err := util.ToAbsPath(buildOpts.ProjectDir)
	if err != nil {
		err = fmt.Errorf("get abs dir err:%v", err)
		return
	}
	projectRewriteRoot := path.Join(rewriteRoot, projectAbsDir)

	genMap := make(map[string]*rewrite.Content)

	var mainPkg0 inspect.Pkg

	getMainPkg0 := func(g inspect.Global) inspect.Pkg {
		pkgs := g.LoadInfo().StarterPkgs()
		if len(pkgs) == 0 {
			panic(fmt.Errorf("no packages"))
		}
		return pkgs[0]
	}
	var proj *project

	initPkgAnalyse := func(g inspect.Global) {
		if opts.Init == nil {
			return
		}

		// ensure mainPkg0
		mainPkg0 = getMainPkg0(g)
		proj = &project{
			g:                  g,
			mainPkg:            mainPkg0,
			rewriteRoot:        rewriteRoot,
			rewriteProjectRoot: projectRewriteRoot,
			genMap:             genMap,
		}
		opts.Init(proj)
	}

	var init bool
	ctrl := &rewrite.ControllerFuncs{
		// TODO: add a explicit init function
		// called first
		FilterPkgsFn: func(g inspect.Global) func(func(p inspect.Pkg, pkgFlag rewrite.PkgFlag) bool) {
			if !init {
				init = true
				initPkgAnalyse(g)
			}

			mod := g.LoadInfo().MainModule()
			return func(f func(p inspect.Pkg, pkgFlag rewrite.PkgFlag) bool) {
				g.RangePkg(func(pkg inspect.Pkg) bool {
					// rewrite for the same module
					if pkg.Module() == mod {
						f(pkg, rewrite.BitStarterMod)
					}
					return true
				})
			}
		},
		GenOverlayFn: func(g inspect.Global, session inspect.Session) map[string]*rewrite.Content {
			if opts.GenOverlay != nil {
				opts.GenOverlay(proj, session)
			}

			// template code
			for file, code := range opts.PreCode {
				genMap[rewrite.CleanGoFsPath(path.Join(rewriteRoot, file))] = &rewrite.Content{
					Content: []byte(code),
				}
			}

			session.Gen(&inspect.EditCallbackFn{
				Rewrites: func(f inspect.FileContext, content string) bool {
					newPath := rewrite.CleanGoFsPath(path.Join(rewriteRoot, f.AbsPath()))
					genMap[newPath] = &rewrite.Content{
						SrcFile: f.AbsPath(),
						Content: []byte(content),
					}
					return true
				},
				Pkg: func(p inspect.Pkg, kind, realName, content string) bool {
					newPath := rewrite.CleanGoFsPath(path.Join(rewriteRoot, p.Dir(), realName+".go"))
					genMap[newPath] = &rewrite.Content{
						Content: []byte(content),
					}
					return true
				},
			})
			return genMap
		},
	}
	vis := &inspect.Visitors{
		VisitFn: func(n ast.Node, session inspect.Session) bool {
			if opts.RewritePackage != nil {
				if pkg, ok := n.(*ast.Package); ok {
					p := session.Global().Registry().Pkg(pkg)
					opts.RewritePackage(proj, p, session)
					return false
				}
			}
			if opts.RewriteFile != nil {
				if file, ok := n.(*ast.File); ok {
					f := session.Global().Registry().File(file)
					opts.RewriteFile(proj, f, session)
					return false
				}
			}
			return true
		},
	}

	// assume vendor mode
	res, err := rewrite.BuildRewrite(loadArgs, ctrl, vis, &rewrite.BuildRewriteOptions{
		Verbose:    buildOpts.Verbose,
		ProjectDir: buildOpts.ProjectDir,
		RebaseRoot: rewriteRoot,
		Force:      buildOpts.Force,

		Debug:  buildOpts.Debug,
		Output: buildOpts.Output,

		ForTest: buildOpts.ForTest,
		GoFlags: buildOpts.GoFlags,
	})
	if err != nil {
		panic(err)
	}

	fmt.Printf("build %s successful.\n", res.Output)
}
