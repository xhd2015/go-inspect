# getg

Export the `runtime.getg()` which is unexported by default.

This makes goroutine local storage easier to implement, like in testing environment.

Don't use this in production.
