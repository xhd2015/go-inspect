package util

import (
	"fmt"
	"go/token"
	"strings"
)

func StripNewline(s string) string {
	return strings.ReplaceAll(s, "\n", "")
}

func IsExportedName(name string) bool {
	if name == "" {
		return false
	}
	c := name[0]
	return c >= 'A' && c <= 'Z'

	// buggy: _X is not exported, but this still gets it.
	// return len(name) > 0 && strings.ToUpper(name[0:1]) == name[0:1]
}

func ContainsSplitWith(s string, e string, split byte) bool {
	if e == "" {
		return false
	}
	idx := strings.Index(s, e)
	if idx < 0 {
		return false
	}
	eidx := idx + len(e)
	return (idx == 0 || s[idx-1] == split) && (eidx == len(s) || s[eidx] == '/')
}

// /a/b/c  ->  /a ok
// /a/b/c  ->  /am !ok
// /a/b/c/ -> /a/b/c ok
// /a/b/cd -> /a/b/c !ok
func HasPrefixSplit(s string, e string, split byte) bool {
	if e == "" {
		return s == ""
	}
	ns, ne := len(s), len(e)
	if ns < ne {
		return false
	}
	for i := 0; i < ne; i++ {
		if s[i] != e[i] {
			return false
		}
	}
	return ns == ne || s[ne] == split
}

func FormatImport(use string, pkgPath string) string {
	if use != "" {
		return fmt.Sprintf("%s %q", use, pkgPath)
	}
	return fmt.Sprintf("%q", pkgPath)
}

func OffsetOf(fset *token.FileSet, pos token.Pos) int {
	if pos == token.NoPos {
		return -1
	}
	return fset.Position(pos).Offset
}
