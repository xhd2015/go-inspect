package rewrite

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
