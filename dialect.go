package jsonschema

import (
	"fmt"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
)

// A Dialect is what CheckSchemas accepts in a family of schemas: the
// keywords allowed, and checks of a keyword's value. Validation itself is
// the library's (see jsonschema.go); a dialect only refuses, before use,
// schemas the validator would not check as their authors meant. Every
// field is optional. A Dialect must not be changed once a Set uses it;
// the Check functions may then run on several goroutines at once.
type Dialect struct {
	// Keywords adds keywords to those CheckSchemas accepts, with a check
	// of their value.
	Keywords map[string]Keyword

	// Allowed says which keywords CheckSchemas accepts beside those in
	// Keywords; nil accepts the package's Keywords list.
	Allowed func(keyword string) bool

	// ObjectSchemasOnly makes CheckSchemas refuse boolean schemas (other
	// than additionalProperties: false/true, which is a keyword form).
	ObjectSchemasOnly bool
}

// Keyword is the dialect's treatment of one keyword.
type Keyword struct {
	// Check runs when CheckSchemas meets the keyword in a schema, to
	// report a value of the wrong shape.
	Check func(c *CheckContext, schema map[string]any, value any)
}

func (s *Set) resolve(d *doc, ref string) (*doc, any, error) {
	u, err := url.Parse(ref)
	if err != nil {
		return nil, nil, fmt.Errorf("$ref %q: %w", ref, err)
	}
	target := d
	if u.Scheme != "" || u.Host != "" || u.Path != "" {
		abs := d.base.ResolveReference(u)
		k := *abs
		k.Fragment = ""
		target = s.byID[k.String()]
		if target == nil {
			target = s.byName[k.String()]
		}
		if target == nil {
			target = s.byName[path.Base(u.Path)]
		}
		if target == nil {
			return nil, nil, fmt.Errorf("$ref %q: no such schema in the set", ref)
		}
	}
	node := target.root
	if frag := u.Fragment; frag != "" {
		if !strings.HasPrefix(frag, "/") {
			return nil, nil, fmt.Errorf("$ref %q: only JSON pointer fragments are supported", ref)
		}
		for _, tok := range strings.Split(frag[1:], "/") {
			tok = strings.ReplaceAll(strings.ReplaceAll(tok, "~1", "/"), "~0", "~")
			switch n := node.(type) {
			case map[string]any:
				var ok bool
				if node, ok = n[tok]; !ok {
					return nil, nil, fmt.Errorf("$ref %q: %s not found", ref, tok)
				}
			case []any:
				i, err := strconv.Atoi(tok)
				if err != nil || i < 0 || i >= len(n) {
					return nil, nil, fmt.Errorf("$ref %q: %s not found", ref, tok)
				}
				node = n[i]
			default:
				return nil, nil, fmt.Errorf("$ref %q: %s not found", ref, tok)
			}
		}
	}
	return target, node, nil
}

// CheckContext is what a keyword's Check sees while CheckSchemas walks.
type CheckContext struct {
	w   *walker
	loc string // pointer of the keyword
}

// Pointer is the JSON pointer of the keyword in its document.
func (c *CheckContext) Pointer() string { return c.loc }

// Fail reports a problem with the keyword's value.
func (c *CheckContext) Fail(format string, a ...any) { c.FailAt(c.loc, format, a...) }

// FailAt reports a problem at another pointer of the document.
func (c *CheckContext) FailAt(pointer, format string, a ...any) {
	c.w.errs = append(c.w.errs, Error{Path: pointer, Keyword: lastToken(c.loc), Message: fmt.Sprintf(format, a...)})
}

// CheckSchemas walks the schemas at the given JSON pointers of document
// name, and every schema they contain or reference, before any validation:
// keywords the dialect does not allow are refused, each keyword's Check
// runs, references must resolve, and a schema must not reach itself through
// $ref, allOf, anyOf, oneOf, not, if, then or else alone (validating it
// would never reach a value). Errors carry pointers into the documents. It
// returns the pointers of the schemas it visited in document name.
func (s *Set) CheckSchemas(name string, pointers ...string) ([]string, error) {
	d := s.byName[name]
	if d == nil {
		return nil, fmt.Errorf("jsonschema: no schema %s in the set", name)
	}
	w := &walker{s: s, seen: map[string]*wnode{}, ix: s.refs()}
	for _, p := range pointers {
		node, err := lookupPointer(d.root, p)
		if err != nil {
			w.errs = append(w.errs, Error{Path: p, Keyword: "$ref", Message: err.Error()})
			continue
		}
		w.visit(d, p, node)
	}
	w.cycles()
	var visited []string
	for _, n := range w.order {
		if n.d == d {
			visited = append(visited, n.loc)
		}
	}
	sort.Strings(visited)
	if len(w.errs) > 0 {
		return visited, w.errs
	}
	return visited, nil
}

