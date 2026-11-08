package tune

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"github.com/tnfssc/agentctl/internal/agent"
	"github.com/tnfssc/agentctl/internal/agent/claude"
	"github.com/tnfssc/agentctl/internal/agent/codex"
	"github.com/tnfssc/agentctl/internal/config"
	"github.com/tnfssc/agentctl/internal/tomledit"
)

// Status is an item and whether the agent's settings already match it.
type Status struct {
	Item
	Applied bool
	Notes   []string
}

// doc is an agent's settings file being edited in memory.
type doc interface {
	holds(op Op) bool
	apply(op Op) error
	reset(op Op) error
	bytes() []byte
}

type session struct {
	agent  string
	path   string
	before []byte
	doc    doc
	shell  *shellEnv
}

func open(agentName string) (*session, error) {
	s := &session{agent: agentName}
	var err error
	switch agentName {
	case claude.Name:
		if s.path, err = claude.SettingsPath(); err != nil {
			return nil, err
		}
		if s.before, err = agent.ReadFile(s.path); err != nil {
			return nil, err
		}
		src := s.before
		if len(bytes.TrimSpace(src)) == 0 {
			src = []byte("{}")
		}
		if !gjson.ValidBytes(src) {
			return nil, fmt.Errorf("%s is not valid JSON; fix it first", s.path)
		}
		s.doc = &jsonDoc{src: src}
	case codex.Name:
		if s.path, err = codex.ConfigPath(); err != nil {
			return nil, err
		}
		if s.before, err = agent.ReadFile(s.path); err != nil {
			return nil, err
		}
		var probe map[string]any
		if err := toml.Unmarshal(s.before, &probe); err != nil {
			return nil, fmt.Errorf("%s: %w", s.path, err)
		}
		s.doc = &tomlDoc{src: s.before}
	default:
		return nil, fmt.Errorf("unknown agent %q", agentName)
	}
	if s.shell, err = loadShellEnv(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *session) holds(op Op) bool {
	if op.Env != "" {
		return s.shell.has(op.Env) || os.Getenv(op.Env) == "1"
	}
	return s.doc.holds(op)
}

func (s *session) applied(it Item) bool {
	for _, op := range it.Ops {
		if !s.holds(op) {
			return false
		}
	}
	return true
}

// Inspect reports which curated items an agent's settings already match.
func Inspect(agentName string) ([]Status, error) {
	s, err := open(agentName)
	if err != nil {
		return nil, err
	}
	items := Items(agentName)
	out := make([]Status, len(items))
	for i, it := range items {
		st := Status{Item: it, Applied: s.applied(it)}
		for _, op := range it.Ops {
			if op.Env != "" && os.Getenv(op.Env) == "1" && !s.shell.has(op.Env) {
				st.Notes = append(st.Notes, op.Env+" is set in your shell outside agentctl; resetting won't remove it")
			}
		}
		out[i] = st
	}
	return out, nil
}

// Recommended is the starting selection: what is already applied plus the
// recommended items.
func Recommended(statuses []Status) map[string]bool {
	want := map[string]bool{}
	for _, st := range statuses {
		want[st.ID] = st.Applied || st.Recommended
	}
	return want
}

// Plan prepares the edits that make the agent match want: ticked items are
// applied, unticked items that are currently applied are reset to the agent's
// defaults. Items missing from want count as unticked.
func Plan(agentName string, want map[string]bool) (agent.Plan, error) {
	s, err := open(agentName)
	if err != nil {
		return agent.Plan{}, err
	}
	plan := agent.Plan{Agent: agentName}
	for _, it := range Items(agentName) {
		applied := s.applied(it)
		switch {
		case want[it.ID] && !applied:
			for _, op := range it.Ops {
				if op.Env != "" {
					s.shell.set(op.Env, true)
				} else if err := s.doc.apply(op); err != nil {
					return agent.Plan{}, fmt.Errorf("%s: %w", it.Title, err)
				}
			}
		case !want[it.ID] && applied:
			for _, op := range it.Ops {
				if op.Env != "" {
					s.shell.set(op.Env, false)
				} else if err := s.doc.reset(op); err != nil {
					return agent.Plan{}, fmt.Errorf("%s: %w", it.Title, err)
				}
			}
		}
	}

	after := s.doc.bytes()
	if s.agent == claude.Name {
		after = claude.Format(after, s.before)
		if s.before == nil && string(after) == "{}\n" {
			after = nil
		}
	}
	plan.Changes = append(plan.Changes, agent.Change{Path: s.path, Before: s.before, After: after, Perm: 0o600})
	shellChanges, notes := s.shell.changes()
	plan.Changes = append(plan.Changes, shellChanges...)
	plan.Warnings = append(plan.Warnings, notes...)
	return plan, nil
}

// JSON (Claude Code settings.json) ---------------------------------------

type jsonDoc struct{ src []byte }

func (d *jsonDoc) bytes() []byte { return d.src }

func (d *jsonDoc) holds(op Op) bool {
	r := gjson.GetBytes(d.src, op.Path)
	switch {
	case op.Drop:
		return !r.Exists()
	case op.Entry != "":
		return d.index(op) >= 0
	default:
		return r.Exists() && reflect.DeepEqual(r.Value(), op.Value)
	}
}

func (d *jsonDoc) index(op Op) int {
	for i, e := range gjson.GetBytes(d.src, op.Path).Array() {
		if e.String() == op.Entry {
			return i
		}
	}
	return -1
}

func (d *jsonDoc) apply(op Op) (err error) {
	switch {
	case d.holds(op):
	case op.Drop:
		d.src, err = sjson.DeleteBytes(d.src, op.Path)
	case op.Entry != "":
		d.src, err = sjson.SetBytes(d.src, op.Path+".-1", op.Entry)
	default:
		d.src, err = sjson.SetBytes(d.src, op.Path, op.Value)
	}
	return err
}

// reset returns a setting to the agent's default: managed values are
// removed, dropped values stay dropped.
func (d *jsonDoc) reset(op Op) (err error) {
	switch {
	case op.Drop:
		return nil
	case op.Entry != "":
		if i := d.index(op); i >= 0 {
			d.src, err = sjson.DeleteBytes(d.src, op.Path+"."+strconv.Itoa(i))
		}
	default:
		d.src, err = sjson.DeleteBytes(d.src, op.Path)
	}
	if err != nil {
		return err
	}
	// Remove containers left empty, so reset leaves no trace.
	parts := strings.Split(op.Path, ".")
	for n := len(parts); n > 0; n-- {
		p := strings.Join(parts[:n], ".")
		r := gjson.GetBytes(d.src, p)
		if !r.Exists() {
			continue
		}
		if r.IsArray() && len(r.Array()) == 0 || r.IsObject() && len(r.Map()) == 0 {
			if d.src, err = sjson.DeleteBytes(d.src, p); err != nil {
				return err
			}
			continue
		}
		break
	}
	return nil
}

// TOML (Codex config.toml) -------------------------------------------------

type tomlDoc struct{ src []byte }

func (d *tomlDoc) bytes() []byte { return d.src }

func (d *tomlDoc) holds(op Op) bool {
	raw, ok, err := tomledit.Get(d.src, strings.Split(op.Path, ".")...)
	switch {
	case err != nil:
		return false
	case op.Drop:
		return !ok
	case !ok:
		return false
	}
	var got, want struct{ V any }
	if toml.Unmarshal([]byte("V = "+raw), &got) != nil {
		return false
	}
	lit, err := literal(op.Value)
	if err != nil || toml.Unmarshal([]byte("V = "+lit), &want) != nil {
		return false
	}
	return reflect.DeepEqual(got.V, want.V)
}

func (d *tomlDoc) apply(op Op) (err error) {
	path := strings.Split(op.Path, ".")
	switch {
	case op.Entry != "":
		return errors.New("array entries are not supported in TOML")
	case d.holds(op):
	case op.Drop:
		d.src, err = tomledit.Delete(d.src, path...)
	default:
		var lit string
		if lit, err = literal(op.Value); err == nil {
			d.src, err = tomledit.Set(d.src, lit, path...)
		}
	}
	return err
}

func (d *tomlDoc) reset(op Op) (err error) {
	if op.Drop {
		return nil
	}
	d.src, err = tomledit.Delete(d.src, strings.Split(op.Path, ".")...)
	return err
}

func literal(v any) (string, error) {
	switch v := v.(type) {
	case bool:
		return strconv.FormatBool(v), nil
	case string:
		return tomledit.String(v), nil
	}
	return "", fmt.Errorf("unsupported value %v", v)
}

// Shell environment -------------------------------------------------------

// shellEnv is the set of variables agentctl exports for agents that only
// read them from the launching shell. It owns two small files: a POSIX
// snippet to source from any shell rc, and a fish conf.d file fish loads on
// its own.
type shellEnv struct {
	shPath, fishPath     string // fishPath is empty without a fish config dir
	shBefore, fishBefore []byte
	vars                 []string
	changed              bool
}

const snippetHeader = "# Managed by agentctl (agentctl tune). Edits here are overwritten.\n"

func loadShellEnv() (*shellEnv, error) {
	dir, err := config.Dir()
	if err != nil {
		return nil, err
	}
	e := &shellEnv{shPath: filepath.Join(dir, "env.sh")}
	if e.shBefore, err = agent.ReadFile(e.shPath); err != nil {
		return nil, err
	}
	for line := range strings.Lines(string(e.shBefore)) {
		if name, ok := strings.CutPrefix(strings.TrimSpace(line), "export "); ok {
			if name, ok = strings.CutSuffix(name, "=1"); ok {
				e.vars = append(e.vars, name)
			}
		}
	}
	fishDir := filepath.Join(filepath.Dir(dir), "fish")
	if info, err := os.Stat(fishDir); err == nil && info.IsDir() {
		e.fishPath = filepath.Join(fishDir, "conf.d", "agentctl.fish")
		if e.fishBefore, err = agent.ReadFile(e.fishPath); err != nil {
			return nil, err
		}
	}
	return e, nil
}

func (e *shellEnv) has(name string) bool { return slices.Contains(e.vars, name) }

func (e *shellEnv) set(name string, on bool) {
	if on == e.has(name) {
		return
	}
	e.changed = true
	if on {
		e.vars = append(e.vars, name)
	} else {
		e.vars = slices.DeleteFunc(e.vars, func(v string) bool { return v == name })
	}
}

func (e *shellEnv) changes() ([]agent.Change, []string) {
	if !e.changed {
		return nil, nil
	}
	var sh, fish strings.Builder
	sh.WriteString(snippetHeader)
	fish.WriteString(snippetHeader)
	for _, v := range e.vars {
		sh.WriteString("export " + v + "=1\n")
		fish.WriteString("set -gx " + v + " 1\n")
	}
	out := []agent.Change{{Path: e.shPath, Before: e.shBefore, After: []byte(sh.String()), Perm: 0o644}}
	if e.fishPath != "" {
		out = append(out, agent.Change{Path: e.fishPath, Before: e.fishBefore, After: []byte(fish.String()), Perm: 0o644})
	}
	if len(e.vars) == 0 {
		return out, nil
	}
	note := fmt.Sprintf("Shell variables only take effect in new shells. Add `. %s` to your ~/.zshrc or ~/.bashrc", e.shPath)
	if e.fishPath != "" {
		note += "; fish loads them on its own"
	}
	return out, []string{note + "."}
}
