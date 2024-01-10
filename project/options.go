package project

import (
	"github.com/xhd2015/go-inspect/inspect"
	"github.com/xhd2015/go-inspect/rewrite"
	"github.com/xhd2015/go-inspect/rewrite/session"
)

type Rewriter interface {
	BeforeLoad(proj session.Project, session session.Session)
	InitSession(proj session.Project, session session.Session)
	AfterLoad(proj session.Project, session session.Session)
	GenOverlay(proj session.Project, session session.Session)
	RewritePackage(proj session.Project, pkg inspect.Pkg, session session.Session)
	RewriteFile(proj session.Project, f inspect.FileContext, session session.Session)

	Finish(proj session.Project, err error, result *RewriteResult)
}
type RewriteCallback struct {
	BeforeLoad     func(proj session.Project, session session.Session)
	InitSession    func(proj session.Project, session session.Session)
	AfterLoad      func(proj session.Project, session session.Session)
	GenOverlay     func(proj session.Project, session session.Session)
	RewritePackage func(proj session.Project, pkg inspect.Pkg, session session.Session)
	RewriteFile    func(proj session.Project, f inspect.FileContext, session session.Session)
	Finish         func(proj session.Project, err error, result *RewriteResult)
}

type Options = session.Options

type loadOptions struct {
	verbose bool

	goFlags    []string // passed to go load
	buildFlags []string // passed to go build
}

var _ session.LoadOptions = (*loadOptions)(nil)

// BuildFlags implements LoadOptions.
func (c *loadOptions) BuildFlags() []string {
	return c.buildFlags
}

// GoFlags implements LoadOptions.
func (c *loadOptions) GoFlags() []string {
	return c.goFlags
}

// Verbose implements LoadOptions.
func (c *loadOptions) Verbose() bool {
	return c.verbose
}

type options struct {
	opts           *RewriteOpts
	underlyingOpts *rewrite.BuildRewriteOptions
}

func (c *options) GetPackageFilter() func(pkg inspect.Pkg) bool {
	return c.opts.ShouldRewritePackage
}

func (c *options) SetPackageFiler(filter func(pkg inspect.Pkg) bool) {
	c.opts.ShouldRewritePackage = filter
}

func (c *options) AddPackageFilter(filter func(pkg inspect.Pkg) bool) {
	if c.opts.ShouldRewritePackage == nil {
		c.opts.ShouldRewritePackage = filter
		return
	}
	prevFilter := c.opts.ShouldRewritePackage
	c.opts.ShouldRewritePackage = func(pkg inspect.Pkg) bool {
		return prevFilter(pkg) || filter(pkg)
	}
}

// RewriteStd implements Options
func (c *options) RewriteStd() bool {
	return c.underlyingOpts.RewriteStd
}

// SetRewriteStd implements Options
func (c *options) SetRewriteStd(rewriteStd bool) {
	c.underlyingOpts.RewriteStd = rewriteStd
}

// Force implements Options
func (c *options) Force() bool {
	return c.underlyingOpts.Force
}

// SetForce implements Options
func (c *options) SetForce(force bool) {
	c.opts.BuildOpts.Force = force
	c.underlyingOpts.Force = force
}

func (c *options) Verbose() bool {
	return c.opts.BuildOpts.Verbose
}

// GoFlags are common to load and build
func (c *options) GoFlags() []string {
	return c.opts.BuildOpts.GoFlags
}

// BuildFlags only apply to build
func (c *options) BuildFlags() []string {
	return c.opts.BuildOpts.BuildFlags
}

var _ Options = (*options)(nil)

type defaultRewriter struct {
	*RewriteCallback
}

var _ Rewriter = (*defaultRewriter)(nil)

func NewDefaultRewriter(callback *RewriteCallback) Rewriter {
	return &defaultRewriter{RewriteCallback: callback}
}

// Init implements Rewriter
func (c *defaultRewriter) BeforeLoad(proj session.Project, session session.Session) {
	if c.RewriteCallback.BeforeLoad != nil {
		c.RewriteCallback.BeforeLoad(proj, session)
	}
}

func (c *defaultRewriter) InitSession(proj session.Project, session session.Session) {
	if c.RewriteCallback.InitSession != nil {
		c.RewriteCallback.InitSession(proj, session)
	}
}
func (c *defaultRewriter) AfterLoad(proj session.Project, session session.Session) {
	if c.RewriteCallback.AfterLoad != nil {
		c.RewriteCallback.AfterLoad(proj, session)
	}
}

// GenOverlay implements Rewriter
func (c *defaultRewriter) GenOverlay(proj session.Project, session session.Session) {
	if c.RewriteCallback.GenOverlay != nil {
		c.RewriteCallback.GenOverlay(proj, session)
	}
}

// RewriteFile implements Rewriter
func (c *defaultRewriter) RewriteFile(proj session.Project, f inspect.FileContext, session session.Session) {
	if c.RewriteCallback.RewriteFile != nil {
		c.RewriteCallback.RewriteFile(proj, f, session)
	}
}

// RewritePackage implements Rewriter
func (c *defaultRewriter) RewritePackage(proj session.Project, pkg inspect.Pkg, session session.Session) {
	if c.RewriteCallback.RewritePackage != nil {
		c.RewriteCallback.RewritePackage(proj, pkg, session)
	}
}

func (c *defaultRewriter) Finish(proj session.Project, err error, result *RewriteResult) {
	if c.RewriteCallback.Finish != nil {
		c.RewriteCallback.Finish(proj, err, result)
	}
}
