package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func execute(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func TestHelpListsDemo(t *testing.T) {
	out, err := execute(t, "--help")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "demo") {
		t.Fatalf("help does not mention demo:\n%s", out)
	}
}

func TestDemoNeedsTerminal(t *testing.T) {
	_, err := execute(t, "demo")
	if err == nil || !strings.Contains(err.Error(), "interactive terminal") {
		t.Fatalf("err = %v, want interactive terminal error", err)
	}
}

func TestDemoRejectsBadSpeed(t *testing.T) {
	_, err := execute(t, "demo", "--speed", "0")
	if err == nil || !strings.Contains(err.Error(), "--speed") {
		t.Fatalf("err = %v, want --speed error", err)
	}
}

func TestDemoRefusesDevNull(t *testing.T) {
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer devNull.Close()
	if isTerminal(devNull) {
		t.Fatal("/dev/null reported as a terminal")
	}
}
