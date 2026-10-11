// Package jsonschema validates JSON documents against families of JSON
// Schema documents (the Open Charge Alliance's OCPP and OSCP schemas) with
// github.com/santhosh-tekuri/jsonschema/v6, and reports each failure as a
// JSON pointer, the schema keyword and a short message of our own.
//
// A Set holds a family of schema documents that refer to each other by
// relative file name ("phase-indicator.json") or JSON pointer
// ("#/definitions/X", "other.json#/definitions/Y"). References are resolved
// against the referring document's $id; when no document carries the
// resolved $id, the document added under that whole URL is used, then the
// one added under the same file name (some published sets name a file
// differently from its $id). Each document is
// validated by the draft its $schema names (draft-04 to 2020-12; draft-07
// when it names none).
//
// What this package adds to the library: the documents are kept in memory
// as added (corrections can be applied to a decoded copy before Add), the
// formats date-time, date, time, email, uri, uri-reference, ipv4, ipv6 and
// uuid are checked with our messages and the library's other formats are
// not checked, failures are listed in a stable order (see Translate), and
// CheckSchemas walks the schemas before use to refuse keywords outside a
// dialect (dialect.go).
//
// Instances are decoded with json.Number (Decode), so integers keep their
// precision and "integer" accepts any number whose fraction is zero.
package jsonschema

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"net/netip"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	v6 "github.com/santhosh-tekuri/jsonschema/v6"
)

// Set is a family of schema documents. It is safe for concurrent use once
// every document is added.
type Set struct {
	byName map[string]*doc // file name ("register.json")
	byID   map[string]*doc // absolute $id without fragment

	dialect *Dialect // nil: no keyword refusal in CheckSchemas

	mu       sync.Mutex
	compiler *v6.Compiler          // created at the first compile; the set is frozen then
	compiled map[string]*v6.Schema // by file name
	byURL    map[string]any        // document roots by the URL the library loaded them from
}

type doc struct {
	name string
	base *url.URL // $id, or the file name
	root any
}

// NewSet returns an empty set.
func NewSet() *Set {
	return &Set{byName: map[string]*doc{}, byID: map[string]*doc{}, compiled: map[string]*v6.Schema{}, byURL: map[string]any{}}
}

// NewDialectSet returns an empty set whose CheckSchemas applies the
// dialect.
func NewDialectSet(d *Dialect) *Set {
	s := NewSet()
	s.dialect = d
	return s
}

// Add parses a schema document and adds it under name (its file name).
func (s *Set) Add(name string, raw []byte) error {
	root, err := decode(raw)
	if err != nil {
		return fmt.Errorf("schema %s: %w", name, err)
	}
	return s.add(name, root)
}

// AddValue adds a schema document that is already decoded (map[string]any,
// []any, string, bool, nil and json.Number values), for instance one read
// from YAML.
func (s *Set) AddValue(name string, root any) error {
	b, err := json.Marshal(root)
	if err != nil {
		return fmt.Errorf("schema %s: %w", name, err)
	}
	return s.Add(name, b)
}

func (s *Set) add(name string, root any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.compiler != nil {
		return fmt.Errorf("schema %s: the set is in use and cannot take more documents", name)
	}
	if _, dup := s.byName[name]; dup {
		return fmt.Errorf("schema %s is added twice", name)
	}
	base, _ := url.Parse(name)
	d := &doc{name: name, base: base, root: root}
	if m, ok := root.(map[string]any); ok {
		if id, ok := m["$id"].(string); ok && id != "" {
			if u, err := url.Parse(id); err == nil {
				if u.IsAbs() {
					d.base = u
				} else {
					d.base = base.ResolveReference(u)
				}
				k := *d.base
				k.Fragment = ""
				s.byID[k.String()] = d
			}
		}
	}
	s.byName[name] = d
	return nil
}

