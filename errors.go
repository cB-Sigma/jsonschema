package jsonschema

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"sort"
	"strconv"
	"strings"

	v6 "github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

// Error is one failed check.
type Error struct {
	// Path is the JSON pointer of the failing value in the instance.
	Path string
	// Keyword is the schema keyword that failed ("false" for a false
	// schema, "json" for a document that is not JSON).
	Keyword string
	Message string
}

func (e Error) String() string {
	p := e.Path
	if p == "" {
		p = "/"
	}
	return p + ": " + e.Message
}

// ValidationError lists every failed check of one validation.
type ValidationError []Error

func (v ValidationError) Error() string {
	parts := make([]string, len(v))
	for i, e := range v {
		parts[i] = e.String()
	}
	return strings.Join(parts, "; ")
}

// Style is how Translate words its messages. The zero value is the wording
// of a Set.
type Style struct {
	// Bound prints the limit of minimum, maximum and the exclusive bounds
	// (default: eight decimals); Multiple prints multipleOf's divisor
	// (default: a fraction such as 1/100).
	Bound, Multiple func(*big.Rat) string
	// Pattern starts the message of a failed pattern (default "does not
	// match "); the pattern follows.
	Pattern string
	// ClosestBranch adds, after an anyOf or oneOf that no branch passes,
	// the failures of the branch with the fewest, each message starting
	// "(closest branch) ".
	ClosestBranch bool
	// Max caps the failures listed (0: no cap).
	Max int
	// Last lists keywords whose failures at a location come after every
	// other failure there and below it, in this order.
	Last []string
}

// rank is the place of a keyword failing at one location (see Translate).
func (st *Style) rank(kw string) int {
	for i, k := range st.Last {
		if k == kw {
			return 100 + i
		}
	}
	if r, ok := rank[kw]; ok {
		return r
	}
	return 26
}

// pre reports whether a keyword's failures come before those of the
// location's members or items.
func (st *Style) pre(kw string) bool {
	switch kw {
	case "false", "type", "enum", "const", "required", "minProperties", "maxProperties", "minItems", "maxItems", "uniqueItems":
		return st.rank(kw) < 100
	}
	return false
}

func (st *Style) bound(r *big.Rat) string {
	if st.Bound != nil {
		return st.Bound(r)
	}
	return r.FloatString(8)
}

func (st *Style) multiple(r *big.Rat) string {
	if st.Multiple != nil {
		return st.Multiple(r)
	}
	return r.RatString()
}

func (s *Set) translate(err error, instance any) error {
	if err == nil {
		return nil
	}
	ve := Translate(err, instance, &Style{}, s.schemaAt)
	if len(ve) == 0 {
		return err
	}
	return ve
}

// schemaAt is the schema object at a location the library reports
// ("mem:///a.json#/properties/x"), or nil.
func (s *Set) schemaAt(loc string) map[string]any {
	u, frag, _ := strings.Cut(loc, "#")
	s.mu.Lock()
	root, ok := s.byURL[u]
	s.mu.Unlock()
	if !ok {
		return nil
	}
	if p, err := url.PathUnescape(frag); err == nil {
		frag = p
	}
	node, err := lookupPointer(root, frag)
	if err != nil {
		return nil
	}
	m, _ := node.(map[string]any)
	return m
}

// Translate turns a validation failure of github.com/santhosh-tekuri/jsonschema/v6
// into a ValidationError: one Error per failed check, with our messages,
// listed depth first by instance location (members by name, items by
// index), and at one location in keyword order: type, enum, const, the
// object, array, string and number keywords, then dependencies, contains,
// anyOf, oneOf and not. Failures under allOf, $ref, if/then/else and
// propertyNames are listed where they occur; anyOf and oneOf report one
// failure each (with the closest branch if the style asks). schemaAt, when
// not nil, finds a schema object by the location the library reports, to
// name multiple types in the schema's order. Any other error is returned
// as nil.
func Translate(err error, instance any, st *Style, schemaAt func(loc string) map[string]any) ValidationError {
	var verr *v6.ValidationError
	if !errors.As(err, &verr) {
		return nil
	}
	if st == nil {
		st = &Style{}
	}
	t := &translator{st: st, inst: instance, schemaAt: schemaAt}
	t.walk(verr, nil)
	out := t.sorted()
	if st.Max > 0 && len(out) > st.Max {
		out = out[:st.Max]
	}
	return out
}

