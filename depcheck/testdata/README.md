# Conclusion
in the same package, imports by file name ordered
in the same file, imports by written order


# Test
p1 vs p2: same file, different imports order

```bash
$ (cd p1 && go run .)
init a
init b
main init
main func

$ (cd p2 && go run .)
init b
init a
main init
main func
```


p3 vs p4: same imports, different file order
```bash
$ (cd p3 && go run .)
init a
init b
main init
main func

$ (cd p4 && go run .)
init b
init a
main init
main func
```

file orders:
p3: f1.go, f2.go
p4: f2.go, f3.go


Package load: Package.Syntax have files already sorted