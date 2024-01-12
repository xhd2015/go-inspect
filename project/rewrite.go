package project

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"go/ast"
	"os"
	"path"
	"path/filepath"

	"github.com/xhd2015/go-inspect/inspect"
	"github.com/xhd2015/go-inspect/inspect/util"
	"github.com/xhd2015/go-inspect/rewrite"
	"github.com/xhd2015/go-inspect/rewrite/session"
	"github.com/xhd2015/go-inspect/rewrite/session/session_impl"
	"github.com/xhd2015/go-vendor-pack/writefs/memfs"
)

type EditCallbackFn = session.EditCallbackFn
type BuildOpts = rewrite.BuildOpts
type RewriteOpts = rewrite.RewriteOpts

type RewriteResult struct {
	*rewrite.BuildResult
}

func Rewrite(loadArgs []string, opts *RewriteOpts) *RewriteResult {
	var extraCallbacks []Rewriter
	return doRewrite(loadArgs, &RewriteCallbackOpts{
		RewriteOpts: opts,
		RewriteCallback: &RewriteCallback{
			BeforeLoad: func(proj session.Project, session session.Session) {
				for _, f := range projectListeners {
					callback := f(proj)
					if callback != nil {
						extraCallbacks = append(extraCallbacks, callback)
					}
				}
				for _, f := range beforeLoadListeners {
					f(proj, session)
				}
				for _, callback := range extraCallbacks {
					callback.BeforeLoad(proj, session)
				}
			},
			InitSession: func(proj session.Project, session session.Session) {
				for _, f := range initSesssionListeners {
					f(proj, session)
				}
				for _, callback := range extraCallbacks {
					callback.InitSession(proj, session)
				}
			},
			AfterLoad: func(proj session.Project, session session.Session) {
				for _, f := range afterLoadListeners {
					f(proj, session)
				}
				for _, callback := range extraCallbacks {
					callback.AfterLoad(proj, session)
				}
			},
			GenOverlay: func(proj session.Project, session session.Session) {
				for _, f := range genOverlayListeners {
					f(proj, session)
				}
				for _, callback := range extraCallbacks {
					callback.GenOverlay(proj, session)
				}
			},
			RewritePackage: func(proj session.Project, pkg inspect.Pkg, session session.Session) {
				for _, f := range rewritePackageListeners {
					f(proj, pkg, session)
				}
				for _, callback := range extraCallbacks {
					callback.RewritePackage(proj, pkg, session)
				}
			},
			RewriteFile: func(proj session.Project, file inspect.FileContext, session session.Session) {
				for _, f := range rewriteFileListeners {
					f(proj, file, session)
				}
				for _, callback := range extraCallbacks {
					callback.RewriteFile(proj, file, session)
				}
			},
			Finish: func(proj session.Project, err error, result *RewriteResult) {
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

	projectAbsDir, err := util.ToAbsPath(buildOpts.ProjectDir)
	if err != nil {
		panic(fmt.Errorf("get abs dir err:%v", err))
	}

	dg := md5.New()
	dg.Write([]byte(projectAbsDir))
	// rewriteMetaRoot = {rewriteBase}/{rewriteName}/{path_md5}
	rewriteMetaRoot := rewrite.GetRewriteRoot(filepath.Join(rewriteBase, rewriteName), hex.EncodeToString(dg.Sum(nil)))
	rewriteRoot := filepath.Join(rewriteMetaRoot, "src")

	projectRewriteRoot := path.Join(rewriteRoot, projectAbsDir)
	rewriteProjectVendorRoot := path.Join(rewriteRoot, "vendor")

	ctrl := &rewrite.ControllerFuncs{
		BeforeLoadFn: func(rwOpts *rewrite.BuildRewriteOptions, session session.Session) {
			session_impl.OnSessionOpts(session, &options{
				opts:           opts.RewriteOpts,
				underlyingOpts: rwOpts,
			})
			dirs := &sessionDirs{
				projectRoot:              projectAbsDir,
				rewriteMetaRoot:          rewriteMetaRoot,
				rewriteRoot:              rewriteRoot,
				rewriteProjectRoot:       projectRewriteRoot,
				rewriteProjectVendorRoot: rewriteProjectVendorRoot,
			}
			session_impl.OnSessionDirs(session, dirs)

			proj = &project{
				opts: &loadOptions{
					verbose:    opts.BuildOpts.Verbose,
					goFlags:    opts.BuildOpts.GoFlags,
					buildFlags: opts.BuildOpts.BuildFlags,
				},
				args:        loadArgs,
				projectRoot: dirs.projectRoot,
				vendor:      hasVendorDir(projectAbsDir),
			}
			session_impl.OnSessionProject(session, proj)
			opts.BeforeLoad(proj, session)
		},
		InitSessionFn: func(g inspect.Global, session session.Session) {
			proj.g = g
			// find the first package, define that as main
			// packages
			pkgs := g.LoadInfo().StarterPkgs()
			if len(pkgs) == 0 {
				panic(fmt.Errorf("no packages"))
			}
			proj.mainPkg = pkgs[0]

			opts.InitSession(proj, session)
		},
		AfterLoadFn: func(g inspect.Global, session session.Session) {
			opts.AfterLoad(proj, session)
		},
		// TODO: add a explicit init function
		// called first
		FilterPkgsFn: func(g inspect.Global, session session.Session) func(func(p inspect.Pkg, pkgFlag rewrite.PkgFlag) bool) {
			pkgFilter := session.Options().GetPackageFilter()
			mod := g.LoadInfo().MainModule()
			return func(f func(p inspect.Pkg, pkgFlag rewrite.PkgFlag) bool) {
				g.RangePkg(func(pkg inspect.Pkg) bool {
					// rewrite for the same module
					if pkg.Module() == mod {
						f(pkg, rewrite.BitStarterMod)
					} else {
						if pkgFilter != nil && pkgFilter(pkg) {
							f(pkg, rewrite.BitExtra)
						}
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
		GenOverlayFn: func(g inspect.Global, session session.Session) map[string]*rewrite.Content {
			if opts.GenOverlay != nil {
				opts.GenOverlay(proj, session)
			}

			// template code
			for file, code := range opts.PreCode {
				err := session.SetRewriteFile(file, code)
				if err != nil {
					panic(fmt.Errorf("pre code: %s %w", file, err))
				}
			}

			session.Gen(&EditCallbackFn{
				Rewrites: func(f inspect.FileContext, content string) bool {
					absPath := f.AbsPath()
					err := session.SetRewriteFile(f.AbsPath(), content)
					if err != nil {
						panic(fmt.Errorf("rewrite: %s %w", absPath, err))
					}
					return true
				},
				Pkg: func(p inspect.Pkg, kind, realName, content string) bool {
					name := path.Join(p.Dir(), realName+".go")
					err := session.SetRewriteFile(name, content)
					if err != nil {
						panic(fmt.Errorf("rewrite pkg file: %s %w", name, err))
					}
					return true
				},
			})

			genMap := make(map[string]*rewrite.Content)
			session.RewriteFS().TraversePath(func(path string, e memfs.MemFileInfo) bool {
				if !e.IsDir() {
					genMap[path] = &rewrite.Content{
						Content: e.Buffer().Bytes(),
					}
				}
				return true
			})
			return genMap
		},
	}
	vis := &rewrite.Visitors{
		// the granliarity is at package and file level
		// detailed nodes are not touched
		VisitFn: func(n ast.Node, session session.Session) bool {
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

func hasVendorDir(projectAbsDir string) bool {
	vendorDir := path.Join(projectAbsDir, "vendor")
	stat, err := os.Stat(vendorDir)
	if err != nil {
		if !os.IsNotExist(err) {
			panic(fmt.Errorf("stating vendor directory: %v", err))
		}
		return false
	}
	return stat.IsDir()
}
