// Package jsonc edits JSON files that may hold comments and trailing commas
// (JSONC, as Pi and OpenCode read them) without disturbing anything it does
// not change: comments, key order, indentation and line endings stay as they
// were, and setting a key then deleting it gives back the original bytes.
package jsonc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// member is one "key": value pair; offsets index the source.
type member struct {
	key              string
	start            int // the key's opening quote
	valStart, valEnd int
}

// object is a {...} with its members in order.
type object struct {
	open, close int // offsets of { and }
	members     []member
}

type scanner struct{ src []byte }

func (s scanner) errorf(at int, format string, args ...any) error {
	line := 1 + bytes.Count(s.src[:min(at, len(s.src))], []byte("\n"))
	return fmt.Errorf("line %d: %s", line, fmt.Sprintf(format, args...))
}

// skip passes whitespace and comments.
func (s scanner) skip(i int) int {
	for i < len(s.src) {
		switch {
		case s.src[i] == ' ' || s.src[i] == '\t' || s.src[i] == '\n' || s.src[i] == '\r':
			i++
		case bytes.HasPrefix(s.src[i:], []byte("//")):
			for i < len(s.src) && s.src[i] != '\n' {
				i++
			}
		case bytes.HasPrefix(s.src[i:], []byte("/*")):
			end := bytes.Index(s.src[i+2:], []byte("*/"))
			if end < 0 {
				return len(s.src)
			}
			i += 2 + end + 2
		default:
			return i
		}
	}
	return i
}

// str scans a string starting at its opening quote.
func (s scanner) str(i int) (end int, value string, err error) {
	for j := i + 1; j < len(s.src); j++ {
		switch s.src[j] {
		case '\\':
			j++
		case '"':
			err := json.Unmarshal(s.src[i:j+1], &value)
			return j + 1, value, err
		}
	}
	return 0, "", s.errorf(i, "unterminated string")
}

// value scans any value starting at i and returns where it ends.
func (s scanner) value(i int) (int, error) {
	if i >= len(s.src) {
		return 0, s.errorf(i, "missing value")
	}
	switch s.src[i] {
	case '{':
		o, err := s.object(i)
		return o.close + 1, err
	case '[':
		j := s.skip(i + 1)
		for j < len(s.src) && s.src[j] != ']' {
			end, err := s.value(j)
			if err != nil {
				return 0, err
			}
			j = s.skip(end)
			if j < len(s.src) && s.src[j] == ',' {
				j = s.skip(j + 1)
			}
		}
		if j >= len(s.src) {
			return 0, s.errorf(i, "unterminated array")
		}
		return j + 1, nil
	case '"':
		end, _, err := s.str(i)
		return end, err
	}
	j := i
	for j < len(s.src) && !strings.ContainsRune(" \t\r\n,}]/", rune(s.src[j])) {
		j++
	}
	if j == i {
		return 0, s.errorf(i, "unexpected %q", s.src[i])
	}
	return j, nil
}

// object scans the object whose { is at i.
func (s scanner) object(i int) (object, error) {
	o := object{open: i}
	j := s.skip(i + 1)
	for j < len(s.src) && s.src[j] != '}' {
		if s.src[j] != '"' {
			return o, s.errorf(j, "expected a key, found %q", s.src[j])
		}
		keyEnd, key, err := s.str(j)
		if err != nil {
			return o, err
		}
		colon := s.skip(keyEnd)
		if colon >= len(s.src) || s.src[colon] != ':' {
			return o, s.errorf(colon, "expected : after %q", key)
		}
		vs := s.skip(colon + 1)
		ve, err := s.value(vs)
		if err != nil {
			return o, err
		}
		o.members = append(o.members, member{key: key, start: j, valStart: vs, valEnd: ve})
		j = s.skip(ve)
		if j < len(s.src) && s.src[j] == ',' {
			j = s.skip(j + 1)
		}
	}
	if j >= len(s.src) {
		return o, s.errorf(i, "unterminated object")
	}
	o.close = j
	return o, nil
}

func (s scanner) root() (object, error) {
	i := s.skip(0)
	if i >= len(s.src) || s.src[i] != '{' {
		return object{}, errors.New("not a JSON object")
	}
	return s.object(i)
}

func (o object) find(key string) (int, bool) {
	for i, m := range o.members {
		if m.key == key {
			return i, true
		}
	}
	return 0, false
}

// walk finds the objects along path, ending with the one that holds (or
// would hold) the last key. found is how many of path's objects exist.
func walk(s scanner, path []string) (objs []object, found int, err error) {
	o, err := s.root()
	if err != nil {
		return nil, 0, err
	}
	objs = []object{o}
	for _, key := range path[:len(path)-1] {
		i, ok := o.find(key)
		if !ok {
			return objs, len(objs), nil
		}
		m := o.members[i]
		if s.src[m.valStart] != '{' {
			return nil, 0, s.errorf(m.valStart, "%q is not an object", key)
		}
		if o, err = s.object(m.valStart); err != nil {
			return nil, 0, err
		}
		objs = append(objs, o)
	}
	return objs, len(objs), nil
}

