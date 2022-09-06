package rewrite

import (
	"fmt"
	"go/ast"
	"path"
	"strings"
	"testing"

	"github.com/xhd2015/go-objpath"

	"github.com/xhd2015/go-inspect/inspect"
)

// go test -run TestRewriteSimple -v ./rewrite
func TestRewriteSimple(t *testing.T) {
	rewriteRoot := GetTmpRewriteRoot(t.Name())
	genMap := make(map[string]*Content)

	ctrl := &ControllerFuncs{
		GenOverlayFn: func(g inspect.Global, session inspect.Session) map[string]*Content {
			session.Gen(&inspect.EditCallbackFn{
				Rewrites: func(f inspect.FileContext, content string) bool {
					newPath := CleanGoFsPath(path.Join(rewriteRoot, f.AbsPath()))
					genMap[newPath] = &Content{
						SrcFile: f.AbsPath(),
						Content: []byte(content),
					}
					return true
				},
			})
			return genMap
		},
	}
	vis := &inspect.Visitors{
		VisitFn: func(n ast.Node, session inspect.Session) bool {
			if file, ok := n.(*ast.File); ok {
				f := session.Global().Registry().File(file)
				edit := session.FileRewrite(f)
				fmtPkg := edit.MustImport("fmt", "fmt", "", nil)
				edit.AddAnaymouseInit(fmt.Sprintf(`;var _ = func() bool { %s.Printf("hello");return true;}`, fmtPkg))
				return false
			}
			return true
		},
	}
	res, err := GenRewrite([]string{"./"}, rewriteRoot, ctrl, vis, &GenRewriteOptions{
		ProjectDir: "./testdata/simple",
		Verbose:    true,
		Force:      true,
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("res:%+v", res)

	objpath.AssertT(t, genMap, `{"$length":1}`)

	var content string
	for _, c := range genMap {
		content = string(c.Content)
		break
	}

	expectEnds := `;var _ = func() bool { fmt.Printf("hello");return true;}`
	if !strings.HasSuffix(content, expectEnds) {
		lines := strings.Split(content, "\n")
		var last string
		if len(lines) > 0 {
			last = lines[len(lines)-1]
		}
		t.Fatalf("expect content ends with %s, actual: %s", expectEnds, last)
	}
}
