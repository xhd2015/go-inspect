package main

import (
	"fmt"

	"github.com/xhd2015/go-inspect/project/testdata/extra_pkg/extra"
)

func main() {
	hello := extra.HelloExtra()
	fmt.Printf("%s\n", hello)
}
