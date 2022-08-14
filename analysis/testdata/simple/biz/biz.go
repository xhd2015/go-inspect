package biz

import (
	"context"
	"fmt"
	"math/rand"
)

func Run(ctx context.Context, status int, _ string) (int, error) {
	fmt.Printf("Run:status = %v\n", status)
	return 0, nil
}

type Status int

func (c Status) Run(ctx context.Context, status int, _ string) (int, error) {
	var x fmt.Stringer
	if rand.Intn(10) > 5 {
		// if false { // analysis will not find if false's constant,so still give
		// a possible report
		// if you delete the following line, myStringer.String() will not be found
		x = myStringer("hello")
	}

	fmt.Printf("Status Run:status = %v\n", status)
	x.String()
	return 0, nil
}

type Caller interface {
}

type myStringer string

func (c myStringer) String() string {
	return "my:" + string(c)
}
