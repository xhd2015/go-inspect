package analysis

import (
	"fmt"
	"go/ast"
	"regexp"
	"strings"
	"testing"

	"github.com/xhd2015/go-inspect/inspect"
	"github.com/xhd2015/go-inspect/inspect/load"
)

// go test -run TestLoadMatch -v ./analysis/
func TestLoadMatch(t *testing.T) {
	m := &Matchers{
		MatchFunc: func(g inspect.Global, decl *ast.FuncDecl, lit *ast.FuncLit, n ast.Node) bool {
			if decl == nil || lit != nil {
				return false
			}
			gdec := g.Registry().FuncDecl(decl).QuanlifiedName()

			reg := regexp.MustCompile(`[Ss]tatus\.Run`)
			if reg.MatchString(gdec) {
				return true
			}
			return false
		},
		IncludeFunc: func(g inspect.Global, n ast.Node) bool {
			f := g.Registry().FileOf(n)
			return strings.Contains(f.Pkg().Path(), "testdata/simple")
		},
	}
	g, nodes, err := LoadMatch([]string{"./testdata/simple"}, m, &load.LoadOptions{})

	if err != nil {
		t.Fatal(err)
	}
	resMap := make(map[string]bool, len(nodes))
	for n := range nodes {
		fn := g.Registry().FuncDecl(n.(*ast.FuncDecl))
		resMap[fn.QuanlifiedName()] = true

		// code := g.CodeSlice(n.Pos(), n.End())
		// t.Logf("%s", code)
	}
	s := fmt.Sprintf("%+v", resMap)
	exp := `map[Status.Run:true myStringer.String:true]`
	if s != exp {
		t.Fatalf("expect %s = %+v, actual:%+v", `s`, exp, s)
	}

}
