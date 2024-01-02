package export_g

import (
	"fmt"
	"path"
	"path/filepath"

	"github.com/xhd2015/go-inspect/inspect"
	"github.com/xhd2015/go-inspect/project"
	"github.com/xhd2015/go-vendor-pack/unpack"
)

//go:generate bash -ec "cd gen_pack && bash gen.sh"

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
func (c *rewritter) BeforeLoad(proj project.Project, session inspect.Session) {
	session.Options().SetRewriteStd(true)
}

// GenOverlay implements project.Rewriter
func (c *rewritter) GenOverlay(proj project.Project, session inspect.Session) {
	g := proj.Global()

	// unpack getg
	unpackGetg(proj)

	// update: skip getg check because a framework may generate it on the fly
	// update(2023.12): now we use unpackGetg to ensure getg always present
	//
	// must check if we have imported the package, if not, dont do rewrite
	// exportGPkg := g.GetPkg("github.com/xhd2015/go-inspect/plugin/getg")
	// if exportGPkg == nil {
	// 	log.Printf("NOTE: github.com/xhd2015/go-inspect/plugin/getg is not used, skip exporting runtime.getg()")
	// 	return
	// }

	// export the runtime.getg()
	runtimePkg := g.GetPkg("runtime")
	edit := session.PackageEdit(runtimePkg, "export_getg")
	// TODO
	edit.AddCode(`func Getg_GoInspectExported() *g { return getg() }`)

	// use getg().m.curg instead of getg()
	// see: https://github.com/golang/go/blob/master/src/runtime/HACKING.md
	edit.AddCode(`func Getcurg_GoInspectExported() *g { return getg().m.curg }`)

	// add an extra package with name 0 to make it import earlier than others
	pkgDir := proj.AllocExtraPkg("0_000_init_getg")
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
	gedit := session.PackageEdit(proj.MainPkg(), "0_000_export_g")
	gedit.MustImport(path.Join(proj.MainPkg().Path(), path.Base(pkgDir)), "export_getg", "_", nil)

	// finally, remove err msg detecting
	removeGetgErrMsg(proj, session)
}

func removeGetgErrMsg(proj project.Project, session inspect.Session) {
	g := proj.Global()

	pkg := g.GetPkg("github.com/xhd2015/go-inspect/plugin/getg")
	if pkg == nil {
		return
	}
	pkg.RangeFiles(func(i int, f inspect.FileContext) bool {
		baseName := filepath.Base(f.AbsPath())
		if baseName == "err_msg.go" {
			ast := f.AST()
			session.FileRewrite(f).Replace(ast.Pos(), ast.End(), "package "+ast.Name.Name)
			return false
		}
		return true
	})
}

func unpackGetg(proj project.Project) {
	err := unpack.UnpackFromBase64Decode(GETG_PACK, proj.RewriteProjectRoot(), &unpack.Options{
		ForceUpgradeModules: map[string]bool{
			"github.com/xhd2015/go-inspect/plugin/getg": true,
		},
		OptionalSumModules: map[string]bool{
			"github.com/xhd2015/go-inspect/plugin/getg": true,
		},
	})
	if err != nil {
		panic(fmt.Errorf("GenOverlay tls: %w", err))
	}
}
