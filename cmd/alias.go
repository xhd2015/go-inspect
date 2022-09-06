package cmd

import "github.com/xhd2015/go-inspect/rewrite"

type GenRewriteOptions = rewrite.GenRewriteOptions
type GenRewriteResult = rewrite.GenRewriteResult
type PkgFilterOptions = rewrite.PkgFilterOptions
type Content = rewrite.Content
type LoadOptions = rewrite.LoadOptions

// Why this PkgFlag?
type PkgFlag = rewrite.PkgFlag

const BitExtra = rewrite.BitExtra
const BitStarter = rewrite.BitStarter
const BitStarterMod = rewrite.BitStarterMod