type wnode struct {
	d     *doc
	loc   string
	key   string
	next  []*wnode // reached without passing through a value
	color int
}

type walker struct {
	s     *Set
	seen  map[string]*wnode
	order []*wnode
	errs  ValidationError
	ix    *refIndex
}

func (w *walker) fail(loc, kw, format string, a ...any) {
	w.errs = append(w.errs, Error{Path: loc, Keyword: kw, Message: fmt.Sprintf(format, a...)})
}

func (w *walker) allowed(d *doc, k string) bool {
	dl := w.s.dialect
	if dl != nil {
		if _, ok := dl.Keywords[k]; ok {
			return true
		}
		if dl.Allowed != nil {
			return dl.Allowed(k)
		}
	}
	return keywords[k] || w.s.modern(d) && keywords2020[k]
}

// keywords2020 is the table behind Keywords2020.
var keywords2020 = map[string]bool{
	"$defs": true, "$anchor": true, "$dynamicRef": true, "$dynamicAnchor": true, "$recursiveRef": true,
	"$recursiveAnchor": true, "$vocabulary": true, "dependentRequired": true, "dependentSchemas": true,
	"prefixItems": true, "unevaluatedProperties": true, "unevaluatedItems": true, "minContains": true,
	"maxContains": true, "contentSchema": true, "deprecated": true,
}

// modern reports whether a document is read by draft 2019-09 or later:
// its $schema names one (or a meta-schema in the set that does), or it
// names none and the set's default draft is one.
func (s *Set) modern(d *doc) bool {
	m, ok := d.root.(map[string]any)
	if !ok {
		return false
	}
	sch, has := m["$schema"].(string)
	if !has {
		sch = s.opts.DefaultDraft
	}
	for range 8 { // a custom meta-schema names its own meta-schema
		if strings.Contains(sch, "/draft/2019-09/") || strings.Contains(sch, "/draft/2020-12/") {
			return true
		}
		if strings.Contains(sch, "json-schema.org/draft-0") || sch == "" {
			return false
		}
		u, err := url.Parse(sch)
		if err != nil {
			return false
		}
		u.Fragment = ""
		md := s.byID[u.String()]
		if md == nil {
			md = s.byName[u.String()]
		}
		if md == nil {
			return false
		}
		mm, ok := md.root.(map[string]any)
		if !ok {
			return false
		}
		sch, _ = mm["$schema"].(string)
	}
	return false
}

// visit walks one schema and returns its graph node.
func (w *walker) visit(d *doc, loc string, node any) *wnode {
	key := d.name + "#" + loc
	if n, ok := w.seen[key]; ok {
		return n
	}
	n := &wnode{d: d, loc: loc, key: key}
	w.seen[key] = n
	w.order = append(w.order, n)
	m, ok := node.(map[string]any)
	if !ok {
		if _, isBool := node.(bool); !isBool || (w.s.dialect != nil && w.s.dialect.ObjectSchemasOnly) {
			w.fail(loc, "schema", "a schema must be an object")
		}
		return n
	}
	var kwds map[string]Keyword
	if w.s.dialect != nil {
		kwds = w.s.dialect.Keywords
	}
	modern := w.s.modern(d)
	if r, has := m["$ref"]; has && !modern { // before 2019-09, siblings of $ref are ignored
		w.ref(d, n, loc, "$ref", r, kwds, m)
		return n
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		sub := m[k]
		kl := loc + "/" + escape(k)
		if !w.allowed(d, k) {
			w.fail(kl, k, "keyword %q is not supported", k)
			continue
		}
		switch k {
		case "$ref", "$dynamicRef", "$recursiveRef": // 2019-09 and later; checked there
			if modern {
				w.ref(d, n, loc, k, sub, kwds, m)
			}
			continue
		}
		if kw, ok := kwds[k]; ok && kw.Check != nil {
			kw.Check(&CheckContext{w: w, loc: kl}, m, sub)
		}
		switch k {
		case "unevaluatedProperties", "unevaluatedItems", "contentSchema":
			w.visit(d, kl, sub)
		case "prefixItems":
			if l, ok := sub.([]any); ok {
				for i, e := range l {
					w.visit(d, kl+"/"+strconv.Itoa(i), e)
				}
			}
		case "dependentSchemas", "$defs":
			if sm, ok := sub.(map[string]any); ok {
				for _, sk := range sortedKeys(sm) {
					if k == "dependentSchemas" {
						n.next = append(n.next, w.visit(d, kl+"/"+escape(sk), sm[sk]))
					} else {
						w.visit(d, kl+"/"+escape(sk), sm[sk])
					}
				}
			}
		case "properties", "patternProperties", "definitions":
			if sm, ok := sub.(map[string]any); ok {
				for _, sk := range sortedKeys(sm) {
					w.visit(d, kl+"/"+escape(sk), sm[sk])
				}
			}
		case "dependencies":
			if sm, ok := sub.(map[string]any); ok {
				for _, sk := range sortedKeys(sm) {
					if _, isList := sm[sk].([]any); !isList {
						w.visit(d, kl+"/"+escape(sk), sm[sk])
					}
				}
			}
		case "items":
			if l, ok := sub.([]any); ok {
				for i, e := range l {
					w.visit(d, kl+"/"+strconv.Itoa(i), e)
				}
			} else {
				w.visit(d, kl, sub)
			}
		case "allOf", "anyOf", "oneOf":
			if l, ok := sub.([]any); ok {
				for i, e := range l {
					n.next = append(n.next, w.visit(d, kl+"/"+strconv.Itoa(i), e))
				}
			}
		case "not", "if", "then", "else":
			n.next = append(n.next, w.visit(d, kl, sub))
		case "additionalProperties", "additionalItems":
			if _, isBool := sub.(bool); !isBool {
				w.visit(d, kl, sub)
			}
		case "propertyNames", "contains":
			w.visit(d, kl, sub)
		}
	}
	return n
}

