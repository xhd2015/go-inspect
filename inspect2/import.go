package inspect

import (
	"errors"
	"fmt"

	"github.com/xhd2015/go-inspect/inspect/util"
)

// TODO: handle "." case
type ImportListContext interface {
	// MustImport make a package foribly imported
	// because name may be hidden, so alias is important
	// a package can reimported multiple times,
	// if every import introduces a new name
	// if name is the same with use, alias can be ignored.
	// NOTE: all names must be consistent
	MustImport(pkgPath string, name string, suggestAlias string, forbidden func(name string) bool) string
}

type pkgPath = string
type pkgName = string
type importList struct {
	edit      Edit
	pkgToName map[pkgPath]pkgName         // this provides the default name of the package. It must be unique.
	importMap map[pkgPath]map[string]bool // pkgPath to their alias
	useMap    map[string]pkgPath          // all uses

	// callback
	addImport func(use string, pkg string)

	// newImports [][2]string // {use,path}
}

var _ ImportListContext = ((*importList)(nil))

func NewImportList_X(initImports func(fn func(pkg string, name string, alias string)), addImport func(use string, pkg string)) ImportListContext {
	// if !insertPos.IsValid() {
	// 	panic(fmt.Errorf("insert pos invalid"))
	// }
	nameMap := make(map[string]string)
	importMap := make(map[string]map[string]bool)
	useMap := make(map[string]string)
	// importMap := make
	c := &importList{
		pkgToName: nameMap,
		importMap: importMap,
		useMap:    useMap,
		addImport: addImport,
	}
	if initImports != nil {
		initImports(func(pkg string, name string, alias string) {
			c.mustCheckName(pkg, name)
			pkgUseMap := importMap[pkg]
			if pkgUseMap == nil {
				pkgUseMap = make(map[string]bool)
				importMap[pkg] = pkgUseMap
			}
			if pkgUseMap[alias] {
				panic(fmt.Errorf("duplicate import %q", pkg))
			}
			pkgUseMap[alias] = true

			realUse := name
			if alias != "" {
				realUse = alias
			}
			useMap[realUse] = pkg
		})
	}
	return c
}

// MustImport
// return the effective name
// ImportOrUseNext will always succeed.
// It do extra work to ensure that only one effective name exists in the list.
// This involves rewritting
// This makes a pkg path has only one name.
func (c *importList) MustImport(pkgPath string, name string, suggestAlias string, forbidden func(name string) bool) string {
	c.checkName(pkgPath, name)
	use := suggestAlias
	if suggestAlias == "" {
		use = name
	}

	aliasMap := c.importMap[pkgPath]
	if aliasMap == nil {
		aliasMap = make(map[string]bool, 1)
		c.importMap[pkgPath] = aliasMap
	}

	// either forbidden or not imported
	use = util.NextName(func(s string) bool {
		if forbidden != nil && forbidden(s) {
			return false
		}
		// not forbidden, not previous or previous is pkgPath
		prev := c.useMap[s]
		return prev == "" || prev == pkgPath
	}, use)

	if aliasMap[use] || (use == name && aliasMap[""]) {
		return use
	}
	aliasMap[use] = true
	c.useMap[use] = pkgPath

	if c.addImport != nil {
		useAlias := use
		if useAlias == name {
			useAlias = ""
		}
		c.addImport(useAlias, pkgPath)
	}

	return use
}

func (c *importList) checkName(pkgPath, name string) error {
	if name == "" {
		return errors.New("name cannot be empty")
	}
	if pkgPath == "" {
		return fmt.Errorf("pkgPath cannot be empty")
	}
	prevName := c.pkgToName[pkgPath]
	if prevName == "" {
		c.pkgToName[pkgPath] = name
	} else if prevName != name {
		return fmt.Errorf("inconsistent name of package:%v given:%v, previous:%v", pkgPath, name, prevName)
	}
	return nil
}
func (c *importList) mustCheckName(pkgPath, name string) {
	err := c.checkName(pkgPath, name)
	if err != nil {
		panic(err)
	}
}
