#!/usr/bin/env bash
set -e

# this standalone package is required because
# we do not want to bring extra dependency to main go.mod

go get github.com/xhd2015/go-vendor-pack@latest

go run github.com/xhd2015/go-vendor-pack/cmd/go-pack pack \
    -pkg export_g \
    -var GETG_PACK \
    -o ../get_g_gen.go \
    -run-go-mod-tidy \
    -run-go-mod-vendor \
    ../pack

echo 'package getg' > ../pack/vendor/github.com/xhd2015/go-inspect/plugin/getg/err_msg.go