// Get returns the raw value at path, comments included.
func Get(src []byte, path ...string) ([]byte, bool, error) {
	if len(bytes.TrimSpace(src)) == 0 {
		return nil, false, nil
	}
	s := scanner{src}
	objs, found, err := walk(s, path)
	if err != nil || found < len(path) {
		return nil, false, err
	}
	o := objs[len(objs)-1]
	i, ok := o.find(path[len(path)-1])
	if !ok {
		return nil, false, nil
	}
	return src[o.members[i].valStart:o.members[i].valEnd], true, nil
}

// GetString returns the string at path, "" if it is missing or not a string.
func GetString(src []byte, path ...string) string {
	raw, ok, err := Get(src, path...)
	var v string
	if err != nil || !ok || json.Unmarshal(raw, &v) != nil {
		return ""
	}
	return v
}

// Unmarshal decodes JSONC into v.
func Unmarshal(src []byte, v any) error {
	if len(bytes.TrimSpace(src)) == 0 {
		return nil
	}
	return json.Unmarshal(Strip(src), v)
}

// Strip removes comments and trailing commas, leaving plain JSON.
func Strip(src []byte) []byte {
	s := scanner{src}
	var out []byte
	for i := 0; i < len(src); {
		switch {
		case src[i] == '"':
			end, _, err := s.str(i)
			if err != nil {
				return append(out, src[i:]...)
			}
			out = append(out, src[i:end]...)
			i = end
		case bytes.HasPrefix(src[i:], []byte("//")) || bytes.HasPrefix(src[i:], []byte("/*")):
			i = s.skip(i)
		case src[i] == ',':
			if j := s.skip(i + 1); j < len(src) && (src[j] == '}' || src[j] == ']') {
				i++
				continue
			}
			out = append(out, ',')
			i++
		default:
			out = append(out, src[i])
			i++
		}
	}
	return out
}

// lineStart is the offset where i's line begins.
func lineStart(src []byte, i int) int { return bytes.LastIndexByte(src[:i], '\n') + 1 }

// ownLine reports whether only whitespace precedes i on its line.
func ownLine(src []byte, i int) bool {
	return len(bytes.TrimLeft(src[lineStart(src, i):i], " \t")) == 0
}

func indentAt(src []byte, i int) string {
	ls := lineStart(src, i)
	j := ls
	for j < len(src) && (src[j] == ' ' || src[j] == '\t') {
		j++
	}
	return string(src[ls:j])
}

// unit guesses the file's indent step from its first member on a line of
// its own.
func unit(s scanner) string {
	o, err := s.root()
	if err == nil {
		for _, m := range o.members {
			if ownLine(s.src, m.start) {
				if ind := indentAt(s.src, m.start); ind != "" {
					return ind
				}
			}
		}
	}
	return "  "
}

