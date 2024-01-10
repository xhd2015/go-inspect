package session

import "github.com/xhd2015/go-inspect/inspect"

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
