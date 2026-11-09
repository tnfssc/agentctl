// Package semver orders exact SemVer versions, prereleases included.
package semver

import (
	"math/big"
	"strings"
)

// Compare orders exact SemVer versions, prereleases included
// (0.1.8-nightly.20261106.4262 < 0.1.8-nightly.20261108.4380 < 0.1.8).
func Compare(a, b string) int {
	coreA, preA, _ := strings.Cut(strings.SplitN(a, "+", 2)[0], "-")
	coreB, preB, _ := strings.Cut(strings.SplitN(b, "+", 2)[0], "-")
	if c := compareIDs(strings.Split(coreA, "."), strings.Split(coreB, ".")); c != 0 {
		return c
	}
	switch {
	case preA == preB:
		return 0
	case preA == "":
		return 1
	case preB == "":
		return -1
	}
	return compareIDs(strings.Split(preA, "."), strings.Split(preB, "."))
}

func compareIDs(a, b []string) int {
	for i := range max(len(a), len(b)) {
		if i >= len(a) {
			return -1
		}
		if i >= len(b) {
			return 1
		}
		x, xNum := new(big.Int).SetString(a[i], 10)
		y, yNum := new(big.Int).SetString(b[i], 10)
		switch {
		case xNum && yNum:
			if c := x.Cmp(y); c != 0 {
				return c
			}
		case xNum != yNum:
			if xNum {
				return -1
			}
			return 1
		default:
			if c := strings.Compare(a[i], b[i]); c != 0 {
				return c
			}
		}
	}
	return 0
}