// ref checks a reference keyword (k is $ref, $dynamicRef or
// $recursiveRef) and visits its static target as reached in place.
func (w *walker) ref(d *doc, n *wnode, loc, k string, r any, kwds map[string]Keyword, m map[string]any) {
	before := len(w.errs)
	if kw, ok := kwds[k]; ok && kw.Check != nil {
		kw.Check(&CheckContext{w: w, loc: loc + "/" + k}, m, r)
	}
	if len(w.errs) > before {
		return
	}
	ref, ok := r.(string)
	if !ok {
		w.fail(loc+"/"+k, k, "%s must be a string", k)
		return
	}
	td, target, ptr, err := w.s.resolveAt(w.ix, d, loc, ref)
	if err != nil {
		w.fail(loc+"/"+k, k, "%v", err)
		return
	}
	n.next = append(n.next, w.visit(td, ptr, target))
}

// cycles reports each schema that leads back to a schema being visited
// without passing through a value.
func (w *walker) cycles() {
	const (
		white = iota
		grey
		black
	)
	var visit func(n *wnode) bool
	visit = func(n *wnode) bool {
		switch n.color {
		case grey:
			return true
		case black:
			return false
		}
		n.color = grey
		for _, x := range n.next {
			if visit(x) {
				n.color = black
				w.fail(n.loc, "$ref", "the schema refers back to itself without passing through a value")
				return false
			}
		}
		n.color = black
		return false
	}
	nodes := append([]*wnode(nil), w.order...)
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].key < nodes[j].key })
	for _, n := range nodes {
		visit(n)
	}
}

func sortedKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func lastToken(p string) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return strings.ReplaceAll(strings.ReplaceAll(p[i+1:], "~1", "/"), "~0", "~")
	}
	return p
}

// lookupPointer resolves a JSON pointer in a decoded document.
func lookupPointer(root any, p string) (any, error) {
	if p == "" {
		return root, nil
	}
	if !strings.HasPrefix(p, "/") {
		return nil, fmt.Errorf("%q is not a JSON pointer", p)
	}
	node := root
	for _, tok := range strings.Split(p[1:], "/") {
		tok = strings.ReplaceAll(strings.ReplaceAll(tok, "~1", "/"), "~0", "~")
		switch n := node.(type) {
		case map[string]any:
			var ok bool
			if node, ok = n[tok]; !ok {
				return nil, fmt.Errorf("%s: %q not found", p, tok)
			}
		case []any:
			i, err := strconv.Atoi(tok)
			if err != nil || i < 0 || i >= len(n) {
				return nil, fmt.Errorf("%s: %q not found", p, tok)
			}
			node = n[i]
		default:
			return nil, fmt.Errorf("%s: %q not found", p, tok)
		}
	}
	return node, nil
}
