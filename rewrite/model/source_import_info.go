package model

import (
	"io"
	"time"

	"github.com/xhd2015/go-vendor-pack/packfs"
)

type Module struct {
	Path              string
	Version           string
	ReplacedWithLocal bool // replaced with local
	UpdateTime        time.Time

	// vendoer FS, which contains: vendor/{pkgPath}
	FS packfs.FS

	Packages map[string]*Package
}

type Package struct {
	Path string
}

type File struct {
	Name string
	Open func() (io.Reader, error)
	// some file may have a cached Checksum
	// if returned ok is false,
	// empty file will also return non-empty checksum
	Checksum func() (checksum string, ok bool)
}
