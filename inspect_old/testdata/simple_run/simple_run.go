package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/xhd2015/go-inspect/inspect/testdata/simple_run/biz"
	"github.com/xhd2015/go-inspect/mock"
)

func main() {
	stubs, err := json.Marshal(mock.GetMockStubs())
	if err != nil {
		panic(err)
	}
	fmt.Printf("stubs:%v\n", string(stubs))
	ctx := context.Background()
	biz.Run(ctx, 1, "s")
}
