package guide_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tnfssc/agentctl/internal/guide"
	"github.com/tnfssc/agentctl/internal/testutil"
)

func TestGuidesNameTheirRecordAndUndo(t *testing.T) {
	testutil.Home(t)
	dir, err := guide.RecordsDir()
	if err != nil {
		t.Fatal(err)
	}
	for _, topic := range guide.Topics() {
		text, err := guide.Get(topic)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(text, "<records>") {
			t.Errorf("%s: records placeholder left in", topic)
		}
		undo := guide.Undo(topic)
		if undo == "" {
			continue
		}
		// The setup writes the record its undo guide reads.
		record := filepath.Join(dir, topic+".md")
		if !strings.Contains(text, record) || !strings.Contains(text, "agentctl guide "+undo) {
			t.Errorf("%s: should name %s and `agentctl guide %s`", topic, record, undo)
		}
		if u, _ := guide.Get(undo); !strings.Contains(u, record) {
			t.Errorf("%s: should read %s", undo, record)
		}
	}
}

func TestAppliedFollowsRecords(t *testing.T) {
	testutil.Home(t)
	if got := guide.Applied(); len(got) != 0 {
		t.Fatalf("nothing set up yet, got %v", got)
	}
	dir, _ := guide.RecordsDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"tunnel.md", "signing.md"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// Signing has no undo guide, so it never counts as something to undo.
	if got := guide.Applied(); !slices.Equal(got, []string{"tunnel"}) {
		t.Fatalf("applied = %v", got)
	}
}
