package inspect

import (
	"strings"
	"testing"

	"github.com/xhd2015/go-inspect/inspect/util"
	"github.com/xhd2015/go-objpath"
)

// go test -run TestImportNonReferencing -v ./inspect
func TestImportNonReferencing(t *testing.T) {
	var imports []string

	imp := NewImportList_X(nil, func(use, pkg string) {
		imports = append(imports, util.FormatImport(use, pkg))
	})

	imp.MustImport("a.b.c/a", "a", "_", nil)

	t.Logf("imports: %+v", strings.Join(imports, "\n"))
	objpath.Assert(imports, `{"$length":1, "0":"_ \"a.b.c/a\""}`)
}

// go test -run TestImportTwiceWithAnotherRef -v ./inspect
func TestImportTwiceWithAnotherRef(t *testing.T) {
	var imports []string

	imp := NewImportList_X(nil, func(use, pkg string) {
		imports = append(imports, util.FormatImport(use, pkg))
	})

	imp.MustImport("a.b.c/a", "a", "", nil)
	imp.MustImport("a.b.c/a", "a", "a2", nil)

	t.Logf("imports: %+v", strings.Join(imports, "\n"))
	objpath.Assert(imports, `{"$length":2, "0":"\"a.b.c/a\"","1":"a2 \"a.b.c/a\""}`)
}

// go test -run TestImportTwiceWithNonRef -v ./inspect
func TestImportTwiceWithNonRef(t *testing.T) {
	var imports []string

	imp := NewImportList_X(nil, func(use, pkg string) {
		imports = append(imports, util.FormatImport(use, pkg))
	})

	imp.MustImport("a.b.c/a", "a", "", nil)
	imp.MustImport("a.b.c/a", "a", "_", nil)

	t.Logf("imports: %+v", strings.Join(imports, "\n"))
	objpath.Assert(imports, `{"$length":1, "0":"\"a.b.c/a\""}`)
}
