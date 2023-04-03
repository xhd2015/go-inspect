package project

import (
	"fmt"
	"go/ast"
	"os"
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
	GoFlags    []string // passed to go load
	BuildFlags []string // passed to go build
}
type RewriteOpts struct {
	BuildOpts *BuildOpts

	RewriteRoot string // default: a randomly made temp dir
	RewriteName string // default: code-lens-agent

	// ShouldRewritePackage an extra filter to include other packages
	ShouldRewritePackage func(pkg inspect.Pkg) bool

	// predefined code sets for generated content
	PreCode map[string]string

	SkipBuild bool
}

type RewriteResult struct {
	*rewrite.BuildResult
}

func Rewrite(loadArgs []string, opts *RewriteOpts) *RewriteResult {
	var extraCallbacks []Rewriter
	return doRewrite(loadArgs, &RewriteCallbackOpts{
		RewriteOpts: opts,
		RewriteCallback: &RewriteCallback{
			BeforeLoad: func(proj Project) {
				for _, f := range projectListeners {
					callback := f(proj)
					if callback != nil {
						extraCallbacks = append(extraCallbacks, callback)
					}
				}
				for _, f := range beforeLoadListeners {
					f(proj)
				}
				for _, callback := range extraCallbacks {
					callback.BeforeLoad(proj)
				}
			},
			AfterLoad: func(proj Project) {
				for _, f := range afterLoadListeners {
					f(proj)
				}
				for _, callback := range extraCallbacks {
					callback.AfterLoad(proj)
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
		rewriteName = "go-inspect"
	}
	rewriteBase := opts.RewriteOpts.RewriteRoot
	if rewriteBase == "" {
		rewriteBase = os.TempDir()
	}
	rewriteRoot := rewrite.GetRewriteRoot(rewriteBase, rewriteName)

	projectAbsDir, err := util.ToAbsPath(buildOpts.ProjectDir)
	if err != nil {
		panic(fmt.Errorf("get abs dir err:%v", err))
	}
	projectRewriteRoot := path.Join(rewriteRoot, projectAbsDir)
	genMap := make(map[string]*rewrite.Content)

	ctrl := &rewrite.ControllerFuncs{
		BeforeLoadFn: func(rwOpts *rewrite.BuildRewriteOptions) {
			proj = &project{
				opts: &options{
					opts:           opts.RewriteOpts,
					underlyingOpts: rwOpts,
				},
				args:               loadArgs,
				rewriteRoot:        rewriteRoot,
				rewriteProjectRoot: projectRewriteRoot,
				projectRoot:        projectAbsDir,
				genMap:             genMap,
				ctxData:            make(map[interface{}]interface{}),
			}
			opts.BeforeLoad(proj)
		},
		AfterLoadFn: func(g inspect.Global) {
			proj.g = g

			// find the first package, define that as main
			// packages
			pkgs := g.LoadInfo().StarterPkgs()
			if len(pkgs) == 0 {
				panic(fmt.Errorf("no packages"))
			}
			proj.mainPkg = pkgs[0]
			opts.AfterLoad(proj)
		},
		// TODO: add a explicit init function
		// called first
		FilterPkgsFn: func(g inspect.Global) func(func(p inspect.Pkg, pkgFlag rewrite.PkgFlag) bool) {
			mod := g.LoadInfo().MainModule()
			return func(f func(p inspect.Pkg, pkgFlag rewrite.PkgFlag) bool) {
				g.RangePkg(func(pkg inspect.Pkg) bool {
					// rewrite for the same module
					if pkg.Module() == mod {
						f(pkg, rewrite.BitStarterMod)
					}
					if opts.ShouldRewritePackage != nil && opts.ShouldRewritePackage(pkg) {
						f(pkg, rewrite.BitExtra)
					}
					// DEBUG
					// pkgPath := pkg.Path()
					// if pkgPath == "github.com/xormplus/xorm/dialects" {
					// 	f(pkg, rewrite.BitExtra)
					// }
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

		Debug:     buildOpts.Debug,
		Output:    buildOpts.Output,
		SkipBuild: opts.SkipBuild,

		ForTest:    buildOpts.ForTest,
		GoFlags:    buildOpts.GoFlags,
		BuildFlags: buildOpts.BuildFlags,
	})
	if err != nil {
		panic(err)
	}

	result = &RewriteResult{
		BuildResult: res,
	}
	return
}
