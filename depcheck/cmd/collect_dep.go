package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/token"
	"io/ioutil"
	"log"
	"strings"

	"github.com/xhd2015/go-inspect/depcheck"
	"golang.org/x/tools/go/packages"
)

var output = flag.String("o", "", "output, default standard")
var test = flag.Bool("test", false, "test mode")
var mod = flag.String("mod", "", "test mode")

var check = flag.String("check", "", "check with pkg")
var pretty = flag.Bool("pretty", false, "pretty json or output")

func main() {
	err := run()
	if err != nil {
		log.Fatalf("%v", err)
	}
}
func run() error {
	flag.Parse()
	args := flag.Args()
	var buildFlags []string
	if *mod != "" {
		buildFlags = append(buildFlags, "-mod="+*mod)
	}
	fset := token.NewFileSet()
	cfg := &packages.Config{
		// Dir:        absDir,
		Mode:       packages.NeedDeps | packages.NeedName | packages.NeedSyntax | packages.NeedImports,
		Fset:       fset,
		Tests:      *test,
		BuildFlags: buildFlags,
	}
	pkgs, err := packages.Load(cfg, args...)
	if err != nil {
		return err
	}
	deps, pkgMapping, err := depcheck.CollectDeps(pkgs, &depcheck.CollectOptions{
		NeedDependedBy: false,
	})
	if err != nil {
		return err
	}

	if *check != "" {
		dep := pkgMapping[*check]
		if dep == nil {
			return fmt.Errorf("package not found with -check=%s", *check)
		}
		importTrace := depcheck.GetImportTrace(deps, *check)

		for i, v := range importTrace {
			if *pretty {
				fmt.Print(strings.Repeat(" ", i*2))
			}
			fmt.Println(v)
		}
		return nil
	}
	var depsJSON []byte
	if *pretty {
		depsJSON, err = json.MarshalIndent(deps, "", "    ")
	} else {
		depsJSON, err = json.Marshal(deps)
	}
	if err != nil {
		return err
	}
	if *output != "" {
		return ioutil.WriteFile(*output, depsJSON, 0755)
	} else {
		fmt.Println(string(depsJSON))
	}
	return nil
}
