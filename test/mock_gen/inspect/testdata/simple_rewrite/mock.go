// Code generated by go-mock; DO NOT EDIT.

package print

import (
    "context"
    "github.com/xhd2015/go-inspect/inspect/testdata/simple_rewrite"
    _mock "github.com/xhd2015/go-inspect/inspect/mock"
)

const _SKIP_MOCK = true
const FULL_PKG_NAME = "github.com/xhd2015/go-inspect/inspect/testdata/simple_rewrite"

func Setup(ctx context.Context,setup func(m *M)) context.Context {
    m:=M{}
    setup(&m)
    return _mock.WithMockSetup(ctx,FULL_PKG_NAME,m)
}


type M struct {
    Run func(ctx context.Context, status int, _ string)(int, error)
    Status struct{
            Run func(c print.Status,ctx context.Context, status int, _ string)(int, error)
    }
}

/* prodives quick link */
var _ = func() { type Pair [2]interface{};e:=M{};_ = map[string]interface{}{
    "Run": Pair{e.Run,print.Run},
    "Status":map[string]interface{}{
             "Run": Pair{e.Status.Run,((*print.Status)(nil)).Run},
    },
}}