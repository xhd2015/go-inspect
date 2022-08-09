package cmdsupport

import (
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/xhd2015/go-inspect/inspect"
	"github.com/xhd2015/go-inspect/inspect/load"
	"github.com/xhd2015/go-inspect/inspect/util"
	api "github.com/xhd2015/go-inspect/inspect2"
)

type LoadOptions struct {
	LoadArgs []string // passed to packages.Load

	ForTest bool
}

func PrintRewrite(file string, printRewrite bool, printMock bool, loadOpts *LoadOptions, opts *inspect.RewriteOptions) {
	if file == "" {
		panic(fmt.Errorf("requires file"))
	}
	if !strings.HasSuffix(file, ".go") {
		panic(fmt.Errorf("requires go file"))
	}
	absFile, err := util.ToAbsPath(file)
	if err != nil {
		panic(fmt.Errorf("make file absolute error:%v", err))
	}
	stat, err := os.Stat(absFile)
	if err != nil {
		panic(fmt.Errorf("file does not exist:%v %v", file, err))
	}
	if stat.IsDir() {
		panic(fmt.Errorf("path is a directory, expecting a file:%v", file))
	}
	if loadOpts == nil {
		loadOpts = &LoadOptions{}
	}
	projectDir := util.FindProjectDir(absFile)

	rel, ok := util.RelPath(projectDir, absFile)
	if !ok {
		panic(fmt.Errorf("%s not child of module:%s", absFile, projectDir))
	}

	loadPkg := "./" + strings.TrimPrefix(path.Dir(rel), "./")
	g, err := load.LoadPackages([]string{loadPkg}, &load.LoadOptions{
		ProjectDir: projectDir,
		ForTest:    loadOpts.ForTest,
		BuildFlags: loadOpts.LoadArgs,
	})
	if err != nil {
		panic(fmt.Errorf("loading packages error:%v", err))
	}

	rw := inspect.NewMockRewritter(opts)
	session := api.NewSession(g)
	api.VisitAll(func(f func(pkg api.Pkg) bool) {
		for _, p := range g.LoadInfo().StarterPkgs() {
			if !f(p) {
				return
			}
		}
	}, session, rw)

	var rewriteContent string
	var mockContent string
	session.Gen(&api.EditCallbackFn{
		Rewrites: func(f api.FileContext, content string) bool {
			if f.AbsPath() == absFile {
				rewriteContent = content
			}
			return true
		},
		Pkg: func(p api.Pkg, kind, realName, content string) bool {
			mockContent = content
			return true
		},
	})
	if rewriteContent == "" {
		fmt.Fprintf(os.Stderr, "no content\n")
		return
	}

	if printRewrite {
		if printMock {
			fmt.Printf("// rewrite of %s:\n", absFile)
		}
		fmt.Print(string(rewriteContent))
	}

	if printMock {
		if printRewrite {
			fmt.Printf("//\n// mock of %s:\n", absFile)
		}
		fmt.Print(string(mockContent))
	}
}
