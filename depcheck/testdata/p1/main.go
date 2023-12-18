package main

import (
	"fmt"
	_ "p1/a"
	_ "p1/b"
)

func init() {
	fmt.Println("main init")
}
func main() {
	fmt.Println("main func")
}
