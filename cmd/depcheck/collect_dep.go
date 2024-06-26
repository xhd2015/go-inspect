package main

import (
	"encoding/json"
	"fmt"
	"go/token"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/xhd2015/go-inspect/depcheck"
	"golang.org/x/tools/go/packages"
)

const help = `
depcheck [FLAGS] <args> 

Options:
     -mod=vendor         load with -mod=vendor 
     --project-dir DIR   project dir
  -o OUTPUT              write output to file
	 --check PKG         check specific package
	 --json              output json
	 --depth N           max depth to print 
	 --pretty            pretty json output 
     --version           show version
  -h,--help              show help

Examples:
  depcheck --check some-git.com/pkg/a -mod=vendor ./src
`
const version = "0.0.1"

func main() {
	err := run(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
		log.Fatalf("%v", err)
	}
}

// TODO: distinguish between go version 1.21 and below
// they have different algorithms
func run(args []string) error {
	n := len(args)

	var showVersion bool
	var showHelp bool
	var mod string
	var remainArgs []string
	var checks []string
	var projectDir string
	var test bool
	var output string
	var pretty bool
	var fmtJSON bool

	var maxDepth int
	for i := 0; i < n; i++ {
		arg := args[i]
		if arg == "--" {
			remainArgs = append(remainArgs, args[i+1:]...)
			break
		}
		if arg == "--version" {
			showVersion = true
			break
		}
		if arg == "-h" || arg == "--help" {
			showHelp = true
			break
		}
		if arg == "--test" {
			test = true
			continue
		}
		if arg == "--pretty" {
			pretty = true
			continue
		}
		if arg == "--json" {
			fmtJSON = true
			continue
		}
		if arg == "--depth" {
			if i+1 >= n {
				return fmt.Errorf("--depth requires value")
			}
			d := args[i+1]
			i++
			dv, err := strconv.ParseInt(d, 10, 64)
			if err != nil {
				return fmt.Errorf("--depth: %w", err)
			}
			maxDepth = int(dv)
			continue
		}
		if arg == "-o" {
			if i+1 >= n {
				return fmt.Errorf("-o requires value")
			}
			output = args[i+1]
			i++
			continue
		}
		if strings.HasPrefix(arg, "-o=") {
			output = strings.TrimPrefix(arg, "-o=")
			continue
		}
		if arg == "-mod" {
			if i+1 >= n {
				return fmt.Errorf("-mod requires value")
			}
			mod = args[i+1]
			i++
			continue
		}
		if strings.HasPrefix(arg, "-mod=") {
			mod = strings.TrimPrefix(arg, "-mod=")
			continue
		}
		if arg == "--check" {
			if i+1 >= n {
				return fmt.Errorf("-check requires value")
			}
			checks = append(checks, args[i+1])
			i++
			continue
		}
		if strings.HasPrefix(arg, "--check=") {
			checks = append(checks, strings.TrimPrefix(arg, "--check="))
			i++
			continue
		}
		if arg == "--project-dir" {
			if i+1 >= n {
				return fmt.Errorf("--project-dir requires value")
			}
			projectDir = args[i+1]
			i++
			continue
		}
		if strings.HasPrefix(arg, "--project-dir=") {
			projectDir = strings.TrimPrefix(arg, "--project-dir=")
			continue
		}
		if !strings.HasPrefix(arg, "-") {
			remainArgs = append(remainArgs, arg)
			continue
		}
		return fmt.Errorf("unrecognized flag: %v", arg)
	}
	if showVersion {
		fmt.Println(version)
		return nil
	}
	if showHelp {
		fmt.Println(strings.TrimPrefix(help, "\n"))
		return nil
	}
	var buildFlags []string
	if mod != "" {
		buildFlags = append(buildFlags, "-mod="+mod)
	}
	fset := token.NewFileSet()
	cfg := &packages.Config{
		Dir: projectDir,
		// to have syntax, must also set NeedFiles
		Mode:       packages.NeedDeps | packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedImports,
		Fset:       fset,
		Tests:      test,
		BuildFlags: buildFlags,
	}
	pkgs, err := packages.Load(cfg, remainArgs...)
	if err != nil {
		return err
	}
	deps, pkgMapping, err := depcheck.CollectDeps(pkgs, &depcheck.CollectOptions{
		NeedDependedBy: false,
	})

	if err != nil {
		return err
	}
	if len(checks) > 0 {
		for _, check := range checks {
			if pkgMapping[check] == nil {
				return fmt.Errorf("pkg %s is not a dependency", check)
			}
		}
		deps = depcheck.FilterDeps(deps, checks)
	}
	if maxDepth > 0 {
		deps = depcheck.LimitDepths(deps, maxDepth)
	}
	var outputData []byte
	if fmtJSON {
		if pretty {
			outputData, err = json.MarshalIndent(deps, "", "    ")
		} else {
			outputData, err = json.Marshal(deps)
		}
	} else {
		outputData = []byte(depcheck.FormatDepTraces(deps))
	}

	if err != nil {
		return err
	}
	if output != "" {
		return ioutil.WriteFile(output, outputData, 0755)
	} else {
		fmt.Println(string(outputData))
	}
	return nil
}
