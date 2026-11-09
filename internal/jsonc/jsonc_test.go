package jsonc

import (
	"encoding/json"
	"strings"
	"testing"
)

var docs = map[string]string{
	"empty":    "",
	"braces":   "{}\n",
	"plain":    "{\n  \"theme\": \"dark\",\n  \"n\": 1\n}\n",
	"comments": "{\n  // \"providers\": {} in a comment\n  \"theme\": \"x\", /* { */\n  \"providers\": {\n    \"writer\": {\"a\": 1} // trailing\n  }\n}\n",
	"trailing": "{\n  \"a\": 1,\n  \"providers\": {\n    \"w\": 2,\n  },\n}\n",
	"tabs":     "{\n\t\"a\": [1, 2,\n\t\t3],\n\t\"b\": \"x\"\n}",
	"compact":  `{"a":1,"providers":{"w":{}}}`,
	"nested":   "{\n  \"providers\": {\n    \"agentctl-other\": {\n      \"x\": true\n    }\n  },\n  \"last\": null\n}\n",
	"escapes":  "{\n  \"k\\\"ey\": \"va\\\\lue // not a comment\",\n  \"url\": \"https://x/y\"\n}\n",
}

func TestSetThenDeleteRoundTrips(t *testing.T) {
	value := map[string]any{"name": "agentctl", "models": []string{"a", "b"}}
	for name, doc := range docs {
		for _, path := range [][]string{{"providers", "agentctl"}, {"model"}, {"providers", "claude-agent", "settings", "baseURL"}} {
			out, err := Set([]byte(doc), value, path...)
			if err != nil {
				t.Errorf("%s: set %v: %v", name, path, err)
				continue
			}
			var check any
			if err := json.Unmarshal(Strip(out), &check); err != nil {
				t.Errorf("%s: set %v gave invalid JSON: %v\n%s", name, path, err, out)
				continue
			}
			got, ok, err := Get(out, path...)
			if err != nil || !ok || !strings.Contains(string(got), `"agentctl"`) {
				t.Errorf("%s: get %v after set = %q %v %v\n%s", name, path, got, ok, err, out)
			}
			back, err := Delete(out, path...)
			if err != nil {
				t.Errorf("%s: delete %v: %v", name, path, err)
				continue
			}
			want := doc
			if strings.TrimSpace(doc) == "" {
				want = "{}\n"
			}
			if string(back) != want {
				t.Errorf("%s: set+delete %v changed the file:\n--- want\n%s\n--- got\n%s", name, path, want, back)
			}
		}
	}
}

func TestSetReplacesInPlace(t *testing.T) {
	doc := "{\n  // keep me\n  \"model\": \"a/b\", // and me\n  \"n\": 1\n}\n"
	out, err := Set([]byte(doc), "agentctl/gpt", "model")
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  // keep me\n  \"model\": \"agentctl/gpt\", // and me\n  \"n\": 1\n}\n"
	if string(out) != want {
		t.Fatalf("got:\n%s", out)
	}
	if GetString(out, "model") != "agentctl/gpt" {
		t.Fatal("GetString")
	}
}

func TestNewMembersMatchIndentation(t *testing.T) {
	out, err := Set([]byte("{\n    \"a\": 1\n}\n"), map[string]any{"b": 2}, "x", "y")
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n    \"a\": 1,\n    \"x\": {\n        \"y\": {\n            \"b\": 2\n        }\n    }\n}\n"
	if string(out) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out, want)
	}
}

func TestDeleteKeepsWhatItDidNotAdd(t *testing.T) {
	doc := "{\n  \"providers\": {\n    \"writer\": {},\n    \"agentctl\": {}\n  }\n}\n"
	out, err := Delete([]byte(doc), "providers", "agentctl")
	if err != nil {
		t.Fatal(err)
	}
	if want := "{\n  \"providers\": {\n    \"writer\": {}\n  }\n}\n"; string(out) != want {
		t.Fatalf("got:\n%s", out)
	}
	if out, _ := Delete([]byte(doc), "nope", "x"); string(out) != doc {
		t.Fatal("deleting a missing path must change nothing")
	}
}

func TestCommentsAreNotKeys(t *testing.T) {
	doc := "{\n  // \"model\": \"fake\"\n  /* \"model\": \"fake\" */\n}\n"
	if _, ok, _ := Get([]byte(doc), "model"); ok {
		t.Fatal("a key inside a comment must not be found")
	}
	var v map[string]any
	if err := Unmarshal([]byte(docs["trailing"]), &v); err != nil || v["a"] != 1.0 {
		t.Fatalf("unmarshal with trailing commas: %v %v", v, err)
	}
}

func TestErrorsNameTheLine(t *testing.T) {
	_, err := Set([]byte("{\n  \"a\": 1\n  \"b\": [\n}\n"), 1, "c")
	if err == nil || !strings.Contains(err.Error(), "line") {
		t.Fatalf("err = %v", err)
	}
}
