package export_g

import (
	"log"
	"path"

	"github.com/xhd2015/go-inspect/inspect"
	"github.com/xhd2015/go-inspect/project"
)

func Use() {
	project.OnProjectRewrite(func(proj project.Project) project.Rewriter {
		return NewRewritter()
	})
}

type rewritter struct {
	project.Rewriter
}

var _ project.Rewriter = (*rewritter)(nil)

func NewRewritter() project.Rewriter {
	return &rewritter{
		Rewriter: project.NewDefaultRewriter(&project.RewriteCallback{}),
	}
}

// BeforeLoad implements project.Rewriter
func (c *rewritter) BeforeLoad(proj project.Project) {
	proj.Options().SetRewriteStd(true)
}

// GenOverlay implements project.Rewriter
func (c *rewritter) GenOverlay(proj project.Project, session inspect.Session) {
	g := proj.Global()

	// must check if we have imported the package, if not, dont do rewrite
	exportGPkg := g.GetPkg("github.com/xhd2015/go-inspect/plugin/getg")
	if exportGPkg == nil {
		log.Printf("NOTE: github.com/xhd2015/go-inspect/plugin/getg is not used, skip exporting runtime.getg()")
		return
	}

	// export the runtime.getg()
	runtimePkg := g.GetPkg("runtime")
	edit := session.PackageEdit(runtimePkg, "export_getg")
	// TODO
	edit.AddCode(`func Getg_GoInspectExported() *g { return getg() }`)

	// use getg().m.curg instead of getg()
	// see: https://github.com/golang/go/blob/master/src/runtime/HACKING.md
	edit.AddCode(`func Getcurg_GoInspectExported() *g { return getg().m.curg }`)

	// add an extra package with name 0 to make it import earlier than others
	pkgDir := proj.AllocExtraPkg("0_init_getg")
	proj.NewFile(path.Join(pkgDir, "export_g_runtime_impl.go"), `package init_getg

import (
	"runtime"
	"unsafe"

	"github.com/xhd2015/go-inspect/plugin/getg"
)

func init(){
	getg.GetImpl = func() unsafe.Pointer { 
		return unsafe.Pointer(runtime.Getcurg_GoInspectExported())
	}
}`)

	// import from main
	gedit := session.PackageEdit(proj.MainPkg(), "0_export_g")
	gedit.MustImport(path.Join(proj.MainPkg().Path(), path.Base(pkgDir)), "export_getg", "_", nil)
}
