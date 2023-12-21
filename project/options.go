package project

import (
	"github.com/xhd2015/go-inspect/inspect"
	"github.com/xhd2015/go-inspect/rewrite"
)

type Rewriter interface {
	BeforeLoad(proj Project)
	AfterLoad(proj Project)
	GenOverlay(proj Project, session inspect.Session)
	RewritePackage(proj Project, pkg inspect.Pkg, session inspect.Session)
	RewriteFile(proj Project, f inspect.FileContext, session inspect.Session)

	Finish(proj Project, err error, result *RewriteResult)
}
type RewriteCallback struct {
	BeforeLoad     func(proj Project)
	AfterLoad      func(proj Project)
	GenOverlay     func(proj Project, session inspect.Session)
	RewritePackage func(proj Project, pkg inspect.Pkg, session inspect.Session)
	RewriteFile    func(proj Project, f inspect.FileContext, session inspect.Session)
	Finish         func(proj Project, err error, result *RewriteResult)
}
type Options interface {
	Force() bool
	SetForce(force bool)

	Verbose() bool

	GetPackageFilter() func(pkg inspect.Pkg) bool
	SetPackageFiler(filter func(pkg inspect.Pkg) bool)
	AddPackageFilter(filter func(pkg inspect.Pkg) bool)

	RewriteStd() bool
	SetRewriteStd(rewriteStd bool)

	// GoFlags are common to load and build
	GoFlags() []string

	// BuildFlags only apply to build
	BuildFlags() []string
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
func (c *defaultRewriter) BeforeLoad(proj Project) {
	if c.RewriteCallback.BeforeLoad != nil {
		c.RewriteCallback.BeforeLoad(proj)
	}
}

func (c *defaultRewriter) AfterLoad(proj Project) {
	if c.RewriteCallback.AfterLoad != nil {
		c.RewriteCallback.AfterLoad(proj)
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
