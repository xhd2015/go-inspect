package load

import (
	"fmt"
	"go/token"
	"path"
	"regexp"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/xhd2015/go-inspect/inspect/util"
	inspect "github.com/xhd2015/go-inspect/inspect2"
)

type LoadOptions struct {
	ProjectDir string
	ForTest    bool
	BuildFlags []string // see FlagBuilder
}

func LoadPackages(args []string, opts *LoadOptions) (inspect.Global, error) {
	if opts == nil {
		opts = &LoadOptions{}
	}
	fset := token.NewFileSet()
	dir := opts.ProjectDir
	absDir, err := util.ToAbsPath(dir)
	if err != nil {
		return nil, err
	}
	cfg := &packages.Config{
		Dir:        absDir,
		Mode:       packages.NeedFiles | packages.NeedSyntax | packages.NeedDeps | packages.NeedImports | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedModule,
		Fset:       fset,
		Tests:      opts.ForTest,
		BuildFlags: opts.BuildFlags,
		// BuildFlags: []string{"-a"}, // TODO: confirm what the extra non-gofile from
	}
	pkgs, err := packages.Load(cfg, args...)
	if err != nil {
		return nil, err
	}
	var perr error
	packages.Visit(pkgs, func(p *packages.Package) bool {
		if perr != nil {
			return false
		}
		if len(p.Errors) > 0 {
			perr = fmt.Errorf("loading package error:%v %v", p, p.Errors)
			return false
		}
		normalizePackage(p)
		return true
	}, nil)
	if perr != nil {
		return nil, perr
	}
	return inspect.NewGlobal(fset, absDir, pkgs), nil
}

func normalizePackage(pkg *packages.Package) {
	// normalize pkg path
	pkg.PkgPath = normalizePkgPath(pkg)
	pkg.Name = normalizePkgName(pkg)
	pkg.Module = GetPkgModule(pkg)
}

var lowCase = regexp.MustCompile("^[a-z0-9]+$")

func GetPkgModule(p *packages.Package) *packages.Module {
	if p.Module != nil {
		return p.Module
	}
	// check if it is std module(i.e. src inside $GOROOT)
	pkgPath := p.PkgPath
	idx := strings.Index(pkgPath, "/")
	//  excluding: github.com, xxx.com, golang.x.org...
	isStd := idx < 0 || lowCase.MatchString(pkgPath[:idx])
	if isStd {
		return util.GetStdModule()
	}
	return nil
}

func normalizePkgName(pkg *packages.Package) string {
	name := pkg.Name
	if name == "" {
		name = pkg.Types.Name()
	}
	return name
}

func normalizePkgPath(pkg *packages.Package) string {
	pkgPath := pkg.PkgPath
	if pkgPath == "" {
		pkgPath = pkg.Types.Path()
	}

	// normalize pkgPath
	if pkgPath == "command-line-arguments" {
		if len(pkg.GoFiles) > 0 {
			pkgPath = GetPkgPathOfFile(pkg.Module, path.Dir(pkg.GoFiles[0]))
		}
	}
	return pkgPath
}

// GetPath the returned result is guranteed to not end with "/"
// `filePath` is an absolute path on the filesystem.
func GetPkgPathOfFile(mod *packages.Module, fsPath string) string {
	modPath, modDir := mod.Path, mod.Dir
	if mod.Replace != nil {
		modPath, modDir = mod.Replace.Path, mod.Replace.Dir
	}

	rel, ok := util.RelPath(modDir, fsPath)
	if !ok {
		panic(fmt.Errorf("%s not child of %s", fsPath, modDir))
	}

	return strings.TrimSuffix(path.Join(modPath, rel), "/")
}
