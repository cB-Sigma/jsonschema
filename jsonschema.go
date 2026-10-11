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
// as added (corrections can be applied to a decoded copy before Add),
// every format of JSON Schema 2020-12 is checked by our own checkers with
// our messages (formats.go, uri.go, idna.go), patterns are ECMA-262
// regular expressions (ecma.go), failures are listed in a stable order
// (see Translate), and CheckSchemas walks the schemas before use to refuse
// keywords outside a dialect (dialect.go).
//
// format is an assertion in drafts 4, 6 and 7 and an annotation in drafts
// 2019-09 and 2020-12, as those drafts say; Options.AssertFormat asserts it
// in every draft.
//
// Instances are decoded with json.Number (Decode), so integers keep their
// precision and "integer" accepts any number whose fraction is zero.
package jsonschema

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"sort"
	"strings"
	"sync"

	v6 "github.com/santhosh-tekuri/jsonschema/v6"
)

// Set is a family of schema documents. It is safe for concurrent use once
// every document is added.
type Set struct {
	byName map[string]*doc // file name ("register.json")
	byID   map[string]*doc // absolute $id without fragment

	dialect *Dialect // nil: no keyword refusal in CheckSchemas
	opts    Options

	mu       sync.Mutex
	compiler *v6.Compiler          // created at the first compile; the set is frozen then
	compiled map[string]*v6.Schema // by file name
	byURL    map[string]any        // document roots by the URL the library loaded them from
}

type doc struct {
	name  string
	base  *url.URL // $id, or the file name
	idKey string   // "$id", or "id" in draft-04
	root  any
}

// Options choose how a Set made by NewSet validates. The zero value gives
// the defaults each field describes.
type Options struct {
	// Dialect, when set, is applied by CheckSchemas: keywords outside it
	// are refused and its keyword checks run. nil makes CheckSchemas
	// accept the keywords Keywords lists (and Keywords2020 in drafts
	// 2019-09 and later). The set keeps the pointer; the caller must not
	// change the dialect afterwards.
	Dialect *Dialect

	// AssertFormat makes "format" an assertion in every draft. Without
	// it, format is an assertion in drafts 4, 6 and 7, and an annotation
	// in drafts 2019-09 and 2020-12 unless the document's meta-schema
	// requires the format-assertion vocabulary (2020-12) or the format
	// vocabulary (2019-09). Unknown formats are never asserted.
	AssertFormat bool

	// AssertContent makes contentEncoding (base64) and contentMediaType
	// (application/json) assertions, and contentSchema with them (2019-09
	// and later). Without it they are annotations.
	AssertContent bool

	// GoRegexp reads "pattern", "patternProperties" and format "regex"
	// with Go's regexp syntax (RE2), as v0.1 did, instead of ECMA-262
	// (CompileRegexp).
	GoRegexp bool

	// DefaultDraft is the meta-schema URL of the draft that documents
	// without $schema are read by, one of the URLs of drafts 4, 6, 7,
	// 2019-09 and 2020-12 ("https://json-schema.org/draft/2020-12/schema").
	// Empty means draft-07, as in v0.1 and v0.2. Another value makes
	// Compile fail.
	DefaultDraft string

	// Draft4Integer refuses, in draft-04 documents, a number written with
	// a fraction or an exponent (1.0, 1e2) where "type" allows integer but
	// not number, as draft-04 defines integer. Without it a number whose
	// value is whole is an integer in every draft.
	Draft4Integer bool
}

// drafts are the meta-schema URLs DefaultDraft takes, without "#".
var drafts = map[string]*v6.Draft{
	"http://json-schema.org/draft-04/schema":       v6.Draft4,
	"http://json-schema.org/draft-06/schema":       v6.Draft6,
	"http://json-schema.org/draft-07/schema":       v6.Draft7,
	"https://json-schema.org/draft/2019-09/schema": v6.Draft2019,
	"https://json-schema.org/draft/2020-12/schema": v6.Draft2020,
}

// NewSet returns an empty set with the options; the zero Options assert
// format in drafts 4, 6 and 7 only (see Options.AssertFormat) and refuse
// no keyword in CheckSchemas.
func NewSet(o Options) *Set {
	return &Set{byName: map[string]*doc{}, byID: map[string]*doc{}, compiled: map[string]*v6.Schema{}, byURL: map[string]any{},
		dialect: o.Dialect, opts: o}
}

// prefixed gives an error returned by an exported function the package
// prefix, keeping the error itself reachable through errors.Is and As.
func prefixed(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("jsonschema: %w", err)
}

// Add parses a schema document and adds it under name (its file name).
func (s *Set) Add(name string, raw []byte) error {
	root, err := decode(raw)
	if err != nil {
		return fmt.Errorf("jsonschema: schema %s: %w", name, err)
	}
	return prefixed(s.add(name, root))
}