type flat struct {
	toks []string
	Error
	seq   int
	extra []flat // listed right after this one (closest branch, allOf)
	group bool   // only a holder of extra (allOf)
}

type translator struct {
	st       *Style
	inst     any
	schemaAt func(string) map[string]any
	out      []flat
}

func (t *translator) add(toks []string, kw, format string, a ...any) {
	p := pointer(toks)
	t.out = append(t.out, flat{toks: toks, Error: Error{Path: p, Keyword: kw, Message: fmt.Sprintf(format, a...)}, seq: len(t.out)})
}

// walk records the leaf failures of e. rebase, when set, replaces the
// instance location of e and its causes (propertyNames validates a name,
// which the library reports at the object).
func (t *translator) walk(e *v6.ValidationError, rebase []string) {
	toks := e.InstanceLocation
	if rebase != nil {
		toks = append(append([]string(nil), rebase...), toks...)
	}
	switch k := e.ErrorKind.(type) {
	case *kind.Schema, *kind.Group, *kind.Reference:
		for _, c := range e.Causes {
			t.walk(c, rebase)
		}
	case *kind.AllOf:
		// each branch's failures together, branch by branch, after the
		// location's own keywords and members
		g := flat{toks: toks, Error: Error{Path: pointer(toks), Keyword: "allOf"}, seq: len(t.out), group: true}
		for _, c := range e.Causes {
			sub := &translator{st: t.st, inst: t.inst, schemaAt: t.schemaAt}
			sub.walk(c, rebase)
			g.extra = append(g.extra, sub.sorted0()...)
		}
		t.out = append(t.out, g)
	case *kind.PropertyNames:
		at := append(append([]string(nil), toks...), k.Property)
		for _, c := range e.Causes {
			t.walk(c, at)
		}
	case *kind.Type:
		want := t.typeNames(e, k)
		got := typeName(valueAt(t.inst, toks))
		t.add(toks, "type", "must be %s, not %s", strings.Join(want, " or "), got)
	case *kind.Enum:
		t.add(toks, "enum", "%s is not one of %s", show(k.Got), show(k.Want))
	case *kind.Const:
		t.add(toks, "const", "must be %s", show(k.Want))
	case *kind.Required:
		for _, m := range k.Missing {
			t.add(toks, "required", "%s is required", m)
		}
	case *kind.MinProperties:
		t.add(toks, "minProperties", "needs at least %d members", k.Want)
	case *kind.MaxProperties:
		t.add(toks, "maxProperties", "allows at most %d members", k.Want)
	case *kind.AdditionalProperties:
		for _, p := range k.Properties {
			t.add(append(append([]string(nil), toks...), p), "additionalProperties", "%s is not an allowed member", p)
		}
	case *kind.Dependency:
		for _, m := range k.Missing {
			t.add(toks, "dependencies", "%s needs %s", k.Prop, m)
		}
	case *kind.DependentRequired:
		for _, m := range k.Missing {
			t.add(toks, "dependentRequired", "%s needs %s", k.Prop, m)
		}
	case *kind.MinItems:
		t.add(toks, "minItems", "needs at least %d items, has %d", k.Want, k.Got)
	case *kind.MaxItems:
		t.add(toks, "maxItems", "allows at most %d items, has %d", k.Want, k.Got)
	case *kind.UniqueItems:
		t.add(toks, "uniqueItems", "items %d and %d are equal", k.Duplicates[0], k.Duplicates[1])
	case *kind.AdditionalItems:
		n := 0
		if a, ok := valueAt(t.inst, toks).([]any); ok {
			n = len(a)
		}
		for i := n - k.Count; i < n; i++ {
			t.add(append(append([]string(nil), toks...), strconv.Itoa(i)), "false", "no value is allowed here")
		}
	case *kind.Contains:
		t.add(toks, "contains", "no item matches the contains schema")
	case *kind.MinLength:
		t.add(toks, "minLength", "needs at least %d characters", k.Want)
	case *kind.MaxLength:
		t.add(toks, "maxLength", "allows at most %d characters", k.Want)
	case *kind.Pattern:
		prefix := t.st.Pattern
		if prefix == "" {
			prefix = "does not match "
		}
		t.add(toks, "pattern", "%s%s", prefix, k.Want)
	case *kind.Format:
		msg := fmt.Sprintf("is not a valid %s", k.Want)
		if k.Err != nil {
			msg = k.Err.Error()
		}
		t.add(toks, "format", "%s", msg)
	case *kind.Minimum:
		t.add(toks, "minimum", "must be >= %s", t.st.bound(k.Want))
	case *kind.Maximum:
		t.add(toks, "maximum", "must be <= %s", t.st.bound(k.Want))
	case *kind.ExclusiveMinimum:
		t.add(toks, "exclusiveMinimum", "must be > %s", t.st.bound(k.Want))
	case *kind.ExclusiveMaximum:
		t.add(toks, "exclusiveMaximum", "must be < %s", t.st.bound(k.Want))
	case *kind.MultipleOf:
		t.add(toks, "multipleOf", "must be a multiple of %s", t.st.multiple(k.Want))
	case *kind.AnyOf:
		t.add(toks, "anyOf", "matches none of the anyOf schemas")
		t.closest(e)
	case *kind.OneOf:
		if len(k.Subschemas) == 0 {
			t.add(toks, "oneOf", "matches none of the oneOf schemas")
			t.closest(e)
		} else {
			t.add(toks, "oneOf", "matches %d of the oneOf schemas, not exactly one", len(k.Subschemas))
		}
	case *kind.Not:
		t.add(toks, "not", "must not match the not schema")
	case *kind.FalseSchema:
		t.add(toks, "false", "no value is allowed here")
	case *kind.RefCycle:
		t.add(toks, "$ref", "schema references nest too deeply")
	case *kind.ContentEncoding:
		t.add(toks, "contentEncoding", "is not %s (contentEncoding)", k.Want)
	case *kind.ContentMediaType:
		t.add(toks, "contentMediaType", "is not %s (contentMediaType)", k.Want)
	case *kind.ContentSchema:
		// the causes are located in the decoded content, not the instance
		t.add(toks, "contentSchema", "the decoded content does not match contentSchema")
	default:
		kw := ""
		if kp := e.ErrorKind.KeywordPath(); len(kp) > 0 {
			kw = kp[0]
		}
		t.add(toks, kw, "%s", e.ErrorKind.LocalizedString(printer))
		for _, c := range e.Causes {
			t.walk(c, rebase)
		}
	}
}

