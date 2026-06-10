## Expected
- `LoadPackages` panics with a helpful message about `vendor/modules.txt`
- The panic message includes the package path and a suggestion to run `go mod vendor`

## Errors
- Panic message contains "non-test pkg"
- Panic message contains "vendor/modules.txt"
- Panic message contains "go mod vendor"

```go
import "strings"

func Assert(t *testing.T, req *Request, resp *Response, err error) {
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Panicked {
		t.Fatal("expected a panic, but no panic occurred")
	}
	if !strings.Contains(resp.ErrorMsg, "non-test pkg") {
		t.Fatalf("expected panic message to contain 'non-test pkg', got: %s", resp.ErrorMsg)
	}
	if !strings.Contains(resp.ErrorMsg, "vendor/modules.txt") {
		t.Fatalf("expected panic message to contain 'vendor/modules.txt', got: %s", resp.ErrorMsg)
	}
	if !strings.Contains(resp.ErrorMsg, "go mod vendor") {
		t.Fatalf("expected panic message to suggest 'go mod vendor', got: %s", resp.ErrorMsg)
	}
}
```
