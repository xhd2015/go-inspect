package export_g

import (
	"fmt"
	"path"
	"path/filepath"

	"github.com/xhd2015/go-inspect/project"
	"github.com/xhd2015/go-inspect/rewrite/session"
	"github.com/xhd2015/go-vendor-pack/writefs"
)

//go:generate bash -ec "cd gen_pack && bash gen.sh"

func Use() {
	project.OnProjectRewrite(func(proj session.Project) project.Rewriter {
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
func (c *rewritter) BeforeLoad(proj session.Project, session session.Session) {
	session.Options().SetRewriteStd(true)
}

// BeforeLoad implements project.Rewriter
func (c *rewritter) AfterLoad(proj session.Project, session session.Session) {
	// unpack getg
	err := session.ImportPackedModulesBase64(GETG_PACK)
	if err != nil {
		panic(fmt.Errorf("import getg: %w", err))
	}
}

// GenOverlay implements project.Rewriter
func (c *rewritter) GenOverlay(proj session.Project, session session.Session) {
	g := proj.Global()

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
	session.SetRewriteFile(path.Join(pkgDir, "export_g_runtime_impl.go"), `package init_getg

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

func removeGetgErrMsg(proj session.Project, session session.Session) {
	var errMsgFileBase string
	if proj.IsVendor() {
		errMsgFileBase = filepath.Join(session.Dirs().RewriteProjectRoot(), "vendor")
	} else {
		errMsgFileBase = session.Dirs().RewriteProjectVendorRoot()
	}
	errMsgFile := filepath.Join(errMsgFileBase, "github.com/xhd2015/go-inspect/plugin/getg/err_msg.go")
	_, statErr := session.RewriteFS().Stat(errMsgFile)
	if statErr != nil {
		if writefs.IsNotExist(statErr) {
			return
		}
		panic(fmt.Errorf("stat getg err_msg file: %w", statErr))
	}
	err := writefs.WriteFile(session.RewriteFS(), errMsgFile, []byte("package getg"))
	if err != nil {
		panic(err)
	}
}