var printer = message.NewPrinter(language.English)

// closest records, for an anyOf or oneOf no branch passes, the failures of
// the branch with the fewest.
func (t *translator) closest(e *v6.ValidationError) {
	if !t.st.ClosestBranch {
		return
	}
	var best []flat
	for _, c := range e.Causes {
		sub := &translator{st: t.st, inst: t.inst, schemaAt: t.schemaAt}
		sub.walk(c, nil)
		if n := len(flatten(sub.out)); n > 0 && (best == nil || n < len(flatten(best))) {
			best = sub.sorted0()
		}
	}
	last := &t.out[len(t.out)-1]
	for _, f := range flatten(best) {
		f.Message = "(closest branch) " + f.Message
		f.extra = nil
		last.extra = append(last.extra, f)
	}
}

// flatten lists each failure followed by its extra ones.
func flatten(fs []flat) []flat {
	var out []flat
	for _, f := range fs {
		if !f.group {
			out = append(out, f)
		}
		out = append(out, flatten(f.extra)...)
	}
	return out
}

// typeNames are the types a failed "type" asks for, in the schema's order
// when the schema can be found, with null last otherwise.
func (t *translator) typeNames(e *v6.ValidationError, k *kind.Type) []string {
	if t.schemaAt != nil {
		if m := t.schemaAt(e.SchemaURL); m != nil {
			switch x := m["type"].(type) {
			case string:
				return []string{x}
			case []any:
				var out []string
				for _, y := range x {
					if s, ok := y.(string); ok {
						out = append(out, s)
					}
				}
				if len(out) > 0 {
					return out
				}
			}
		}
	}
	var out []string
	null := false
	for _, w := range k.Want {
		if w == "null" {
			null = true
			continue
		}
		out = append(out, w)
	}
	if null {
		out = append(out, "null")
	}
	return out
}

