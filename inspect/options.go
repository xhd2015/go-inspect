package inspect

type Options interface {
	Force() bool
	SetForce(force bool)

	Verbose() bool

	GetPackageFilter() func(pkg Pkg) bool
	SetPackageFiler(filter func(pkg Pkg) bool)
	AddPackageFilter(filter func(pkg Pkg) bool)

	RewriteStd() bool
	SetRewriteStd(rewriteStd bool)

	// GoFlags are common to load and build
	GoFlags() []string

	// BuildFlags only apply to build
	BuildFlags() []string
}
