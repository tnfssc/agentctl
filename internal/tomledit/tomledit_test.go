package tomledit

import (
	"strings"
	"testing"
)

const sample = `# my codex config
model = "gpt-6.1-sol" # daily driver
model_reasoning_effort = "high"
notify = [
  "say", # speak
  "done",
]

[features]
hooks = true

# provider managed by agentctl
[model_providers.agentctl]
name = "old"

[model_providers.agentctl.auth]
command = "agentctl"


# keep me
[projects."/x"]
trust_level = "trusted"
`

func TestSetTopLevelReplacesInPlace(t *testing.T) {
	out, err := SetTopLevel([]byte(sample), "model", String("gpt-6-luna"))
	if err != nil {
		t.Fatal(err)
	}
	want := `model = "gpt-6-luna" # daily driver`
	if got := string(out); !contains(got, want) || !contains(got, "# my codex config") {
		t.Fatalf("missing %q or comments in:\n%s", want, got)
	}
}

func TestSetTopLevelInsertsBeforeTables(t *testing.T) {
	out, err := SetTopLevel([]byte(sample), "model_provider", String("agentctl"))
	if err != nil {
		t.Fatal(err)
	}
	want := "  \"done\",\n]\nmodel_provider = \"agentctl\"\n\n[features]"
	if !contains(string(out), want) {
		t.Fatalf("want %q in:\n%s", want, out)
	}
}

func TestSetTopLevelOnTableOnlyDoc(t *testing.T) {
	out, err := SetTopLevel([]byte("[features]\nhooks = true\n"), "model", String("m"))
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "model = \"m\"\n\n[features]\nhooks = true\n" {
		t.Fatalf("got:\n%s", out)
	}
}

func TestSetTopLevelOnEmptyDoc(t *testing.T) {
	out, err := SetTopLevel(nil, "model", String("m"))
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "model = \"m\"\n" {
		t.Fatalf("got %q", out)
	}
}

func TestDeleteTopLevelMultiline(t *testing.T) {
	out, err := DeleteTopLevel([]byte(sample), "notify")
	if err != nil {
		t.Fatal(err)
	}
	if contains(string(out), "notify") || contains(string(out), `"say"`) {
		t.Fatalf("notify not removed:\n%s", out)
	}
	if !contains(string(out), "model_reasoning_effort = \"high\"\n\n[features]") {
		t.Fatalf("neighbors damaged:\n%s", out)
	}
}

func TestDeleteTopLevelIgnoresTableKeys(t *testing.T) {
	out, err := DeleteTopLevel([]byte(sample), "hooks")
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != sample {
		t.Fatalf("table key should not be touched:\n%s", out)
	}
}

func TestRemoveTableWithSubtables(t *testing.T) {
	out, err := RemoveTable([]byte(sample), "model_providers", "agentctl")
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	if contains(got, "[model_providers") || contains(got, `"old"`) {
		t.Fatalf("table not removed:\n%s", got)
	}
	if !contains(got, "# keep me\n[projects.\"/x\"]") {
		t.Fatalf("following table damaged:\n%s", got)
	}
}

func TestAppendBlock(t *testing.T) {
	out, err := AppendBlock([]byte("a = 1"), "[t]\nb = 2\n")
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "a = 1\n\n[t]\nb = 2\n" {
		t.Fatalf("got %q", out)
	}
}

func TestStringEscapes(t *testing.T) {
	out, err := SetTopLevel(nil, "k", String("a\"b\\c\nd\x01"))
	if err != nil {
		t.Fatal(err)
	}
	got, ok, err := TopLevel(out, "k")
	if err != nil || !ok || got != "a\"b\\c\nd\x01" {
		t.Fatalf("round trip = %q %v %v", got, ok, err)
	}
}

func TestInvalidInputIsRejected(t *testing.T) {
	if _, err := SetTopLevel([]byte("a = "), "b", "1"); err == nil {
		t.Fatal("want parse error")
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }

func TestAppendThenRemoveIsIdentity(t *testing.T) {
	for _, src := range []string{"a = 1\n", "a = 1\n\n[t]\nb = 2\n", "", "[t]\nb = 2", "a = 1\n\n", "[t]\nb = 2\n\n\n"} {
		out, err := AppendBlock([]byte(src), "[x.y]\nc = 3\n\n[x.y.z]\nd = 4\n")
		if err != nil {
			t.Fatal(err)
		}
		out, err = RemoveTable(out, "x", "y")
		if err != nil {
			t.Fatal(err)
		}
		want := src
		if want != "" && !strings.HasSuffix(want, "\n") {
			want += "\n"
		}
		if string(out) != want {
			t.Errorf("round trip of %q = %q", src, out)
		}
	}
}

func TestSetInTables(t *testing.T) {
	tests := []struct {
		name, src, want string
		path            []string
	}{
		{"replace keeps comment", "[features]\nhooks = true # mine\n", "[features]\nhooks = false # mine\n", []string{"features", "hooks"}},
		{"add to existing table", "[features]\nhooks = true\n\n[tui]\nx = 1\n", "[features]\nhooks = true\nimage_generation = false\n\n[tui]\nx = 1\n", []string{"features", "image_generation"}},
		{"add to header-only table at EOF", "[features]", "[features]\nimage_generation = false\n", []string{"features", "image_generation"}},
		{"dotted style", "model = \"m\"\nfeatures.hooks = true\n\n[tui]\n", "model = \"m\"\nfeatures.hooks = true\nfeatures.image_generation = false\n\n[tui]\n", []string{"features", "image_generation"}},
		{"new table", "model = \"m\"\n", "model = \"m\"\n\n[features]\nimage_generation = false\n", []string{"features", "image_generation"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := Set([]byte(tt.src), "false", tt.path...)
			if err != nil {
				t.Fatal(err)
			}
			if string(out) != tt.want {
				t.Fatalf("got:\n%q\nwant:\n%q", out, tt.want)
			}
			if got, ok, _ := Get(out, tt.path...); !ok || got != "false" {
				t.Fatalf("Get = %q %v", got, ok)
			}
		})
	}
}

func TestSetThenDeleteRestores(t *testing.T) {
	for _, src := range []string{"model = \"m\"\n", "[features]\nhooks = true\n", "model = \"m\"\n\n[tui]\nx = 1\n"} {
		out, err := Set([]byte(src), "false", "analytics", "enabled")
		if err != nil {
			t.Fatal(err)
		}
		if out, err = Delete(out, "analytics", "enabled"); err != nil {
			t.Fatal(err)
		}
		if string(out) != src {
			t.Errorf("round trip of %q = %q", src, out)
		}
	}
}

func TestDeleteKeepsNonEmptyTable(t *testing.T) {
	src := "[features]\nhooks = true\nx = 1\n"
	out, err := Delete([]byte(src), "features", "x")
	if err != nil || string(out) != "[features]\nhooks = true\n" {
		t.Fatalf("got %q %v", out, err)
	}
}
