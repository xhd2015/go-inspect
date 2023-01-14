package export_g

import (
	"fmt"
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
	edit.AddCode(fmt.Sprintf(`func Getg_GoInspectExported() *g { return getg() }`))

	// add an extra package with name 0 to make it import earlier than others
	pkgDir := proj.AllocExtraPkg("0")
	proj.NewFile(path.Join(pkgDir, "export_g_runtime_impl.go"), fmt.Sprintf(`package init_getg

import (
	"runtime"
	"unsafe"

	"github.com/xhd2015/go-inspect/plugin/getg"
)

func init(){
	getg.GetImpl = func() unsafe.Pointer { 
		return unsafe.Pointer(runtime.Getg_GoInspectExported())
	}
}`))

	// import from main
	gedit := session.PackageEdit(proj.MainPkg(), "0")
	gedit.MustImport(path.Join(proj.MainPkg().Path(), path.Base(pkgDir)), "export_getg", "_", nil)
}

var _ project.Rewriter = (*rewritter)(nil)
