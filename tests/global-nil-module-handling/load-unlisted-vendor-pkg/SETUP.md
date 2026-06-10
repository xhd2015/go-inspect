## Preconditions
- `testdata/vendor-unlisted/` contains a Go module project with a vendored dependency
- The vendored module `example.com/module` has sub-package `pkg/unlisted` that
  is NOT listed in `vendor/modules.txt`
- Sub-package `pkg/listed` IS listed in `vendor/modules.txt` and imports `pkg/unlisted`

## Steps
- Set `req.InputDir` to the `testdata/vendor-unlisted/` fixture directory

## Context
- The unlisted `pkg/unlisted` package causes a panic in `NewGlobal` because
  `packages.Load` returns it with `Module == nil`
- The panic message guides users to run `go mod vendor` to fix the
  `vendor/modules.txt` inconsistency

```go
import "path/filepath"

func Setup(t *testing.T, req *Request) error {
	req.InputDir = filepath.Join(DOCTEST_ROOT, "testdata", "vendor-unlisted")
	return nil
}
```
