package analysis

import (
	"encoding/json"
	"io/ioutil"
	"testing"

	"github.com/xhd2015/go-inspect/inspect/load"
)

// go test -run TestLoadCallGraph -v ./analysis/
func TestLoadCallGraph(t *testing.T) {
	g, res, err := LoadCallGraph([]string{"./testdata/simple"}, &load.LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	resJSON, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	ioutil.WriteFile("./testdata/simple/callgraph.tmp.json", resJSON, 0777)
	_ = g
}
