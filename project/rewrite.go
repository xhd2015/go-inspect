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
	BuildFlags []string
}
type RewriteOpts struct {
	BuildOpts *BuildOpts

	RewriteName string // default: code-lens-agent

	// predefined code sets for generated content
	PreCode map[string]string
}

type RewriteResult struct {
	*rewrite.BuildResult
}

func Rewrite(loadArgs []string, opts *RewriteOpts) *RewriteResult {
	var extraCallbacks []Rewriter
	return doRewrite(loadArgs, &RewriteCallbackOpts{
		RewriteOpts: opts,
		RewriteCallback: &RewriteCallback{
			Init: func(proj Project) {
				for _, f := range projectListeners {
					callback := f(proj)
					if callback != nil {
						extraCallbacks = append(extraCallbacks, callback)
					}
				}
				for _, f := range initListeners {
					f(proj)
				}
				for _, callback := range extraCallbacks {
					callback.Init(proj)
				}
			},
			GenOverlay: func(proj Project, session inspect.Session) {
				for _, f := range genOverlayListeners {
					f(proj, session)
				}
				for _, callback := range extraCallbacks {
					callback.GenOverlay(proj, session)
				}
			},
			RewritePackage: func(proj Project, pkg inspect.Pkg, session inspect.Session) {
				for _, f := range rewritePackageListeners {
					f(proj, pkg, session)
				}
				for _, callback := range extraCallbacks {
					callback.RewritePackage(proj, pkg, session)
				}
			},
			RewriteFile: func(proj Project, file inspect.FileContext, session inspect.Session) {
				for _, f := range rewriteFileListeners {
					f(proj, file, session)
				}
				for _, callback := range extraCallbacks {
					callback.RewriteFile(proj, file, session)
				}
			},
			Finish: func(proj Project, err error, result *RewriteResult) {
				for _, f := range finishListeners {
					f(proj, err, result)
				}
				for _, callback := range extraCallbacks {
					callback.Finish(proj, err, result)
				}
			},
		},
	})
}

type RewriteCallbackOpts struct {
	*RewriteOpts
	*RewriteCallback
}

func RewriteNoInterceptors(loadArgs []string, opts *RewriteCallbackOpts) *RewriteResult {
	return doRewrite(loadArgs, opts)
}

// Rewrite always rewrite same module, though it can be
// extended to rewrite other modules
func doRewrite(loadArgs []string, opts *RewriteCallbackOpts) *RewriteResult {
	var proj *project
	var res *RewriteResult
	defer func() {
		if opts != nil && opts.RewriteCallback != nil && opts.RewriteCallback.Finish != nil {
			var e interface{}
			var err error
			if e = recover(); e != nil {
				if a, ok := e.(error); ok {
					err = a
				} else {
					err = fmt.Errorf("%v", e)
				}
			}
			opts.RewriteCallback.Finish(proj, err, res)
			if e != nil {
				// panic out again
				panic(e)
			}
		}
	}()
	proj, res = doRewriteNoCheckPanic(loadArgs, opts)
	return res
}
func doRewriteNoCheckPanic(loadArgs []string, opts *RewriteCallbackOpts) (proj *project, result *RewriteResult) {
	if opts == nil {
		opts = &RewriteCallbackOpts{}
	}
	if opts.RewriteOpts == nil {
		opts.RewriteOpts = &RewriteOpts{}
	}
	if opts.RewriteOpts.BuildOpts == nil {
		opts.RewriteOpts.BuildOpts = &BuildOpts{}
	}
	if opts.RewriteCallback == nil {
		opts.RewriteCallback = &RewriteCallback{}
	}

	buildOpts := opts.RewriteOpts.BuildOpts
	rewriteName := opts.RewriteName
	if rewriteName == "" {
		rewriteName = "code-lens-agent"
	}
	rewriteRoot := rewrite.GetTmpRewriteRoot(rewriteName)

	projectAbsDir, err := util.ToAbsPath(buildOpts.ProjectDir)
	if err != nil {
		panic(fmt.Errorf("get abs dir err:%v", err))
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

	initPkgAnalyse := func(g inspect.Global) {
		if opts.Init == nil {
			return
		}

		// ensure mainPkg0
		mainPkg0 = getMainPkg0(g)
		proj = &project{
			g:                  g,
			mainPkg:            mainPkg0,
			opts:               opts.RewriteOpts,
			args:               loadArgs,
			rewriteRoot:        rewriteRoot,
			rewriteProjectRoot: projectRewriteRoot,
			projectRoot:        projectAbsDir,
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
		// the granliarity is at package and file level
		// detailed nodes are not touched
		VisitFn: func(n ast.Node, session inspect.Session) bool {
			if opts.RewritePackage != nil {
				if pkg, ok := n.(*ast.Package); ok {
					p := session.Global().Registry().Pkg(pkg)
					opts.RewritePackage(proj, p, session)
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

		ForTest:    buildOpts.ForTest,
		GoFlags:    buildOpts.GoFlags,
		BuildFlags: buildOpts.BuildFlags,
	})
	if err != nil {
		panic(err)
	}

	fmt.Printf("build %s successful.\n", res.Output)
	result = &RewriteResult{
		BuildResult: res,
	}
	return
}
