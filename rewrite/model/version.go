package model

import (
	"strconv"
	"strings"
)

type Version struct {
	Major int
	Minor int
	Patch int
}

func (c *Version) Compare(b *Version) int {
	mj := c.Major - b.Major
	if mj != 0 {
		return mj
	}
	mi := c.Minor - b.Minor
	if mi != 0 {
		return mi
	}
	return c.Patch - b.Patch
}

// 1 -> major = 1
// 1.2 -> major = 1, minor = 2
// 1.2.3 -> major = 1, minor = 2, patch=3
// 1.2.3.4 -> no patches
func ParseVersion(v string) Version {
	splits := strings.Split(v, ".")
	if len(splits) == 0 {
		return Version{}
	}
	ver := Version{}
	// can be anything plus version number
	head := []byte(splits[0])

	i := len(head) - 1
	for ; i >= 0; i-- {
		// not number
		if head[i] < '0' || head[i] > '9' {
			break
		}
	}
	if i+1 < len(head) {
		ver.Major, _ = strconv.Atoi(string(head[i+1:]))
	}
	if len(splits) >= 2 {
		ver.Minor, _ = strconv.Atoi(splits[1])
	}
	if len(splits) >= 3 {
		ver.Patch, _ = strconv.Atoi(splits[2])
	}

	return ver
}
