# About

This package provides a high level project-level rewriting interface, as compared to the [rewrite](../rewrite) package

# Usage

This is an exmaple:

```go
package run

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"

	"github.com/xhd2015/go-inspect/inspect"
	"github.com/xhd2015/go-inspect/project"
)

func Rewrite(loadArgs []string, opts *project.BuildOpts) {
	projectDir := opts.ProjectDir

	agentPkgNameAsWritten := "agent"
	var agentPkg string
	var agentDir string

	var interceptorPkgName string
	var interceptorPkg string
	var interceptorPkgDir string

	var featureEnable bool
	var missingFeatureMsg string
	var overlayFeatureSupportPkg func(targetDir string) error
	initRewrite := func(proj project.Project) {
		g := proj.Global()
		mainPkg := proj.MainPkg()

		// ensure agent package
		agentDirName := proj.AllocExtraPkg(agentPkgNameAsWritten)
		agentPkg = path.Join(mainPkg.Path(), agentDirName)
		agentDir = path.Join(mainPkg.Dir(), agentDirName)

		// detect interceptor
		featureEnable, missingFeatureMsg, overlayFeatureSupportPkg = ext.DetectSpecialDep(g, projectDir)
		if featureEnable {
			interceptorPkgName = proj.AllocExtraPkg("agent_interceptor")
			interceptorPkg = path.Join(mainPkg.Path(), interceptorPkgName)
			interceptorPkgDir = path.Join(mainPkg.Dir(), interceptorPkgName)
		}
	}

	genOverlay := func(proj project.Project, session inspect.Session) {
		if featureEnable && overlayFeatureSupportPkg != nil {
			featureErr := overlayFeatureSupportPkg(proj.RewriteProjectRoot())
			if featureErr != nil {
				panic(featureErr)
			}
		}
		g := proj.Global()
		mainPkg := proj.MainPkg()
		mainPkgEdit := session.PackageEdit(mainPkg, "init_agent")
		mainPkgEdit.MustImport(agentPkg, "agent", "_", nil)
		if featureEnable {
			proj.NewFile(path.Join(interceptorPkgDir, "feature_interceptor.go"), feature.BRIGE_CODE)
		}

		for file, content := range agent_rewrite.CodeMap {
			proj.NewFile(path.Join(agentDir, file), content)
		}
		varMap := genVarMap(g, projectDir, loadArgs, opts.GoFlags, featureEnable, missingFeatureMsg)
		if progArgs.Verbose {
			log.Printf("execute var map: %v", varMap)
		}
		paramsCode := agent_rewrite.PlaceholderTemplate
		for k, v := range varMap {
			paramsCode = strings.ReplaceAll(paramsCode, k, v)
		}

		proj.NewFile(path.Join(agentDir, "params.go"), paramsCode)
	}

	featureBridge := strconv.Quote(ext.FeatureImplPkg)
	rewriteFile := func(proj project.Project, f inspect.FileContext, session inspect.Session) {
		// debug
		// name := f.AST().Name.Name
		// _ = name
		h := proj.ShortHashFile(f)

		if featureEnable && proj.HasImportPkg(f.AST(), featureBridge) {
			// TODO: add shortcut: MustImportUnnamed(pkgName) for anaymous import
			session.FileRewrite(f).MustImport(interceptorPkg, interceptorPkgName, "_", nil)
		}
		agent_rewrite.RewriteFile(session, f.AST(), &agent_rewrite.Options{
			EnableLabel:    featureEnable,
			VarSuffix:      h,
			RegPkgPath:     agentPkg,
			RegPkgName:     agentPkgNameAsWritten,
			TLSPkg:         tlsPkg,
			TLSPkgName:     tlsPkgName,
			TLSLabelFunc:   "Count",
			ConcurrentSafe: progArgs.SafeLabelMap,
		})
	}

	project.Rewrite(loadArgs, &project.RewriteOpts{
		BuildOpts:   opts,
		RewriteName: "my-test",
		Init:        initRewrite,
		GenOverlay:  genOverlay,
		RewriteFile: rewriteFile,
	})
}
```
