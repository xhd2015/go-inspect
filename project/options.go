package project

import "github.com/xhd2015/go-inspect/inspect"

type Rewriter interface {
	Init(proj Project)
	GenOverlay(proj Project, session inspect.Session)
	RewritePackage(proj Project, pkg inspect.Pkg, session inspect.Session)
	RewriteFile(proj Project, f inspect.FileContext, session inspect.Session)

	Finish(proj Project, err error, result *RewriteResult)
}
type RewriteCallback struct {
	Init           func(proj Project)
	GenOverlay     func(proj Project, session inspect.Session)
	RewritePackage func(proj Project, pkg inspect.Pkg, session inspect.Session)
	RewriteFile    func(proj Project, f inspect.FileContext, session inspect.Session)
	Finish         func(proj Project, err error, result *RewriteResult)
}

type defaultRewriter struct {
	*RewriteCallback
}

var _ Rewriter = (*defaultRewriter)(nil)

func NewDefaultRewriter(callback *RewriteCallback) Rewriter {
	return &defaultRewriter{RewriteCallback: callback}
}

// Init implements Rewriter
func (c *defaultRewriter) Init(proj Project) {
	if c.RewriteCallback.Init != nil {
		c.RewriteCallback.Init(proj)
	}
}

// GenOverlay implements Rewriter
func (c *defaultRewriter) GenOverlay(proj Project, session inspect.Session) {
	if c.RewriteCallback.GenOverlay != nil {
		c.RewriteCallback.GenOverlay(proj, session)
	}
}

// RewriteFile implements Rewriter
func (c *defaultRewriter) RewriteFile(proj Project, f inspect.FileContext, session inspect.Session) {
	if c.RewriteCallback.RewriteFile != nil {
		c.RewriteCallback.RewriteFile(proj, f, session)
	}
}

// RewritePackage implements Rewriter
func (c *defaultRewriter) RewritePackage(proj Project, pkg inspect.Pkg, session inspect.Session) {
	if c.RewriteCallback.RewritePackage != nil {
		c.RewriteCallback.RewritePackage(proj, pkg, session)
	}
}

func (c *defaultRewriter) Finish(proj Project, err error, result *RewriteResult) {
	if c.RewriteCallback.Finish != nil {
		c.RewriteCallback.Finish(proj, err, result)
	}
}
