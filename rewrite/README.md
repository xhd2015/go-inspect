# bug at v0.0.12

> fixed at v0.0.13

Take the following file for example:

```go
// some leading comment
package example

func Example(){
}

// some trailing space
```

Current implementation only see the middle part:

```go
package example

func Example(){
}
```

This is because we use `ast.Package` as begin pos, so comments before that are not included.
