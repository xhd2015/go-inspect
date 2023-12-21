# getg

Export the `runtime.getg()` which is unexported by default.

This makes goroutine local storage easier to implement, like in testing environment.

Don't use this in production.

# How rewrite of standard lib works?

See [project/rewrite_std_test.go](project/rewrite_std_test.go) for example,basically:

- after loading and AST inspecting, before copying files, set `rewriteStd` to true
- in `GenOverlay` phase, gen extra file aside to `GOROOT/src/runtime` package

This technique does not need to inspect the runtime's AST, so we don't change the `ShouldVisitPackage` options, instead, we generate the file in the `GenOverlay` phase.