// Names lists the documents in the set.
func (s *Set) Names() []string {
	out := make([]string, 0, len(s.byName))
	for n := range s.byName {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// ValidateJSON decodes data and validates it against the named document.
func (s *Set) ValidateJSON(name string, data []byte) error {
	v, err := decode(data)
	if err != nil {
		return ValidationError{{Keyword: "json", Message: "not JSON: " + err.Error()}}
	}
	return s.Validate(name, v)
}

// Validate checks an instance decoded with Decode (or any value built of
// map[string]any, []any, string, bool, nil, json.Number and float64). A
// failed check is a ValidationError; a schema that cannot be compiled is
// another error.
func (s *Set) Validate(name string, instance any) error {
	sch, err := s.Compile(name)
	if err != nil {
		return err
	}
	return s.translate(sch.Validate(instance), instance)
}

// Compile compiles the named document (and what it references) once, so
// that a schema the library refuses is reported before any validation.
// The first call freezes the set.
func (s *Set) Compile(name string) (*v6.Schema, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sch, ok := s.compiled[name]; ok {
		return sch, nil
	}
	if s.byName[name] == nil {
		return nil, fmt.Errorf("no schema %s in the set", name)
	}
	if s.compiler == nil {
		s.compiler = s.newCompiler()
	}
	sch, err := s.compiler.Compile(docURL(name))
	if err != nil {
		return nil, fmt.Errorf("schema %s: %w", name, err)
	}
	s.compiled[name] = sch
	return sch, nil
}

// docURL is the URL a document is given to the library under.
func docURL(name string) string { return "mem:///" + url.PathEscape(name) }

func (s *Set) newCompiler() *v6.Compiler {
	c := v6.NewCompiler()
	c.DefaultDraft(v6.Draft7)
	c.AssertFormat()
	for _, f := range ignoredFormats {
		c.RegisterFormat(&v6.Format{Name: f, Validate: func(any) error { return nil }})
	}
	for name, check := range ownFormats {
		check := check
		c.RegisterFormat(&v6.Format{Name: name, Validate: func(v any) error {
			if str, ok := v.(string); ok {
				if msg := check(str); msg != "" {
					return errors.New(str + " " + msg)
				}
			}
			return nil
		}})
	}
	c.UseLoader(loader{s})
	return c
}

// loader hands the library the set's documents: by name for the URLs Compile
// asks for, then by $id, then by a name equal to the whole URL, then by file
// name (a published set may name a file differently from its $id).
type loader struct{ s *Set }

func (l loader) Load(u string) (any, error) {
	s := l.s
	var d *doc
	if strings.HasPrefix(u, "mem:///") {
		if n, err := url.PathUnescape(strings.TrimPrefix(u, "mem:///")); err == nil {
			d = s.byName[n]
		}
	}
	if d == nil {
		d = s.byID[u]
	}
	if d == nil {
		d = s.byName[u] // a document added under its full URL
	}
	if d == nil {
		if pu, err := url.Parse(u); err == nil {
			d = s.byName[path.Base(pu.Path)]
		}
	}
	if d == nil {
		return nil, fmt.Errorf("%s: no such schema in the set", u)
	}
	s.byURL[u] = d.root
	return d.root, nil
}

// Decode parses JSON the way Validate expects instances (numbers as
// json.Number); trailing data is an error.
func Decode(data []byte) (any, error) { return decode(data) }

func decode(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	if dec.More() {
		return nil, errors.New("data after the JSON value")
	}
	var extra any
	if err := dec.Decode(&extra); err == nil {
		return nil, errors.New("data after the JSON value")
	}
	return v, nil
}

// ignoredFormats are the formats the library knows that this package does
// not check ("regex" cannot be replaced; no OCA schema uses it).
var ignoredFormats = []string{"json-pointer", "relative-json-pointer", "duration", "period", "hostname",
	"iri", "iri-reference", "uri-template", "semver"}

var (
	dateTimeRE = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}[Tt]\d{2}:\d{2}:\d{2}(\.\d+)?([Zz]|[+-]\d{2}:\d{2})$`)
	timeRE     = regexp.MustCompile(`^\d{2}:\d{2}:\d{2}(\.\d+)?([Zz]|[+-]\d{2}:\d{2})$`)
	uuidRE     = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
)

// ownFormats return why a string is not in the format, or "".
var ownFormats = map[string]func(s string) string{
	"date-time": func(s string) string {
		if !dateTimeRE.MatchString(s) {
			return "is not an RFC 3339 date-time with a time zone"
		}
		if _, err := time.Parse("2006-01-02", s[:10]); err != nil || !validTime(s[11:]) {
			return "is not a valid date-time"
		}
		return ""
	},
	"date": func(s string) string {
		if _, err := time.Parse("2006-01-02", s); err != nil {
			return "is not an RFC 3339 full-date"
		}
		return ""
	},
	"time": func(s string) string {
		if !timeRE.MatchString(s) || !validTime(s) {
			return "is not an RFC 3339 full-time"
		}
		return ""
	},
	"email": func(s string) string {
		if _, err := mail.ParseAddress(s); err != nil || strings.ContainsAny(s, "<> ") {
			return "is not an e-mail address"
		}
		return ""
	},
	"ipv4": func(s string) string {
		if a, err := netip.ParseAddr(s); err != nil || !a.Is4() {
			return "is not an IPv4 address"
		}
		return ""
	},
	"ipv6": func(s string) string {
		if a, err := netip.ParseAddr(s); err != nil || !a.Is6() || a.Zone() != "" {
			return "is not an IPv6 address"
		}
		return ""
	},
	"uri": func(s string) string {
		if u, err := url.Parse(s); err != nil || !u.IsAbs() {
			return "is not an absolute URI"
		}
		return ""
	},
	"uri-reference": func(s string) string {
		if _, err := url.Parse(s); err != nil {
			return "is not a URI reference"
		}
		return ""
	},
	"uuid": func(s string) string {
		if !uuidRE.MatchString(s) {
			return "is not a UUID"
		}
		return ""
	},
}

// validTime checks the ranges of an RFC 3339 full-time that matched timeRE
// (or the time part of a date-time): hours below 24, minutes below 60, an
// offset of at most 23:59, and second 60 only where the time is 23:59 in
// UTC (a leap second, RFC 3339 section 5.7).
func validTime(s string) bool {
	two := func(i int) int { return int(s[i]-'0')*10 + int(s[i+1]-'0') }
	h, m, sec := two(0), two(3), two(6)
	if h > 23 || m > 59 || sec > 60 {
		return false
	}
	off := 0
	if z := s[len(s)-1]; z != 'Z' && z != 'z' {
		o := s[len(s)-6:]
		oh, om := int(o[1]-'0')*10+int(o[2]-'0'), int(o[4]-'0')*10+int(o[5]-'0')
		if oh > 23 || om > 59 {
			return false
		}
		off = oh*60 + om
		if o[0] == '-' {
			off = -off
		}
	}
	if sec == 60 {
		utc := ((h*60+m-off)%1440 + 1440) % 1440
		return utc == 23*60+59
	}
	return true
}

// Keywords are the draft-07 keywords (and $schema, $id, $comment, title,
// description, default, examples, definitions, readOnly, writeOnly and
// contentMediaType/contentEncoding, which carry no check) that a dialect
// accepts by default in CheckSchemas.
var Keywords = map[string]bool{
	"type": true, "enum": true, "const": true, "properties": true, "required": true, "additionalProperties": true,
	"patternProperties": true, "propertyNames": true, "minProperties": true, "maxProperties": true, "dependencies": true,
	"items": true, "additionalItems": true, "contains": true, "minItems": true, "maxItems": true, "uniqueItems": true,
	"minimum": true, "maximum": true, "exclusiveMinimum": true, "exclusiveMaximum": true, "multipleOf": true,
	"minLength": true, "maxLength": true, "pattern": true, "format": true, "allOf": true, "anyOf": true, "oneOf": true,
	"not": true, "if": true, "then": true, "else": true, "$ref": true,
	"$schema": true, "$id": true, "$comment": true, "title": true, "description": true, "default": true, "examples": true,
	"definitions": true, "readOnly": true, "writeOnly": true, "contentMediaType": true, "contentEncoding": true,
}
