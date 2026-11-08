// Package tomledit makes small edits to a TOML document while leaving every
// other byte alone, so comments and the user's formatting survive. It knows
// just enough to set or delete top-level keys and to replace whole tables.
package tomledit

import (
	"bytes"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable"
)

type itemKind int

const (
	keyValue itemKind = iota
	header
)

// item is one top-level expression: a key/value pair or a table header.
type item struct {
	kind  itemKind
	table []string // enclosing table for key/values; the table itself for headers
	key   []string // key/value key; nil for headers
	start int      // byte offset of the first byte
	end   int      // byte offset just past the last byte
}

func parse(src []byte) ([]item, error) {
	var (
		p     unstable.Parser
		items []item
		table []string
	)
	p.Reset(src)
	for p.NextExpression() {
		e := p.Expression()
		var (
			keys  []string
			first unstable.Range
		)
		for it := e.Key(); it.Next(); {
			n := it.Node()
			if keys == nil {
				first = n.Raw
			}
			keys = append(keys, string(n.Data))
		}
		switch e.Kind {
		case unstable.KeyValue:
			items = append(items, item{
				kind: keyValue, table: table, key: keys,
				start: int(e.Raw.Offset), end: int(e.Raw.Offset + e.Raw.Length),
			})
		case unstable.Table, unstable.ArrayTable:
			table = keys
			// The key range sits inside the brackets; widen to the line.
			start := lineStart(src, int(first.Offset))
			items = append(items, item{kind: header, table: keys, start: start, end: lineEnd(src, start)})
		default:
			// Comments and whitespace are not reported as expressions.
		}
	}
	return items, p.Error()
}

func lineStart(src []byte, off int) int {
	return bytes.LastIndexByte(src[:off], '\n') + 1
}

// lineEnd returns the offset just past the newline ending the line at off,
// or len(src).
func lineEnd(src []byte, off int) int {
	if i := bytes.IndexByte(src[off:], '\n'); i >= 0 {
		return off + i + 1
	}
	return len(src)
}

// SetTopLevel sets key (outside any table) to a TOML literal such as
// String("x"). An existing value is replaced in place; otherwise the key is
// added after the last top-level key, before the first table.
func SetTopLevel(src []byte, key, literal string) ([]byte, error) {
	items, err := parse(src)
	if err != nil {
		return nil, err
	}
	line := key + " = " + literal
	insertAt, firstHeader := 0, -1
	for _, it := range items {
		if it.kind == header {
			firstHeader = it.start
			break
		}
		if slices.Equal(it.key, []string{key}) {
			return validate(splice(src, it.start, it.end, line))
		}
		insertAt = lineEnd(src, it.end-1)
	}
	switch {
	case insertAt > 0:
		return validate(splice(src, insertAt, insertAt, line+"\n"))
	case firstHeader >= 0:
		return validate(splice(src, firstHeader, firstHeader, line+"\n\n"))
	default:
		return validate(append(withNewline(src), line+"\n"...))
	}
}

// DeleteTopLevel removes key and its whole line(s) if it exists outside any
// table.
func DeleteTopLevel(src []byte, key string) ([]byte, error) {
	items, err := parse(src)
	if err != nil {
		return nil, err
	}
	for _, it := range items {
		if it.kind == header {
			break
		}
		if slices.Equal(it.key, []string{key}) {
			return validate(splice(src, lineStart(src, it.start), lineEnd(src, it.end-1), ""))
		}
	}
	return src, nil
}

// TopLevel returns the raw literal of a top-level key and whether it exists.
func TopLevel(src []byte, key string) (string, bool, error) {
	var doc map[string]any
	if err := toml.Unmarshal(src, &doc); err != nil {
		return "", false, err
	}
	v, ok := doc[key]
	if !ok {
		return "", false, nil
	}
	if s, isString := v.(string); isString {
		return s, true, nil
	}
	return fmt.Sprint(v), true, nil
}

// RemoveTable deletes the table at path, its sub-tables, their keys, and
// blank lines that follow them. Comments above the next table are kept.
func RemoveTable(src []byte, path ...string) ([]byte, error) {
	items, err := parse(src)
	if err != nil {
		return nil, err
	}
	type span struct{ start, end int }
	var spans []span
	for i := 0; i < len(items); i++ {
		it := items[i]
		if it.kind != header || !hasPrefix(it.table, path) {
			continue
		}
		end := it.end
		for i+1 < len(items) && items[i+1].kind == keyValue {
			i++
			end = lineEnd(src, items[i].end-1)
		}
		for end < len(src) && isBlankLine(src[end:lineEnd(src, end)]) {
			end = lineEnd(src, end)
		}
		if n := len(spans); n > 0 && it.start <= spans[n-1].end {
			spans[n-1].end = end
			continue
		}
		spans = append(spans, span{it.start, end})
	}
	// A table that runs to the end also takes the one blank line AppendBlock
	// puts before it, so append-then-remove restores the file exactly.
	if n := len(spans); n > 0 && spans[n-1].end == len(src) && spans[n-1].start > 0 {
		prev := lineStart(src, spans[n-1].start-1)
		if isBlankLine(src[prev:spans[n-1].start]) {
			spans[n-1].start = prev
		}
	}
	out := src
	for i := len(spans) - 1; i >= 0; i-- {
		out = splice(out, spans[i].start, spans[i].end, "")
	}
	return validate(out)
}

// AppendBlock appends text (one or more tables) after one blank line.
func AppendBlock(src []byte, text string) ([]byte, error) {
	out := withNewline(src)
	if len(out) > 0 {
		out = append(out, '\n')
	}
	return validate(append(out, strings.TrimRight(text, "\n")+"\n"...))
}

// String formats s as a TOML basic string.
func String(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		case '\r':
			b.WriteString(`\r`)
		default:
			if r < 0x20 || r == 0x7f || r == utf8.RuneError {
				b.WriteString(`\u` + fmt.Sprintf("%04X", r))
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

// Strings formats a TOML array of basic strings.
func Strings(ss ...string) string {
	quoted := make([]string, len(ss))
	for i, s := range ss {
		quoted[i] = String(s)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

// Int formats an integer literal.
func Int(n int) string { return strconv.Itoa(n) }

func splice(src []byte, start, end int, text string) []byte {
	out := make([]byte, 0, len(src)-(end-start)+len(text))
	out = append(out, src[:start]...)
	out = append(out, text...)
	return append(out, src[end:]...)
}

func withNewline(src []byte) []byte {
	if len(src) > 0 && src[len(src)-1] != '\n' {
		return append(slices.Clip(src), '\n')
	}
	return src
}

func isBlankLine(line []byte) bool { return len(bytes.TrimSpace(line)) == 0 }

func hasPrefix(path, prefix []string) bool {
	return len(path) >= len(prefix) && slices.Equal(path[:len(prefix)], prefix)
}

// validate refuses to hand back a document that no longer parses, so a bug
// here can never corrupt a user's config.
func validate(out []byte) ([]byte, error) {
	var v map[string]any
	if err := toml.Unmarshal(out, &v); err != nil {
		return nil, fmt.Errorf("edit produced invalid TOML (not written): %w", err)
	}
	return out, nil
}
