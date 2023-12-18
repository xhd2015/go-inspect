package main

import (
	"fmt"
	_ "p2/b"
	_ "p2/a"
)

func init() {
	fmt.Println("main init")
}
func main() {
	fmt.Println("main func")
}
