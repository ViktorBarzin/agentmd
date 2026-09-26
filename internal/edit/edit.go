// Package edit saves agent files: atomically, through symlinks to the real
// file, and for embedded files by replacing only one field's string.
package edit

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/ViktorBarzin/agentmd/internal/discover"
	"github.com/ViktorBarzin/agentmd/internal/model"
)

// Conflict means the file changed on disk after the editor loaded it.
type Conflict struct {
	Current string
	Hash    string
}

func (c *Conflict) Error() string { return "the file changed on disk since it was opened" }

// ErrReadOnly is returned when the file cannot be edited here.
var ErrReadOnly = errors.New("read-only")

// Target is where a save lands: the real file, and the field for an embedded
// file.
func Target(f *model.File) (string, error) {
	p := f.Path
	if f.Field == "" && f.IsLink {
		p = f.RealPath
	}
	real, err := filepath.EvalSymlinks(p)
	if err != nil {
		return "", err
	}
	return real, nil
}

// Current reads what the file (or field) holds now.
func Current(f *model.File) (string, error) {
	real, err := Target(f)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(real)
	if err != nil {
		return "", err
	}
	if f.Field == "" {
		return string(data), nil
	}
	v, ok := discover.EmbeddedField(real, data, f.Field)
	if !ok {
		return "", fmt.Errorf("%s has no %q field any more", real, f.Field)
	}
	return v, nil
}

// Save writes content if the file still has baseHash, and returns the path
// written.
func Save(f *model.File, content, baseHash string) (string, error) {
	if !f.Access.Writable {
		reason := f.Access.Reason
		if reason == "" {
			reason = "not writable"
		}
		return "", fmt.Errorf("%w: %s", ErrReadOnly, reason)
	}
	real, err := Target(f)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(real)
	if err != nil {
		return "", err
	}
	if f.Field == "" {
		if h := discover.Hash(string(data)); h != baseHash {
			return "", &Conflict{Current: string(data), Hash: h}
		}
		return real, writeAtomic(real, []byte(content))
	}
	if !strings.HasSuffix(real, ".json") {
		return "", fmt.Errorf("%w: only JSON fields can be edited", ErrReadOnly)
	}
	cur, ok := discover.EmbeddedField(real, data, f.Field)
	if !ok {
		return "", fmt.Errorf("%s has no %q field any more", real, f.Field)
	}
	if h := discover.Hash(cur); h != baseHash {
		return "", &Conflict{Current: cur, Hash: h}
	}
	out, err := ReplaceJSONField(data, f.Field, content)
	if err != nil {
		return "", err
	}
	return real, writeAtomic(real, out)
}