// AddValue adds a schema document that is already decoded (map[string]any,
// []any, string, bool, nil and json.Number values), for instance one read
// from YAML.
func (s *Set) AddValue(name string, root any) error {
	b, err := json.Marshal(root)
	if err != nil {
		return fmt.Errorf("jsonschema: schema %s: %w", name, err)
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
	d := &doc{name: name, base: base, root: root, idKey: "$id"}
	if s.readsAsDraft4(root) {
		d.idKey = "id" // draft-04 names it "id"
	}
	if m, ok := root.(map[string]any); ok {
		if id, ok := m[d.idKey].(string); ok && id != "" {
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

// ValidateGo validates any Go value (a struct, a map, a slice) as the JSON
// that encoding/json makes of it, so struct tags and MarshalJSON methods
// apply. A value that cannot be encoded is a ValidationError with keyword
// "json".
func (s *Set) ValidateGo(name string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return ValidationError{{Keyword: "json", Message: "cannot be encoded as JSON: " + err.Error()}}
	}
	return s.ValidateJSON(name, b)
}

// Validate checks an instance decoded with Decode (or any value built of
// map[string]any, []any, string, bool, nil, json.Number and float64). A
// failed check is a ValidationError; a schema that cannot be compiled is
// another error.
func (s *Set) Validate(name string, instance any) error {
	sch, err := s.compile(name)
	if err != nil {
		return prefixed(err)
	}
	return s.translate(sch.Validate(instance), instance)
}

// Compile compiles the named document (and what it references) once, so
// that a schema the validator refuses is reported before any validation;
// later validations reuse the result. The first call freezes the set: Add
// fails after it.
func (s *Set) Compile(name string) error {
	_, err := s.compile(name)
	return prefixed(err)
}

// compile is Compile, returning the library's compiled schema.
func (s *Set) compile(name string) (*v6.Schema, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sch, ok := s.compiled[name]; ok {
		return sch, nil
	}
	if s.byName[name] == nil {
		return nil, fmt.Errorf("no schema %s in the set", name)
	}
	if s.compiler == nil {
		c, err := s.newCompiler()
		if err != nil {
			return nil, err
		}
		s.compiler = c
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

func (s *Set) newCompiler() (*v6.Compiler, error) {
	c := v6.NewCompiler()
	c.DefaultDraft(v6.Draft7)
	if s.opts.DefaultDraft != "" {
		d, ok := drafts[strings.TrimSuffix(s.opts.DefaultDraft, "#")]
		if !ok {
			return nil, fmt.Errorf("default draft %q is not a meta-schema URL of drafts 4 to 2020-12", s.opts.DefaultDraft)
		}
		c.DefaultDraft(d)
	}
	if s.opts.Draft4Integer {
		c.RegisterVocabulary(draft4IntVocab)
	}
	if s.opts.AssertFormat {
		c.AssertFormat()
	}
	if s.opts.AssertContent {
		c.AssertContent()
	}
	if !s.opts.GoRegexp {
		c.UseRegexpEngine(func(src string) (v6.Regexp, error) { return CompileRegexp(src) })
	}
	for _, f := range ignoredFormats {
		c.RegisterFormat(&v6.Format{Name: f, Validate: func(any) error { return nil }})
	}
	for name, check := range checkers {
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
	return c, nil
}

// readsAsDraft4 reports whether a document is read by draft-04: it names
// draft-04, or names no draft and draft-04 is the set's default.
func (s *Set) readsAsDraft4(root any) bool {
	if isDraft4(root) {
		return true
	}
	m, ok := root.(map[string]any)
	if !ok {
		return false
	}
	if _, has := m["$schema"]; has {
		return false
	}
	return strings.TrimSuffix(s.opts.DefaultDraft, "#") == draft4Meta
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
	root := withFormatAssertion(d.root)
	if s.opts.Draft4Integer && s.readsAsDraft4(root) {
		root = markDraft4Integers(root)
	}
	return root, nil
}

const formatAssertionVocab = "https://json-schema.org/draft/2020-12/vocab/format-assertion"

// withFormatAssertion gives the library a meta-schema that lists the
// 2020-12 format-assertion vocabulary as optional (false) as one that
// requires it: this package implements the vocabulary, and an
// implementation that does applies it either way. The document itself is
// not changed.
func withFormatAssertion(root any) any {
	m, ok := root.(map[string]any)
	if !ok {
		return root
	}
	voc, ok := m["$vocabulary"].(map[string]any)
	if !ok || voc[formatAssertionVocab] != false {
		return root
	}
	m2 := make(map[string]any, len(m))
	for k, v := range m {
		m2[k] = v
	}
	voc2 := make(map[string]any, len(voc))
	for k, v := range voc {
		voc2[k] = v
	}
	voc2[formatAssertionVocab] = true
	m2["$vocabulary"] = voc2
	return m2
}

// Decode parses JSON the way Validate expects instances (numbers as
// json.Number); trailing data is an error.
func Decode(data []byte) (any, error) {
	v, err := decode(data)
	return v, prefixed(err)
}

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

// Keywords returns, sorted, the draft-07 keywords (and $schema, $id,
// $comment, title, description, default, examples, definitions, readOnly,
// writeOnly and contentMediaType/contentEncoding, which carry no check)
// that CheckSchemas accepts by default. The slice is the caller's.
func Keywords() []string { return sortedNames(keywords) }

// Keywords2020 returns, sorted, the keywords of drafts 2019-09 and 2020-12
// that are not in Keywords. Without a dialect's Allowed, CheckSchemas
// accepts them in documents of those drafts. The slice is the caller's.
func Keywords2020() []string { return sortedNames(keywords2020) }

func sortedNames(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// keywords is the table behind Keywords.
var keywords = map[string]bool{
	"type": true, "enum": true, "const": true, "properties": true, "required": true, "additionalProperties": true,
	"patternProperties": true, "propertyNames": true, "minProperties": true, "maxProperties": true, "dependencies": true,
	"items": true, "additionalItems": true, "contains": true, "minItems": true, "maxItems": true, "uniqueItems": true,
	"minimum": true, "maximum": true, "exclusiveMinimum": true, "exclusiveMaximum": true, "multipleOf": true,
	"minLength": true, "maxLength": true, "pattern": true, "format": true, "allOf": true, "anyOf": true, "oneOf": true,
	"not": true, "if": true, "then": true, "else": true, "$ref": true,
	"$schema": true, "$id": true, "$comment": true, "title": true, "description": true, "default": true, "examples": true,
	"definitions": true, "readOnly": true, "writeOnly": true, "contentMediaType": true, "contentEncoding": true,
}
