package project

import "github.com/xhd2015/go-inspect/inspect"

var projectListeners []func(proj Project) Rewriter
var beforeLoadListeners []func(proj Project)
var afterLoadListeners []func(proj Project)
var genOverlayListeners []func(proj Project, session inspect.Session)
var rewritePackageListeners []func(proj Project, pkg inspect.Pkg, session inspect.Session)
var rewriteFileListeners []func(proj Project, f inspect.FileContext, session inspect.Session)
var finishListeners []func(proj Project, err error, result *RewriteResult)

// OnProjectRewrite called for all projects,then the
// returned Rewritter will only be applied to the
// project only
func OnProjectRewrite(fn func(proj Project) Rewriter) {
	projectListeners = append(projectListeners, fn)
}

// BeforeLoad called for all projects
func BeforeLoad(fn func(proj Project)) {
	beforeLoadListeners = append(beforeLoadListeners, fn)
}
func AfterLoad(fn func(proj Project)) {
	afterLoadListeners = append(afterLoadListeners, fn)
}

// OnOverlay called for all projects
func OnOverlay(fn func(proj Project, session inspect.Session)) {
	genOverlayListeners = append(genOverlayListeners, fn)
}

// OnRewritePackage called for all projects
func OnRewritePackage(fn func(proj Project, pkg inspect.Pkg, session inspect.Session)) {
	rewritePackageListeners = append(rewritePackageListeners, fn)
}

// OnRewriteFile called for all projects
func OnRewriteFile(fn func(proj Project, f inspect.FileContext, session inspect.Session)) {
	rewriteFileListeners = append(rewriteFileListeners, fn)
}

// OnRewriteFile called for all projects
func OnFinish(fn func(proj Project, err error, result *RewriteResult)) {
	finishListeners = append(finishListeners, fn)
}
