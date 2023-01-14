package inspect

import (
	"testing"

	"github.com/xhd2015/go-inspect/inspect"
	"github.com/xhd2015/go-inspect/inspect/load"
)

// go test -run TestStdModule -v ./inspect/test
func TestStdModuleNoVendor(t *testing.T) {
	g, err := load.LoadPackages([]string{"./"}, &load.LoadOptions{
		ProjectDir: "../testdata/hello",
	})
	if err != nil {
		t.Fatal(err)
	}

	g.RangePkg(func(pkg inspect.Pkg) bool {
		if pkg.Module().IsStd() {
			t.Logf("found std: pkg=%v modPath=%v", pkg.Name(), pkg.Module().Path())
			// example: unsafe, atomic, modPath always empty
		}
		return true
	})

}

// go test -run TestStdModuleVendor -v ./inspect/test
func TestStdModuleVendor(t *testing.T) {
	g, err := load.LoadPackages([]string{"./"}, &load.LoadOptions{
		ProjectDir: "../testdata/hello",
		BuildFlags: []string{"-mod=vendor"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// same as above
	g.RangePkg(func(pkg inspect.Pkg) bool {
		if pkg.Module().IsStd() {
			t.Logf("found std: pkg=%v modPath=%v", pkg.Name(), pkg.Module().Path())
			// example: unsafe, atomic, modPath always empty
		}
		return true
	})

}
