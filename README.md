# Introduction

Let you do anything by manipulating the go code.

We provide source file parsing, rewriting and building utilities in this repository. A classical usage is to implement function trace by adding a log in begining of each function's body.

A more sophisticated usage is [https://github.com/xhd2015/go-mock](https://github.com/xhd2015/go-mock).

# Usage

## CLI

```bash
# add dependency
go get github.com/xhd2015/go-inspect
```

# How it works?

The whole process can be splitted into the following phases:

```bash
ParseOptions
LoadPackages
FindMainModule
FindStarterPackages
FindExtraPackages
TraversePackages
    TraverseFiles
        TraverseFunctions
            TraverseNodes
SyncContent
Build
```

# Rewrite

How we refactor rewrite?

1. reduce context
   we wrap all contextual information with proper struct

2. provide Visitor-like pattern
