package inspect

import (
	"testing"

	"github.com/xhd2015/go-inspect/inspect/util"
)

// go test -run TestHasPrefixSplit -v ./support/xgo/inspect
func TestHasPrefixSplit(t *testing.T) {
	v := util.HasPrefixSplit("test/aka", "test", '/')

	t.Logf("%v", v)
}
