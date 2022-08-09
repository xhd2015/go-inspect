package util

import (
	"fmt"
	"os"
	"path"
	"strings"
)

// if pathName is "", cwd is returned
func ToAbsPath(pathName string) (string, error) {
	// if pathName == "" {
	// 	return "", fmt.Errorf("dir should not be empty")
	// }
	if path.IsAbs(pathName) {
		return pathName, nil
	}
	// _, err := os.Stat(pathName)
	// if err != nil {
	// 	return "", fmt.Errorf("%s not exists:%v", pathName, err)
	// }
	// if !f.IsDir() {
	// 	return "", fmt.Errorf("%s is not a dir", pathName)
	// }
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get cwd error:%v", err)
	}
	return path.Join(cwd, pathName), nil
}

// RelPath returns "" if child is not in base
func RelPath(base string, child string) (sub string, ok bool) {
	if base == "" || child == "" {
		return "", false
	}
	last := base[len(base)-1]
	if last == '/' || last == '\\' { // last is separator
		base = base[:len(base)-1]
	}
	idx := strings.Index(child, base)
	if idx < 0 {
		return "", false // not found
	}
	// found base, check the case /a/b_c /a/b
	idx += len(base)
	if idx >= len(child) {
		return "", true
	}
	if child[idx] == '/' || child[idx] == '\\' {
		return child[idx+1:], true
	}
	return "", false
}

// FindProjectDir return the first path that contains go.mod
func FindProjectDir(absFile string) string {
	projectDir := path.Dir(absFile)
	for projectDir != "/" {
		modStat, ferr := os.Stat(path.Join(projectDir, "go.mod"))
		if ferr == nil && !modStat.IsDir() {
			break
		}
		projectDir = path.Dir(projectDir)
		continue
	}
	if projectDir == "/" {
		panic(fmt.Errorf("no go.mod found for file:%v", absFile))
	}
	return projectDir
}
