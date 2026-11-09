package t3

import (
	"errors"
	"testing"
)

func TestErrorMessage(t *testing.T) {
	exit := errors.New("exit status 1")
	for _, c := range []struct{ out, want string }{
		{"\nERROR\n  T3 Code service is not installed.\n", "T3 Code service is not installed."},
		{"[04:05:30.372] ERROR (#2): NoRunningServerError: No running T3 Code server found.\n  checked /x\n    at file:///y", "No running T3 Code server found."},
		{"Downloading t3 0.1.9…\nchecksum mismatch for t3-linux-x64.tar.gz\n", "checksum mismatch for t3-linux-x64.tar.gz"},
		{"", "exit status 1"},
	} {
		if got := errorMessage(c.out, exit); got != c.want {
			t.Errorf("errorMessage(%q) = %q, want %q", c.out, got, c.want)
		}
	}
}
