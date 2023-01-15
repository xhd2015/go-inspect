package load

import (
	"golang.org/x/tools/go/packages"
)

// packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedTypesSizes | packages.NeedSyntax | packages.NeedDeps | packages.NeedImports | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedModule

var LoadModeSyntax = []packages.LoadMode{
	packages.NeedName,
	// packages.NeedFiles, // fill the GoFiles fields
	// packages.NeedCompiledGoFiles,
	// packages.NeedTypesSizes,
	packages.NeedSyntax,
	// packages.NeedDeps,
	packages.NeedImports, // required when you need to do MustImports, this will ensure an initial import list is correctly created
	// packages.NeedTypes,
	// packages.NeedTypesInfo,
	packages.NeedModule,
}
