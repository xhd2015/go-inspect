package cmd

import (
	inspect_old "github.com/xhd2015/go-inspect/inspect_old"
)

type GenRewriteOptions struct {
	Verbose        bool
	VerboseCopy    bool
	VerboseRewrite bool
	// VerboseGomod   bool
	ProjectDir     string // the project dir
	RewriteOptions *inspect_old.RewriteOptions
	StubGenDir     string // relative path, default test/mock_gen
	SkipGenMock    bool

	PkgFilterOptions

	Force bool // force indicates no cache

	LoadOptions
}

type LoadOptions struct {
	LoadArgs []string // passed to packages.Load

	ForTest bool
}

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
