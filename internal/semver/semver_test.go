package semver

import "testing"

func TestCompare(t *testing.T) {
	ordered := []string{"0.0.46-nightly.20261012.3010", "0.1.8-nightly.20261106.4262", "0.1.8-nightly.20261108.4380", "0.1.8", "0.1.10"}
	for i := range ordered {
		for j := range ordered {
			want := 0
			if i < j {
				want = -1
			} else if i > j {
				want = 1
			}
			if got := Compare(ordered[i], ordered[j]); got != want {
				t.Errorf("Compare(%s, %s) = %d, want %d", ordered[i], ordered[j], got, want)
			}
		}
	}
}
