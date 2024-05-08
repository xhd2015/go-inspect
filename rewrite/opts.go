package rewrite

import "github.com/xhd2015/go-inspect/inspect"

type PkgFilterOptions struct {
	OnlyPackages map[string]bool
	Packages     map[string]bool
	Modules      map[string]bool
	AllowMissing bool
}

type GenRewriteResult struct {
	// original dir to new dir, original dir may contain @version, new dir replace @ with /
	// to be used as -trim when building
	MappedMod    map[string]string
	UseNewGOROOT string
}

// TODO: merge these 4 options

// read only
type BuildOpts struct {
	ProjectDir string
	Verbose    bool
	Force      bool
	Debug      bool
	Output     string
	ForTest    bool
	GoFlags    []string // passed to go load
	BuildFlags []string // passed to go build

	DisableTrimPath bool
	GoBinary        string
}

// readonly options
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

type BuildOptions struct {
	Verbose     bool
	ProjectRoot string // default CWD
	RebaseRoot  string
	Debug       bool
	Output      string
	ForTest     bool
	GoFlags     []string
	// extra trim path map to be applied
	// cleanedModOrigAbsDir - modOrigAbsDir
	MappedMod map[string]string
	NewGoROOT string

	DisableTrimPath bool
	GoBinary        string
}

type BuildRewriteOptions struct {
	Verbose        bool
	VerboseCopy    bool
	VerboseRewrite bool
	// VerboseGomod   bool
	ProjectDir string // the project dir
	RebaseRoot string

	// RewriteStd should files inside GOROOT/src
	// be modified?
	RewriteStd bool

	Force bool // force indicates no cache

	// for load & build
	ForTest    bool
	GoFlags    []string // passed to load packages,go build
	BuildFlags []string // flags only passed to go build, not loading

	// for build
	Debug     bool
	Output    string
	SkipBuild bool

	DisableTrimPath bool
	GoBinary        string
}
