package cmd

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"

	"github.com/xhd2015/go-inspect/rewrite"
	"github.com/xhd2015/go-inspect/sh"
)

func GetRewriteRoot() string {
	// return path.Join(os.MkdirTemp(, "go-rewrite")
	return path.Join(os.TempDir(), "go-rewrite")
}

func BuildRewrite(args []string, genOpts *GenRewriteOptions, opts *rewrite.BuildOptions) (*rewrite.BuildResult, error) {
	if opts == nil {
		opts = &rewrite.BuildOptions{}
	}
	verbose := opts.Verbose
	if genOpts == nil {
		genOpts = &GenRewriteOptions{
			Verbose: verbose,
		}
	}
	genOpts.ProjectDir = opts.ProjectRoot

	rewriteRoot := GetRewriteRoot()
	res, err := GenRewrite(args, rewriteRoot, genOpts)
	if err != nil {
		panic(err)
	}
	opts.RebaseRoot = rewriteRoot
	opts.MappedMod = res.MappedMod
	opts.NewGoROOT = res.UseNewGOROOT
	return rewrite.Build(args, opts)
}

var Quotes = sh.Quotes
var Quote = sh.Quote

// we are actually creating overlay, so CopyDirs can be ignored.

type CopyOpts struct {
	Verbose     bool
	IgnoreNames []string // dirs should be ignored from srcDirs. Still to be supported
	ProcessDest func(name string) string
}

// CopyDirs
// TODO: may use hard link or soft link instead of copy
func CopyDirs(srcDirs []string, destRoot string, opts CopyOpts) error {
	if len(srcDirs) == 0 {
		return fmt.Errorf("CopyDirs empty srcDirs")
	}
	for i, srcDir := range srcDirs {
		if srcDir == "" {
			return fmt.Errorf("srcDirs contains empty dir:%v at %d", srcDirs, i)
		}
	}
	if destRoot == "" {
		return fmt.Errorf("CopyDirs no destRoot")
	}
	if destRoot == "/" {
		return fmt.Errorf("destRoot cannot be /")
	}

	ignoreMap := make(map[string]bool, len(opts.IgnoreNames))
	for _, ignore := range opts.IgnoreNames {
		ignoreMap[ignore] = true
	}

	// try our best to ignore level-1 files
	files := make([][]string, 0, len(srcDirs))
	for _, srcDir := range srcDirs {
		dirFiles, err := ioutil.ReadDir(srcDir)
		if err != nil {
			return fmt.Errorf("list file of %s error:%v", srcDir, err)
		}
		dirFileNames := make([]string, 0, len(dirFiles))
		for _, f := range dirFiles {
			if ignoreMap[f.Name()] || (!f.IsDir() && f.Size() > 10*1024*1024 /* >10M */) {
				continue
			}
			dirFileNames = append(dirFileNames, f.Name())
		}
		files = append(files, dirFileNames)
	}

	cmdList := make([]string, 0, len(srcDirs))
	cmdList = append(cmdList,
		"set -e",
		fmt.Sprintf("rm -rf %s", Quote(destRoot)),
	)
	for i, srcDir := range srcDirs {
		srcFiles := files[i]
		if len(srcFiles) == 0 {
			continue
		}

		dstDir := path.Join(destRoot, srcDir)
		if opts.ProcessDest != nil {
			dstDir = opts.ProcessDest(dstDir)
			if dstDir == "" {
				continue
			}
		}
		qsrcDir := Quote(srcDir)
		qdstDir := Quote(dstDir)

		cmdList = append(cmdList, fmt.Sprintf("rm -rf %s && mkdir -p %s", qdstDir, qdstDir))
		for _, srcFile := range srcFiles {
			qsrcFile := Quote(srcFile)
			cmdList = append(cmdList, fmt.Sprintf("cp -R %s/%s %s/%s", qsrcDir, qsrcFile, qdstDir, qsrcFile))
		}
		cmdList = append(cmdList, fmt.Sprintf("chmod -R 0777 %s", qdstDir))
	}
	if opts.Verbose {
		log.Printf("copying dirs:%v", srcDirs)
	}
	return sh.RunBash(
		cmdList,
		opts.Verbose,
	)
}
