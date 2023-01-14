package project

import (
	"fmt"
	"path"
	"testing"

	"github.com/xhd2015/go-inspect/inspect"
)

// working
// go test -run TestRewriteStdRuntimeGenInPlace -v ./project
func TestRewriteStdRuntimeGenInPlace(t *testing.T) {
	OnProjectRewrite(func(proj Project) Rewriter {
		return NewDefaultRewriter(&RewriteCallback{
			BeforeLoad: func(proj Project) {
				proj.Options().SetRewriteStd(true)
			},
			GenOverlay: func(proj Project, session inspect.Session) {
				g := proj.Global()

				runtimePkg := g.GetPkg("runtime")
				edit := session.PackageEdit(runtimePkg, "export_getg")
				edit.AddCode(fmt.Sprintf(`func Getg_GoInspectExported() *g { return getg() }`))

				// must check if we have imported the package, if not, dont do rewrite
				exportGPkg := g.GetPkg("github.com/xhd2015/go-inspect/project/testdata/export_g")
				if exportGPkg == nil {
					panic(fmt.Errorf("no export_g package found"))
				}

				pkgDir := proj.AllocExtraPkg("0")
				proj.NewFile(path.Join(pkgDir, "export_g_runtime_impl.go"), fmt.Sprintf(`package init_getg

				import (
					"runtime"
					"unsafe"
					"github.com/xhd2015/go-inspect/project/testdata/export_g"
				)

				func init(){
					export_g.GetImpl = func() unsafe.Pointer { 
						return unsafe.Pointer(runtime.Getg_GoInspectExported())
					}
				}`))

				gedit := session.PackageEdit(proj.MainPkg(), "0")
				gedit.MustImport(path.Join(proj.MainPkg().Path(), path.Base(pkgDir)), "export_getg", "_", nil)
			},
		})
	})
	Rewrite([]string{}, &RewriteOpts{
		BuildOpts: &BuildOpts{
			ProjectDir: "./testdata/hello_g",
			Output:     "test.bin",
			Verbose:    true,
			Force:      true,
		},
	})
}

// not working, show error:  no required module provides package github.com/xhd2015/go-inspect/inspect/project/testdata/export_g; to add it: ....
// go test -run TestRewriteStdRuntimeCallback -v ./project
func TestRewriteStdRuntimeCallback(t *testing.T) {
	OnProjectRewrite(func(proj Project) Rewriter {
		return NewDefaultRewriter(&RewriteCallback{
			BeforeLoad: func(proj Project) {
				proj.Options().SetRewriteStd(true)
			},
			GenOverlay: func(proj Project, session inspect.Session) {
				g := proj.Global()
				runtimePkg := g.GetPkg("runtime")

				edit := session.PackageEdit(runtimePkg, "export_getg")
				refExportG := edit.MustImport("github.com/xhd2015/go-inspect/inspect/project/testdata/export_g", "export_g", "", nil)
				unsafePkg := edit.MustImport("unsafe", "unsafe", "", nil)

				edit.AddCode(fmt.Sprintf(`func init(){ %s.GetImpl = func() %s.Pointer { return %s.Pointer(getg()) } }`, refExportG, unsafePkg, unsafePkg))
			},
		})
	})
	Rewrite([]string{}, &RewriteOpts{
		BuildOpts: &BuildOpts{
			ProjectDir: "./testdata/hello",
			Output:     "test.bin",
		},
	})
}

// not working, because only main module and std is rewritten
// the export_g belongs to another module
// go test -run TestRewriteStdRuntimeGenIntoExportG -v ./project
func TestRewriteStdRuntimeGenIntoExportG(t *testing.T) {
	OnProjectRewrite(func(proj Project) Rewriter {
		return NewDefaultRewriter(&RewriteCallback{
			BeforeLoad: func(proj Project) {
				proj.Options().SetRewriteStd(true)
			},
			GenOverlay: func(proj Project, session inspect.Session) {
				g := proj.Global()

				runtimePkg := g.GetPkg("runtime")
				edit := session.PackageEdit(runtimePkg, "export_getg")
				edit.AddCode(fmt.Sprintf(`func Getg_GoInspectExported() *g { return getg() }`))

				exportGPkg := g.GetPkg("github.com/xhd2015/go-inspect/project/testdata/export_g")
				if exportGPkg == nil {
					panic(fmt.Errorf("no export_g package found"))
				}
				gedit := session.PackageEdit(exportGPkg, "runtime_impl")
				gedit.MustImport("runtime", "runtime", "", nil)
				gedit.MustImport("unsafe", "unsafe", "", nil)
				gedit.AddCode(fmt.Sprintf("func init() { GetImpl = func() unsafe.Pointer { return unsafe.Pointer(runtime.Getg_GoInspectExported()) } }"))
			},
		})
	})
	Rewrite([]string{}, &RewriteOpts{
		BuildOpts: &BuildOpts{
			ProjectDir: "./testdata/hello_g",
			Output:     "test.bin",
			Verbose:    true,
		},
	})
}
