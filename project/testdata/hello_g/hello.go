package main

import (
	"fmt"

	"github.com/xhd2015/go-inspect/project/testdata/export_g"
)

func main() {
	x := export_g.Get()

	fmt.Printf("x: %v\n", x)
	// Output:
	//  x: 0xc0000001a0
}