// encode renders v for a member indented by ind, nested by step.
func encode(v any, ind, step string) (string, error) {
	b, err := json.MarshalIndent(v, ind, step)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Set puts v at path, creating missing objects on the way.
func Set(src []byte, v any, path ...string) ([]byte, error) {
	if len(path) == 0 {
		return nil, errors.New("jsonc: empty path")
	}
	if len(bytes.TrimSpace(src)) == 0 {
		src = []byte("{}\n")
	}
	s := scanner{src}
	step := unit(s)
	objs, found, err := walk(s, path)
	if err != nil {
		return nil, err
	}
	o := objs[len(objs)-1]
	rest := path[len(objs)-1:]
	if found == len(path) {
		if i, ok := o.find(path[len(path)-1]); ok {
			m := o.members[i]
			val, err := encode(v, indentAt(src, m.start), step)
			if err != nil {
				return nil, err
			}
			return splice(src, m.valStart, m.valEnd, val), nil
		}
	}
	// Wrap v in objects for the keys that don't exist yet, innermost first.
	for i := len(rest) - 1; i > 0; i-- {
		v = map[string]any{rest[i]: v}
	}
	return insert(s, o, rest[0], v, step)
}

// insert adds "key": v as o's last member.
func insert(s scanner, o object, key string, v any, step string) ([]byte, error) {
	src := s.src
	k, _ := json.Marshal(key)
	multiline := bytes.IndexByte(src[o.open:o.close], '\n') >= 0 || len(o.members) == 0
	if len(o.members) == 0 {
		outer := indentAt(src, o.open)
		ind := outer + step
		val, err := encode(v, ind, step)
		if err != nil {
			return nil, err
		}
		inner := src[o.open+1 : o.close]
		if len(bytes.TrimSpace(inner)) == 0 {
			return splice(src, o.open+1, o.close, "\n"+ind+string(k)+": "+val+"\n"+outer), nil
		}
		return splice(src, o.open+1, o.open+1, "\n"+ind+string(k)+": "+val+","), nil
	}
	last := o.members[len(o.members)-1]
	after := s.skip(last.valEnd)
	trailing := after < len(src) && src[after] == ','
	if !multiline {
		val, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		if trailing {
			return splice(src, after+1, after+1, " "+string(k)+": "+string(val)+","), nil
		}
		return splice(src, last.valEnd, last.valEnd, ", "+string(k)+": "+string(val)), nil
	}
	ind := indentAt(src, last.start)
	val, err := encode(v, ind, step)
	if err != nil {
		return nil, err
	}
	line := "\n" + ind + string(k) + ": " + val
	// After the last member's line, so a comment at its end stays with it.
	at := last.valEnd
	if trailing {
		at = after + 1
		line += ","
	}
	if nl := bytes.IndexByte(src[at:o.close], '\n'); nl >= 0 && len(bytes.TrimSpace(stripComments(src[at:at+nl]))) == 0 {
		at += nl
	} else if !trailing {
		return nil, s.errorf(last.valEnd, "can't add %q after a member that shares its line", key)
	}
	out := splice(src, at, at, line)
	if !trailing {
		out = splice(out, last.valEnd, last.valEnd, ",")
	}
	return out, nil
}

func stripComments(b []byte) []byte {
	if i := bytes.Index(b, []byte("//")); i >= 0 {
		b = b[:i]
	}
	return b
}

func splice(src []byte, from, to int, with string) []byte {
	out := make([]byte, 0, len(src)-(to-from)+len(with))
	out = append(out, src[:from]...)
	out = append(out, with...)
	return append(out, src[to:]...)
}

// Delete removes the member at path, and then any object the removal left
// empty on the way up. A missing path is not an error.
func Delete(src []byte, path ...string) ([]byte, error) {
	if len(path) == 0 || len(bytes.TrimSpace(src)) == 0 {
		return src, nil
	}
	s := scanner{src}
	objs, found, err := walk(s, path)
	if err != nil || found < len(path) {
		return src, err
	}
	o := objs[len(objs)-1]
	i, ok := o.find(path[len(path)-1])
	if !ok {
		return src, nil
	}
	if len(o.members) == 1 && len(path) > 1 && emptyBetween(src, o) {
		return Delete(src, path[:len(path)-1]...)
	}
	return remove(s, o, i), nil
}

// emptyBetween reports whether o holds nothing but its one member:
// no comments that deleting the whole object would lose.
func emptyBetween(src []byte, o object) bool {
	m := o.members[0]
	s := scanner{src}
	end := s.skip(m.valEnd)
	if end < len(src) && src[end] == ',' {
		end++
	}
	plain := func(b []byte) bool { return len(bytes.TrimSpace(b)) == 0 }
	return plain(src[o.open+1:m.start]) && plain(src[end:o.close])
}

// remove cuts member i out of o, with the comma that separated it.
func remove(s scanner, o object, i int) []byte {
	src := s.src
	m := o.members[i]
	after := s.skip(m.valEnd)
	hasComma := after < len(src) && src[after] == ','
	end := m.valEnd
	if hasComma {
		end = after + 1
	}
	if ownLine(src, m.start) {
		if nl := bytes.IndexByte(src[end:], '\n'); nl >= 0 && len(bytes.TrimSpace(stripComments(src[end:end+nl]))) == 0 {
			start := lineStart(src, m.start)
			out := splice(src, start, end+nl+1, "")
			if !hasComma && i > 0 {
				// It was last: drop the comma the previous member got for it.
				prev := o.members[i-1]
				if c := s.skip(prev.valEnd); src[c] == ',' && c < start {
					out = splice(out, c, c+1, "")
				}
			}
			if len(o.members) == 1 {
				return collapse(out, o.open)
			}
			return out
		}
	}
	switch {
	case i > 0 && !hasComma:
		prev := o.members[i-1]
		return splice(src, prev.valEnd, m.valEnd, "")
	case i == 0 && len(o.members) > 1:
		return splice(src, m.start, o.members[1].start, "")
	default:
		return collapse(splice(src, m.start, end, ""), o.open)
	}
}

// collapse turns an object at open left holding only whitespace into {}.
func collapse(src []byte, open int) []byte {
	end := scanner{src}.skip(open + 1)
	if end < len(src) && src[end] == '}' && len(bytes.TrimSpace(src[open+1:end])) == 0 {
		return splice(src, open+1, end, "")
	}
	return src
}
