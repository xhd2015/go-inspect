## Preconditions
- The go-inspect module is the parent module of this test tree
- `load.LoadPackages` is importable as `github.com/xhd2015/go-inspect/inspect/load`
- `inspect.Pkg` and `inspect.Global` are importable as `github.com/xhd2015/go-inspect/inspect`

## Overview
This test tree validates that `NewGlobal` panics with a helpful message when
encountering a vendored package whose source files exist in `vendor/` but are
NOT listed in `vendor/modules.txt`. The panic message guides the user to run
`go mod vendor` to fix the inconsistency.

## Context
- Go 1.19 with `-mod=vendor` is the default behavior when `vendor/` exists
- This test requires Go 1.19 (skips on other versions due to dependency compatibility)

```go
import (
	"fmt"
	"runtime"
	"strings"

	"github.com/xhd2015/go-inspect/inspect/load"
)

type Request struct {
	InputDir string
}

type Response struct {
	Panicked bool
	ErrorMsg string
}

func Run(t *testing.T, req *Request) (*Response, error) {
	if !strings.HasPrefix(runtime.Version(), "go1.19.") {
		t.Skipf("test requires Go 1.19, current: %s", runtime.Version())
	}

	var panicked bool
	var panicMsg string

	func() {
		defer func() {
			if e := recover(); e != nil {
				panicked = true
				panicMsg = fmt.Sprintf("%v", e)
			}
		}()

		_, err := load.LoadPackages([]string{"."}, &load.LoadOptions{
			ProjectDir: req.InputDir,
			ForTest:    false,
		})
		if err != nil {
			panic(fmt.Errorf("LoadPackages returned error: %w", err))
		}
	}()

	return &Response{
		Panicked: panicked,
		ErrorMsg: panicMsg,
	}, nil
}
```
