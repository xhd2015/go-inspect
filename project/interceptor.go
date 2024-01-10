package project

import (
	"github.com/xhd2015/go-inspect/inspect"
	"github.com/xhd2015/go-inspect/rewrite/session"
)

var projectListeners []func(proj session.Project) Rewriter
var beforeLoadListeners []func(proj session.Project, session session.Session)
var initSesssionListeners []func(proj session.Project, session session.Session)
var afterLoadListeners []func(proj session.Project, session session.Session)
var genOverlayListeners []func(proj session.Project, session session.Session)
var rewritePackageListeners []func(proj session.Project, pkg inspect.Pkg, session session.Session)
var rewriteFileListeners []func(proj session.Project, f inspect.FileContext, session session.Session)
var finishListeners []func(proj session.Project, err error, result *RewriteResult)

// OnProjectRewrite called for all projects,then the
// returned Rewritter will only be applied to the
// project only
func OnProjectRewrite(fn func(proj session.Project) Rewriter) {
	projectListeners = append(projectListeners, fn)
}

// BeforeLoad called for all projects
func BeforeLoad(fn func(proj session.Project, session session.Session)) {
	beforeLoadListeners = append(beforeLoadListeners, fn)
}

// BeforeLoad called for all projects
func InitSesson(fn func(proj session.Project, session session.Session)) {
	initSesssionListeners = append(initSesssionListeners, fn)
}

func AfterLoad(fn func(proj session.Project, session session.Session)) {
	afterLoadListeners = append(afterLoadListeners, fn)
}

// OnOverlay called for all projects
func OnOverlay(fn func(proj session.Project, session session.Session)) {
	genOverlayListeners = append(genOverlayListeners, fn)
}

// OnRewritePackage called for all projects
func OnRewritePackage(fn func(proj session.Project, pkg inspect.Pkg, session session.Session)) {
	rewritePackageListeners = append(rewritePackageListeners, fn)
}

// OnRewriteFile called for all projects
func OnRewriteFile(fn func(proj session.Project, f inspect.FileContext, session session.Session)) {
	rewriteFileListeners = append(rewriteFileListeners, fn)
}

// OnRewriteFile called for all projects
func OnFinish(fn func(proj session.Project, err error, result *RewriteResult)) {
	finishListeners = append(finishListeners, fn)
}
