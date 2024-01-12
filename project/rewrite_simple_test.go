package project

import (
	"fmt"
	"testing"

	"github.com/xhd2015/go-inspect/rewrite/session"
)

// go test -run TestRewriteSimpleProject -v ./project
func TestRewriteSimpleProject(t *testing.T) {
	OnProjectRewrite(func(proj session.Project) Rewriter {
		return NewDefaultRewriter(&RewriteCallback{
			BeforeLoad: func(proj session.Project, session session.Session) {
				fmt.Printf("before load\n")
			},
			GenOverlay: func(proj session.Project, session session.Session) {
				fmt.Printf("GenOverlay load\n")
			},
		})
	})
	Rewrite([]string{}, &RewriteOpts{
		BuildOpts: &BuildOpts{
			ProjectDir: "./testdata/simple",
			Output:     "test.bin",
			Verbose:    true,
			// Force:      true,
		},
	})
}
