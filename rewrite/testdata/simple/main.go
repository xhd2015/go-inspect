package main

import (
	"fmt"
	"math/rand"
)

func main() {
	printHelloOrWorld()
}

func printHelloOrWorld() {
	if rand.Intn(10) > 5 {
		fmt.Printf("hello\n")
		return
	}
	fmt.Printf("world\n")
}
