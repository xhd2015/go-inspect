package project

import (
	"fmt"
	"strings"
	"testing"

	"github.com/xhd2015/go-inspect/inspect"
	"github.com/xhd2015/go-inspect/rewrite/session"
)

// go test -run TestRewriteExtraPkg -v ./project
func TestRewriteExtraPkg(t *testing.T) {
	OnProjectRewrite(func(proj session.Project) Rewriter {
		return NewDefaultRewriter(&RewriteCallback{
			BeforeLoad: func(proj session.Project, session session.Session) {
				session.Options().AddPackageFilter(func(pkg inspect.Pkg) bool {
					if strings.HasSuffix(pkg.Path(), "extra_pkg/extra") {
						fmt.Printf("set rewrite extra to true\n")
						return true
					}
					return false
				})
			},
			RewriteFile: func(proj session.Project, f inspect.FileContext, session session.Session) {
				if strings.HasSuffix(f.Pkg().Path(), "extra_pkg/extra") {
					fmt.Printf("rewrite extra file:%s\n", f.AbsPath())
					session.FileRewrite(f).Append("\n// Hello world\n")
				}
			},
			GenOverlay: func(proj session.Project, session session.Session) {
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
