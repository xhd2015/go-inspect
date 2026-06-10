# global-nil-module-handling

Validates that `NewGlobal` does not panic when encountering a vendored package
whose source files exist in `vendor/` but are NOT listed in `vendor/modules.txt`.

## Test Cases

| # | Test Case | Description |
|---|-----------|-------------|
| 1 | `load-unlisted-vendor-pkg` | Load a project with a vendored sub-package not listed in vendor/modules.txt — module should be inferred from parent module |

## Decision Tree

```
global-nil-module-handling/
├── README.md
├── SETUP.md
├── testdata/
│   └── vendor-unlisted/
│       ├── go.mod
│       ├── main.go
│       └── vendor/
│           ├── modules.txt       # lists "pkg/listed" but NOT "pkg/unlisted"
│           └── example.com/module/
│               └── pkg/
│                   ├── listed/   # LISTED, imports unlisted
│                   └── unlisted/ # UNLISTED → triggers nil Module
└── load-unlisted-vendor-pkg/
    ├── SETUP.md
    └── ASSERT.md
```
