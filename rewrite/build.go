package rewrite

import (
	"fmt"
	"log"
	"path"
	"strings"

	"github.com/xhd2015/go-inspect/inspect/util"
	"github.com/xhd2015/go-inspect/sh"
)

func Build(args []string, opts *BuildOptions) (result *BuildResult, err error) {
	return build(args, opts)
}

func BuildRewrite(args []string, ctrl Controller, rewritter Visitor, opts *BuildRewriteOptions) (*BuildResult, error) {
	return buildRewrite(args, ctrl, rewritter, opts)
}

type BuildResult struct {
	Output string
}

func buildRewrite(args []string, ctrl Controller, rewritter Visitor, opts *BuildRewriteOptions) (*BuildResult, error) {
	if opts == nil {
		opts = &BuildRewriteOptions{}
	}

	rewriteRoot := opts.RebaseRoot
	if rewriteRoot == "" {
		rewriteRoot = GetTmpRewriteRoot("go-rewrite")
	}
	res, err := GenRewrite(args, rewriteRoot, ctrl, rewritter, opts)
	if err != nil {
		panic(err)
	}
	if opts.SkipBuild {
		return &BuildResult{
			Output: "skipped",
		}, nil
	}
	buildOpts := &BuildOptions{
		Verbose:     opts.Verbose,
		ProjectRoot: opts.ProjectDir,
		RebaseRoot:  rewriteRoot,
		MappedMod:   res.MappedMod,
		NewGoROOT:   res.UseNewGOROOT,
		Debug:       opts.Debug,
		Output:      opts.Output,
		ForTest:     opts.ForTest,
		GoFlags:     opts.BuildFlags,
	}
	return build(args, buildOpts)
}

func build(args []string, opts *BuildOptions) (result *BuildResult, err error) {
	if opts == nil {
		opts = &BuildOptions{}
	}
	verbose := opts.Verbose
	debug := opts.Debug
	mappedMod := opts.MappedMod
	newGoROOT := opts.NewGoROOT
	forTest := opts.ForTest
	goFlags := opts.GoFlags
	// project root
	projectRoot, err := util.ToAbsPath(opts.ProjectRoot)
	if err != nil {
		return
	}
	// output
	output := ""
	if opts != nil && opts.Output != "" {
		output = opts.Output
		if !path.IsAbs(output) {
			output, err = util.ToAbsPath(output)
			if err != nil {
				err = fmt.Errorf("make abs path err:%v", err)
				return
			}
		}
	} else {
		output = "exec"
		if debug {
			output = "debug"
		}
		if forTest {
			output = output + "-test"
		}
		output = output + ".bin"
		if !path.IsAbs(output) {
			output = path.Join(projectRoot, output)
		}
	}

	var gcflagList []string

	// rebaseRoot dir is errous:
	//     /path/to/rewrite-rebaseRoot=>/
	//     //Users/x/gopath/pkg/mod/github.com/xhd2015/go-inspect/v1/src/db/impl/util.go
	//
	// so replacement must have at least one child:
	//     /path/to/rewrite-rebaseRoot/X=>/X
	var rebaseRoot string
	if opts.RebaseRoot != "" {
		rebaseRoot, err = util.ToAbsPath(opts.RebaseRoot)
		if err != nil {
			err = fmt.Errorf("get absolute path failed:%v %v", opts.RebaseRoot, err)
			return
		}
	}
	if debug {
		gcflagList = append(gcflagList, "-N", "-l")
	}
	fmtTrimPath := func(from, to string) string {
		if to == "" {
			// cannot be empty, dlv does not support relative path
			panic(fmt.Errorf("trimPath to must not be empty:%v", from))
		}
		if to == "/" {
			log.Printf("WARNING trim path found / replacement, should contains at least one child:from=%v, to=%v", from, to)
		}
		return fmt.Sprintf("%s=>%s", from, to)
	}
	workDir := projectRoot
	if rebaseRoot != "" {
		workDir = path.Join(rebaseRoot, projectRoot)
		trimList := []string{fmtTrimPath(workDir, projectRoot)}
		for origAbsDir, cleanedAbsDir := range mappedMod {
			trimList = append(trimList, fmtTrimPath(path.Join(rebaseRoot, cleanedAbsDir), origAbsDir))
		}
		gcflagList = append(gcflagList, fmt.Sprintf("-trimpath=%s", strings.Join(trimList, ";")))
	}
	outputFlags := ""
	if output != "" {
		outputFlags = fmt.Sprintf(`-o %s`, sh.Quote(output))
	}
	gcflags := " "
	if len(gcflagList) > 0 {
		gcflags = "-gcflags=all=" + sh.Quotes(gcflagList...)
	}

	// NOTE: can only specify -gcflags once, the last flag wins.
	// example:
	//     MOD=$(go list -m);W=${workspaceFolder};R=/var/folders/y8/kmfy7f8s5bb5qfsp0z8h7j5m0000gq/T/go-rewrite;D=$R$W;cd $D;DP=$(cd $D;cd ..;pwd); with-go1.14 go build -gcflags="all=-N -l -trimpath=/var/folders/y8/kmfy7f8s5bb5qfsp0z8h7j5m0000gq/T/go-rewrite/Users/xhd2015/Projects/gopath/src/github.com/xhd2015/go-inspect=>/Users/xhd2015/Projects/gopath/src/github.com/xhd2015/go-inspect" -o /tmp/xgo/${workspaceFolderBasename}/inspect_rewrite.with_go_mod.bin ./support/xgo/inspect/testdata/inspect_rewrite.go
	cmdList := []string{
		"set -e",
		// fmt.Sprintf("REWRITE_ROOT=%s", quote(root)),
		// fmt.Sprintf("PROJECT_ROOT=%s", quote(projectRoot)),
		fmt.Sprintf("cd %s", sh.Quote(workDir)),
	}
	if newGoROOT != "" {
		cmdList = append(cmdList, fmt.Sprintf("export GOROOT=%s", sh.Quote(path.Join(rebaseRoot, newGoROOT))))
	}
	buildCmd := "build"
	if forTest {
		buildCmd = "test -c"
	}
	goFlagsSpace := ""
	if len(goFlags) > 0 {
		goFlagsSpace = " " + sh.Quotes(goFlags...)
	}
	cmdList = append(cmdList, fmt.Sprintf(`go %s %s %s%s %s`, buildCmd, outputFlags, sh.Quote(gcflags), goFlagsSpace, sh.JoinArgs(args)))

	_, _, err = sh.RunBashWithOpts(cmdList, sh.RunBashOptions{
		Verbose: verbose,
	})
	if err != nil {
		log.Printf("build %s failed", output)
		err = fmt.Errorf("build %s err:%v", output, err)
		return
	}

	if verbose {
		log.Printf("build successful: %s", output)
	}

	result = &BuildResult{
		Output: output,
	}
	return
}