// writeAtomic replaces path with data via a temp file in the same directory,
// keeping the mode. When the directory is not writable it writes in place.
func writeAtomic(path string, data []byte) error {
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".agentmd-*")
	if err != nil {
		if os.IsPermission(err) {
			return os.WriteFile(path, data, st.Mode().Perm())
		}
		return err
	}
	name := tmp.Name()
	fail := func(e error) error {
		tmp.Close()
		os.Remove(name)
		return e
	}
	if _, err := tmp.Write(data); err != nil {
		return fail(err)
	}
	if err := tmp.Sync(); err != nil {
		return fail(err)
	}
	if err := tmp.Chmod(st.Mode().Perm()); err != nil {
		return fail(err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

// ReplaceJSONField returns data with the top-level string field replaced by
// value, leaving every other byte as it was. The new string follows the old
// one's escaping style.
func ReplaceJSONField(data []byte, field, value string) ([]byte, error) {
	start, end, err := findField(data, field)
	if err != nil {
		return nil, err
	}
	old := string(data[start:end])
	enc := encodeString(value, strings.Contains(old, `\u`) && hasEscapedNonASCII(old), strings.Contains(old, `\/`))
	out := make([]byte, 0, len(data)-(end-start)+len(enc))
	out = append(out, data[:start]...)
	out = append(out, enc...)
	out = append(out, data[end:]...)
	return out, nil
}

// findField returns the byte span of the string value of a top-level key.
func findField(data []byte, field string) (int, int, error) {
	s := &scanner{b: data}
	s.ws()
	if !s.eat('{') {
		return 0, 0, errors.New("the settings file is not a JSON object")
	}
	for {
		s.ws()
		if s.eat('}') {
			return 0, 0, fmt.Errorf("no %q field", field)
		}
		kStart := s.i
		if err := s.str(); err != nil {
			return 0, 0, err
		}
		key, err := unquote(data[kStart:s.i])
		if err != nil {
			return 0, 0, err
		}
		s.ws()
		if !s.eat(':') {
			return 0, 0, errors.New("malformed JSON: missing ':'")
		}
		s.ws()
		vStart := s.i
		isString := s.peek() == '"'
		if err := s.value(); err != nil {
			return 0, 0, err
		}
		if key == field {
			if !isString {
				return 0, 0, fmt.Errorf("%q is not a string", field)
			}
			return vStart, s.i, nil
		}
		s.ws()
		if s.eat(',') {
			continue
		}
		if s.eat('}') {
			return 0, 0, fmt.Errorf("no %q field", field)
		}
		return 0, 0, errors.New("malformed JSON: expected ',' or '}'")
	}
}

type scanner struct {
	b []byte
	i int
}

func (s *scanner) peek() byte {
	if s.i < len(s.b) {
		return s.b[s.i]
	}
	return 0
}

func (s *scanner) eat(c byte) bool {
	if s.peek() == c {
		s.i++
		return true
	}
	return false
}

func (s *scanner) ws() {
	for s.i < len(s.b) && strings.IndexByte(" \t\r\n", s.b[s.i]) >= 0 {
		s.i++
	}
}

func (s *scanner) str() error {
	if !s.eat('"') {
		return errors.New("malformed JSON: expected a string")
	}
	for s.i < len(s.b) {
		switch s.b[s.i] {
		case '\\':
			s.i += 2
		case '"':
			s.i++
			return nil
		default:
			s.i++
		}
	}
	return errors.New("malformed JSON: unterminated string")
}

func (s *scanner) value() error {
	switch c := s.peek(); {
	case c == '"':
		return s.str()
	case c == '{' || c == '[':
		depth := 0
		for s.i < len(s.b) {
			switch s.b[s.i] {
			case '"':
				if err := s.str(); err != nil {
					return err
				}
				continue
			case '{', '[':
				depth++
			case '}', ']':
				depth--
				if depth == 0 {
					s.i++
					return nil
				}
			}
			s.i++
		}
		return errors.New("malformed JSON: unterminated value")
	default:
		for s.i < len(s.b) && strings.IndexByte(",}] \t\r\n", s.b[s.i]) < 0 {
			s.i++
		}
		return nil
	}
}

func unquote(raw []byte) (string, error) {
	var out strings.Builder
	s := string(raw[1 : len(raw)-1])
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' {
			out.WriteByte(s[i])
			continue
		}
		i++
		if i >= len(s) {
			return "", errors.New("bad escape")
		}
		switch s[i] {
		case 'n':
			out.WriteByte('\n')
		case 't':
			out.WriteByte('\t')
		case 'r':
			out.WriteByte('\r')
		case 'b':
			out.WriteByte('\b')
		case 'f':
			out.WriteByte('\f')
		case 'u':
			if i+4 >= len(s)+1 {
				return "", errors.New("bad escape")
			}
			var r rune
			if _, err := fmt.Sscanf(s[i+1:i+5], "%04x", &r); err != nil {
				return "", err
			}
			out.WriteRune(r)
			i += 4
		default:
			out.WriteByte(s[i])
		}
	}
	return out.String(), nil
}

func hasEscapedNonASCII(raw string) bool {
	for i := 0; i+5 < len(raw); i++ {
		if raw[i] == '\\' && raw[i+1] == 'u' {
			var r rune
			if _, err := fmt.Sscanf(raw[i+2:i+6], "%04x", &r); err == nil && r >= 0x80 {
				return true
			}
		}
	}
	return false
}

// encodeString writes a JSON string. asciiOnly escapes non-ASCII as \uXXXX;
// escapeSlash writes "/" as "\/". HTML characters are never escaped.
func encodeString(v string, asciiOnly, escapeSlash bool) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range v {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\b':
			b.WriteString(`\b`)
		case r == '\f':
			b.WriteString(`\f`)
		case r == '/' && escapeSlash:
			b.WriteString(`\/`)
		case r < 0x20 || r == 0x2028 || r == 0x2029:
			fmt.Fprintf(&b, `\u%04x`, r)
		case r >= 0x80 && asciiOnly:
			if r > 0xFFFF {
				r -= 0x10000
				fmt.Fprintf(&b, `\u%04x\u%04x`, 0xD800+(r>>10), 0xDC00+(r&0x3FF))
			} else {
				fmt.Fprintf(&b, `\u%04x`, r)
			}
		case r == utf8.RuneError:
			b.WriteString(`�`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
