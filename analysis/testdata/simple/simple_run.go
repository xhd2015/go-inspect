package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/xhd2015/go-inspect/analysis/testdata/simple/biz"
)

func main() {
	stubs, err := json.Marshal([]int{1, 2, 3})
	if err != nil {
		panic(err)
	}
	fmt.Printf("stubs:%v\n", string(stubs))
	ctx := context.Background()
	biz.Run(ctx, 1, "s")

	b1 := biz.Status(1)
	b1.Run(ctx, 0, "unused")
}
