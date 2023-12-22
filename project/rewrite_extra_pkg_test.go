package project

import (
	"fmt"
	"strings"
	"testing"

	"github.com/xhd2015/go-inspect/inspect"
)

// go test -run TestRewriteExtraPkg -v ./project
func TestRewriteExtraPkg(t *testing.T) {
	OnProjectRewrite(func(proj Project) Rewriter {
		return NewDefaultRewriter(&RewriteCallback{
			BeforeLoad: func(proj Project) {
				proj.Options().AddPackageFilter(func(pkg inspect.Pkg) bool {
					if strings.HasSuffix(pkg.Path(), "extra_pkg/extra") {
						fmt.Printf("set rewrite extra to true\n")
						return true
					}
					return false
				})
			},
			RewriteFile: func(proj Project, f inspect.FileContext, session inspect.Session) {
				if strings.HasSuffix(f.Pkg().Path(), "extra_pkg/extra") {
					fmt.Printf("rewrite extra file:%s\n", f.AbsPath())
					session.FileRewrite(f).Append("\n// Hello world\n")
				}
			},
			GenOverlay: func(proj Project, session inspect.Session) {
			},
		})
	})
	Rewrite([]string{}, &RewriteOpts{
		BuildOpts: &BuildOpts{
			ProjectDir: "./testdata/extra_pkg/primary",
			Output:     "test.bin",
			Verbose:    true,
			Force:      true,
		},
	})
}