// rank orders the keywords failing at one location; pre keywords come
// before the failures of the location's members or items.
var rank = map[string]int{
	"false": 0, "type": 0, "enum": 1, "const": 2, "required": 3, "minProperties": 4, "maxProperties": 5,
	"additionalProperties": 6, "dependencies": 7, "dependentRequired": 7,
	"minItems": 8, "maxItems": 9, "uniqueItems": 10, "contains": 12,
	"minLength": 13, "maxLength": 14, "pattern": 15, "format": 16,
	"minimum": 17, "maximum": 18, "exclusiveMinimum": 19, "exclusiveMaximum": 20, "multipleOf": 21,
	"allOf": 22, "anyOf": 23, "oneOf": 24, "not": 25,
}

func (t *translator) sorted() ValidationError {
	fs := flatten(t.sorted0())
	out := make(ValidationError, len(fs))
	for i, f := range fs {
		out[i] = f.Error
	}
	return out
}

// sorted0 sorts the failures in place: depth first by location, members by
// name and items by index; at one location by keyword rank, then as found.
func (t *translator) sorted0() []flat {
	fs := t.out
	sort.SliceStable(fs, func(i, j int) bool { return t.less(fs[i], fs[j]) })
	return fs
}

func (t *translator) less(a, b flat) bool {
	n := len(a.toks)
	if len(b.toks) < n {
		n = len(b.toks)
	}
	for i := 0; i < n; i++ {
		if a.toks[i] == b.toks[i] {
			continue
		}
		if _, isArr := valueAt(t.inst, a.toks[:i]).([]any); isArr {
			x, e1 := strconv.Atoi(a.toks[i])
			y, e2 := strconv.Atoi(b.toks[i])
			if e1 == nil && e2 == nil {
				return x < y
			}
		}
		return a.toks[i] < b.toks[i]
	}
	if len(a.toks) != len(b.toks) {
		if len(a.toks) < len(b.toks) { // a is at an ancestor of b
			return t.st.pre(a.Keyword)
		}
		return !t.st.pre(b.Keyword)
	}
	ra, rb := t.st.rank(a.Keyword), t.st.rank(b.Keyword)
	if ra != rb {
		return ra < rb
	}
	if a.Keyword == "dependencies" && b.Keyword == "dependencies" && a.Message != b.Message {
		return a.Message < b.Message
	}
	return a.seq < b.seq
}

// pointer joins instance location tokens into a JSON pointer.
func pointer(toks []string) string {
	var b strings.Builder
	for _, t := range toks {
		b.WriteByte('/')
		b.WriteString(escape(t))
	}
	return b.String()
}

func escape(k string) string {
	return strings.ReplaceAll(strings.ReplaceAll(k, "~", "~0"), "/", "~1")
}

// valueAt is the value at the location in v, or nil.
func valueAt(v any, toks []string) any {
	for _, tok := range toks {
		switch c := v.(type) {
		case map[string]any:
			v = c[tok]
		case []any:
			n, err := strconv.Atoi(tok)
			if err != nil || n < 0 || n >= len(c) {
				return nil
			}
			v = c[n]
		default:
			return nil
		}
	}
	return v
}

func typeName(inst any) string {
	switch inst.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case string:
		return "string"
	case json.Number, float64, float32, int, int64, int32:
		return "number"
	}
	return fmt.Sprintf("%T", inst)
}

func show(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	if len(b) > 120 {
		return string(b[:117]) + "..."
	}
	return string(b)
}
