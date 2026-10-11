// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package jsonschema

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	v6 "github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
)

// OutputFormat is one of the four output structures of JSON Schema
// 2019-09 and 2020-12 (core specification, section 12.4 of 2020-12).
type OutputFormat int

const (
	// OutputFlag is the result alone: {"valid": false}.
	OutputFlag OutputFormat = iota + 1
	// OutputBasic is a flat list: the failures of an invalid instance, or
	// the annotations of a valid one.
	OutputBasic
	// OutputDetailed is a tree that follows the schema, with only the
	// failing (or, when valid, annotating) branches, and a unit with a
	// single child replaced by that child.
	OutputDetailed
	// OutputVerbose is the whole tree: every subschema applied to every
	// instance location, valid or not.
	OutputVerbose
)

// OutputUnit is one unit of the specification's output.
type OutputUnit struct {
	Valid bool
	// KeywordLocation is the JSON pointer of the keyword or subschema in
	// the schema, through any $ref, $dynamicRef or $recursiveRef followed.
	KeywordLocation string
	// AbsoluteKeywordLocation is the same place as an absolute URI: the
	// enclosing schema resource's $id (a document without one is
	// "mem:///" and its name in the set) and a pointer in it.
	AbsoluteKeywordLocation string
	// InstanceLocation is the JSON pointer of the value in the instance.
	InstanceLocation string
	// Error is our message for a failed keyword (empty on a unit whose
	// failure is its Errors).
	Error string
	// Annotation is the value a keyword annotates the instance with, as
	// JSON; nil when the unit carries none.
	Annotation json.RawMessage
	// Errors are the nested units that failed; Annotations the nested
	// units that passed.
	Errors      []OutputUnit
	Annotations []OutputUnit

	flag bool
}

// MarshalJSON writes the unit with the specification's property names;
// a flag result is {"valid": ...} alone.
func (u OutputUnit) MarshalJSON() ([]byte, error) {
	if u.flag {
		return json.Marshal(struct {
			Valid bool `json:"valid"`
		}{u.Valid})
	}
	return json.Marshal(struct {
		Valid                   bool            `json:"valid"`
		KeywordLocation         string          `json:"keywordLocation"`
		AbsoluteKeywordLocation string          `json:"absoluteKeywordLocation,omitempty"`
		InstanceLocation        string          `json:"instanceLocation"`
		Error                   string          `json:"error,omitempty"`
		Annotation              json.RawMessage `json:"annotation,omitempty"`
		Errors                  []OutputUnit    `json:"errors,omitempty"`
		Annotations             []OutputUnit    `json:"annotations,omitempty"`
	}{u.Valid, u.KeywordLocation, u.AbsoluteKeywordLocation, u.InstanceLocation, u.Error, u.Annotation, u.Errors, u.Annotations})
}

// Output validates an instance (as Validate does) and returns the result
// in one of the specification's output formats. The result's Valid is
// always the outcome of Validate. A schema that cannot be compiled is an
// error.
//
// How the tree is built: each subschema is evaluated against the part of
// the instance it applies to, failures come from the library's
// evaluation, and annotations are read from the schema documents. A
// subschema reached through $dynamicRef or $recursiveRef is shown at its
// static target; whether it passed comes from the evaluation in context.
func (s *Set) Output(name string, instance any, f OutputFormat) (OutputUnit, error) {
	sch, err := s.Compile(name)
	if err != nil {
		return OutputUnit{}, err
	}
	verr := sch.Validate(instance)
	if verr != nil {
		var ve *v6.ValidationError
		if !errors.As(verr, &ve) {
			return OutputUnit{}, verr
		}
	}
	valid := verr == nil
	if f == OutputFlag {
		return OutputUnit{Valid: valid, flag: true}, nil
	}
	if f != OutputBasic && f != OutputDetailed && f != OutputVerbose {
		return OutputUnit{}, fmt.Errorf("unknown output format %d", f)
	}
	w := &outWalker{s: s}
	var root OutputUnit
	var ve *v6.ValidationError
	if !valid {
		errors.As(verr, &ve)
	}
	if !valid && f != OutputVerbose {
		// failures exactly as the library found them, in context
		root = w.fromError(ve, instance, "", "")
	} else {
		root, _ = w.schema(sch, instance, nil, "", 0)
		if !valid && len(root.Errors) == 0 {
			// the walk evaluates each subschema alone; under $dynamicRef
			// the dynamic scope can change the result, so the library's
			// failures are shown
			root = w.fromError(ve, instance, "", "")
		}
		root.Valid = valid
	}
	switch f {
	case OutputBasic:
		out := OutputUnit{Valid: valid, KeywordLocation: "", AbsoluteKeywordLocation: root.AbsoluteKeywordLocation, InstanceLocation: ""}
		if valid {
			collectAnnotations(root, &out.Annotations)
		} else {
			collectErrors(root, &out.Errors)
		}
		return out, nil
	case OutputDetailed:
		d := detailed(root)
		d.Valid = valid
		return d, nil
	}
	return root, nil
}

// fromError turns the library's failure tree into units, as its own
// output does, with our messages. parentURL is the schema location of the
// parent failure ("" at the root).
func (w *outWalker) fromError(e *v6.ValidationError, inst any, kloc, parentURL string) OutputUnit {
	ref, isRef := e.ErrorKind.(*kind.Reference)
	if parentURL != "" && strings.HasPrefix(e.SchemaURL, parentURL) {
		rest := e.SchemaURL[len(parentURL):]
		if p, err := url.PathUnescape(rest); err == nil {
			rest = p
		}
		kloc += rest
		if isRef {
			kloc = kw(kloc, ref.KeywordPath()...)
		}
	}
	here := e.SchemaURL
	if isRef {
		here = ref.URL
	}
	u := OutputUnit{KeywordLocation: kloc, InstanceLocation: pointer(e.InstanceLocation)}
	if isRef {
		u.AbsoluteKeywordLocation = w.abs(ref.URL, nil)
	} else {
		kp := e.ErrorKind.KeywordPath()
		u.KeywordLocation = kw(kloc, kp...)
		u.AbsoluteKeywordLocation = w.abs(e.SchemaURL, kp)
	}
	for _, c := range e.Causes {
		cu := w.fromError(c, inst, kloc, here)
		if cr, ok := c.ErrorKind.(*kind.Reference); ok && cr != nil && len(c.Causes) == 1 && cu.Error == "" && len(cu.Errors) == 1 {
			cu = cu.Errors[0]
		}
		u.Errors = append(u.Errors, cu)
	}
	switch e.ErrorKind.(type) {
	case *kind.Schema, *kind.Group, *kind.Reference, *kind.AllOf, *kind.PropertyNames:
		if len(u.Errors) == 0 {
			u.Error = w.message(e, inst)
		}
	default:
		u.Error = w.message(e, inst)
	}
	return u
}

// collectAnnotations lists the annotating units under valid units.
func collectAnnotations(u OutputUnit, out *[]OutputUnit) {
	if !u.Valid {
		return
	}
	if u.Annotation != nil {
		leaf := u
		leaf.Errors, leaf.Annotations = nil, nil
		*out = append(*out, leaf)
	}
	for _, c := range u.Annotations {
		collectAnnotations(c, out)
	}
}

// collectErrors lists the failing units with a message under failing
// units.
func collectErrors(u OutputUnit, out *[]OutputUnit) {
	if u.Valid {
		return
	}
	if u.Error != "" {
		leaf := u
		leaf.Errors, leaf.Annotations = nil, nil
		*out = append(*out, leaf)
	}
	for _, c := range u.Errors {
		collectErrors(c, out)
	}
}

// detailed prunes the verbose tree: an invalid unit keeps its failing
// children, a valid one its annotating children; a unit with no message
// or annotation of its own and one child is replaced by the child.
func detailed(u OutputUnit) OutputUnit {
	if !u.Valid {
		var kids []OutputUnit
		for _, c := range u.Errors {
			kids = append(kids, detailed(c))
		}
		u.Errors, u.Annotations = kids, nil
		if u.Error == "" && len(kids) == 1 {
			return kids[0]
		}
		return u
	}
	var kids []OutputUnit
	for _, c := range u.Annotations {
		if hasAnnotation(c) {
			kids = append(kids, detailed(c))
		}
	}
	u.Errors, u.Annotations = nil, kids
	if u.Annotation == nil && len(kids) == 1 {
		return kids[0]
	}
	return u
}

func hasAnnotation(u OutputUnit) bool {
	if !u.Valid {
		return false
	}
	if u.Annotation != nil {
		return true
	}
	for _, c := range u.Annotations {
		if hasAnnotation(c) {
			return true
		}
	}
	return false
}

// maxOutputDepth bounds the walk through references that do not move in
// the instance.
const maxOutputDepth = 256

type outWalker struct {
	s *Set
}

// evaluated are the members and items a valid subschema evaluated, for
// unevaluatedProperties and unevaluatedItems.
type evaluated struct {
	props map[string]bool
	items map[int]bool
}

func (e *evaluated) add(o evaluated) {
	for k := range o.props {
		e.prop(k)
	}
	for k := range o.items {
		e.item(k)
	}
}

func (e *evaluated) prop(k string) {
	if e.props == nil {
		e.props = map[string]bool{}
	}
	e.props[k] = true
}

func (e *evaluated) item(i int) {
	if e.items == nil {
		e.items = map[int]bool{}
	}
	e.items[i] = true
}

func child(iloc []string, tok string) []string {
	return append(append([]string(nil), iloc...), tok)
}

func kw(kloc string, toks ...string) string {
	for _, t := range toks {
		kloc += "/" + escape(t)
	}
	return kloc
}

// add puts a nested unit under Errors or Annotations by its result.
func (u *OutputUnit) add(c OutputUnit) {
	if c.Valid {
		u.Annotations = append(u.Annotations, c)
	} else {
		u.Errors = append(u.Errors, c)
	}
}

// schema is the unit of one subschema applied to inst at iloc.
func (w *outWalker) schema(sch *v6.Schema, inst any, iloc []string, kloc string, depth int) (OutputUnit, evaluated) {
	var ev evaluated
	u := OutputUnit{KeywordLocation: kloc, AbsoluteKeywordLocation: w.abs(sch.Location, nil), InstanceLocation: pointer(iloc)}
	if sch.Bool != nil {
		u.Valid = *sch.Bool
		if !u.Valid {
			u.Error = "no value is allowed here"
		}
		return u, ev
	}
	err := sch.Validate(inst)
	u.Valid = err == nil
	var causes []*v6.ValidationError
	var ve *v6.ValidationError
	if errors.As(err, &ve) {
		causes = topCauses(ve)
	}
	if depth > maxOutputDepth {
		if !u.Valid {
			u.Error = "schema references nest too deeply"
		}
		return u, ev
	}
	failed := map[string]bool{} // applicator keywords the library reports as failed
	for _, c := range causes {
		switch c.ErrorKind.(type) {
		case *kind.Schema, *kind.Group, *kind.Reference, *kind.AllOf, *kind.PropertyNames:
			// shown through the walk below
		case *kind.AnyOf:
			failed["anyOf"] = true
		case *kind.OneOf:
			failed["oneOf"] = true
		case *kind.Not:
			failed["not"] = true
		case *kind.Contains:
			failed["contains"] = true
		default:
			kp := c.ErrorKind.KeywordPath()
			u.Errors = append(u.Errors, OutputUnit{
				KeywordLocation:         kw(kloc, kp...),
				AbsoluteKeywordLocation: w.abs(c.SchemaURL, kp),
				InstanceLocation:        pointer(append(append([]string(nil), iloc...), c.InstanceLocation...)),
				Error:                   w.message(c, inst),
			})
		}
	}
	raw := w.s.schemaAt(sch.Location)

	// in place applicators
	inPlace := func(c OutputUnit, cev evaluated) {
		u.add(c)
		if c.Valid {
			ev.add(cev)
		}
	}
	if sch.Ref != nil {
		inPlace(w.schema(sch.Ref, inst, iloc, kw(kloc, "$ref"), depth+1))
	}
	if sch.RecursiveRef != nil {
		inPlace(w.schema(sch.RecursiveRef, inst, iloc, kw(kloc, "$recursiveRef"), depth+1))
	}
	if sch.DynamicRef != nil && sch.DynamicRef.Ref != nil {
		inPlace(w.schema(sch.DynamicRef.Ref, inst, iloc, kw(kloc, "$dynamicRef"), depth+1))
	}
	for i, sub := range sch.AllOf {
		inPlace(w.schema(sub, inst, iloc, kw(kloc, "allOf", strconv.Itoa(i)), depth+1))
	}
	for _, k := range []struct {
		name string
		subs []*v6.Schema
	}{{"anyOf", sch.AnyOf}, {"oneOf", sch.OneOf}} {
		if len(k.subs) == 0 {
			continue
		}
		g := OutputUnit{Valid: !failed[k.name], KeywordLocation: kw(kloc, k.name), AbsoluteKeywordLocation: w.abs(sch.Location, []string{k.name}), InstanceLocation: pointer(iloc)}
		for i, sub := range k.subs {
			c, cev := w.schema(sub, inst, iloc, kw(kloc, k.name, strconv.Itoa(i)), depth+1)
			g.add(c)
			if c.Valid && g.Valid {
				ev.add(cev)
			}
		}
		if !g.Valid {
			g.Error = w.keywordMessage(causes, k.name, inst)
		}
		u.add(g)
	}
	if sch.Not != nil {
		g := OutputUnit{Valid: !failed["not"], KeywordLocation: kw(kloc, "not"), AbsoluteKeywordLocation: w.abs(sch.Location, []string{"not"}), InstanceLocation: pointer(iloc)}
		c, _ := w.schema(sch.Not, inst, iloc, kw(kloc, "not"), depth+1)
		g.add(c)
		if !g.Valid {
			g.Error = "must not match the not schema"
		}
		u.add(g)
	}
	if sch.If != nil {
		c, cev := w.schema(sch.If, inst, iloc, kw(kloc, "if"), depth+1)
		// a failed "if" is not a failure of the schema
		g := OutputUnit{Valid: true, KeywordLocation: kw(kloc, "if"), AbsoluteKeywordLocation: c.AbsoluteKeywordLocation, InstanceLocation: pointer(iloc)}
		g.add(c)
		u.add(g)
		if c.Valid {
			ev.add(cev)
			if sch.Then != nil {
				inPlace(w.schema(sch.Then, inst, iloc, kw(kloc, "then"), depth+1))
			}
		} else if sch.Else != nil {
			inPlace(w.schema(sch.Else, inst, iloc, kw(kloc, "else"), depth+1))
		}
	}

	switch v := inst.(type) {
	case map[string]any:
		w.object(sch, v, iloc, kloc, depth, &u, &ev, raw)
	case []any:
		w.array(sch, v, iloc, kloc, depth, &u, &ev, failed)
	}

	if u.Valid {
		w.annotate(&u, sch, raw, iloc, kloc)
	}
	return u, ev
}

func (w *outWalker) object(sch *v6.Schema, obj map[string]any, iloc []string, kloc string, depth int, u *OutputUnit, ev *evaluated, raw map[string]any) {
	names := sortedKeys(obj)
	own := evaluated{}
	if len(sch.Properties) > 0 {
		var matched []string
		for _, n := range names {
			if sub, ok := sch.Properties[n]; ok {
				c, _ := w.schema(sub, obj[n], child(iloc, n), kw(kloc, "properties", n), depth+1)
				u.add(c)
				own.prop(n)
				matched = append(matched, n)
			}
		}
		u.annotation(kw(kloc, "properties"), w.abs(sch.Location, []string{"properties"}), iloc, matched)
	}
	if len(sch.PatternProperties) > 0 {
		var matched []string
		res := make([]v6.Regexp, 0, len(sch.PatternProperties))
		for re := range sch.PatternProperties {
			res = append(res, re)
		}
		sort.Slice(res, func(i, j int) bool { return res[i].String() < res[j].String() })
		for _, n := range names {
			hit := false
			for _, re := range res {
				if re.MatchString(n) {
					c, _ := w.schema(sch.PatternProperties[re], obj[n], child(iloc, n), kw(kloc, "patternProperties", re.String()), depth+1)
					u.add(c)
					hit = true
				}
			}
			if hit {
				own.prop(n)
				matched = append(matched, n)
			}
		}
		u.annotation(kw(kloc, "patternProperties"), w.abs(sch.Location, []string{"patternProperties"}), iloc, matched)
	}
	if ap, ok := sch.AdditionalProperties.(*v6.Schema); ok {
		var matched []string
		for _, n := range names {
			if sch.Properties[n] != nil || matchesAny(sch.PatternProperties, n) {
				continue
			}
			c, _ := w.schema(ap, obj[n], child(iloc, n), kw(kloc, "additionalProperties"), depth+1)
			u.add(c)
			own.prop(n)
			matched = append(matched, n)
		}
		u.annotation(kw(kloc, "additionalProperties"), w.abs(sch.Location, []string{"additionalProperties"}), iloc, matched)
	} else if ap == nil && sch.AdditionalProperties == true {
		for _, n := range names {
			own.prop(n)
		}
	}
	if sch.PropertyNames != nil {
		for _, n := range names {
			c, _ := w.schema(sch.PropertyNames, n, child(iloc, n), kw(kloc, "propertyNames"), depth+1)
			u.add(c)
		}
	}
	for _, n := range names {
		if sub := sch.DependentSchemas[n]; sub != nil {
			c, cev := w.schema(sub, obj, iloc, kw(kloc, "dependentSchemas", n), depth+1)
			u.add(c)
			if c.Valid {
				ev.add(cev)
			}
		}
		if sub, ok := sch.Dependencies[n].(*v6.Schema); ok {
			c, cev := w.schema(sub, obj, iloc, kw(kloc, "dependencies", n), depth+1)
			u.add(c)
			if c.Valid {
				ev.add(cev)
			}
		}
	}
	ev.add(own)
	if sch.UnevaluatedProperties != nil {
		var matched []string
		for _, n := range names {
			if ev.props[n] {
				continue
			}
			c, _ := w.schema(sch.UnevaluatedProperties, obj[n], child(iloc, n), kw(kloc, "unevaluatedProperties"), depth+1)
			u.add(c)
			matched = append(matched, n)
		}
		for _, n := range matched {
			ev.prop(n)
		}
		u.annotation(kw(kloc, "unevaluatedProperties"), w.abs(sch.Location, []string{"unevaluatedProperties"}), iloc, matched)
	}
}

func matchesAny(pp map[v6.Regexp]*v6.Schema, n string) bool {
	for re := range pp {
		if re.MatchString(n) {
			return true
		}
	}
	return false
}

func (w *outWalker) array(sch *v6.Schema, arr []any, iloc []string, kloc string, depth int, u *OutputUnit, ev *evaluated, failed map[string]bool) {
	n := len(arr)
	next := 0 // first item not taken by a positional keyword
	switch items := sch.Items.(type) {
	case *v6.Schema: // drafts before 2020-12: one schema for every item
		for i := 0; i < n; i++ {
			c, _ := w.schema(items, arr[i], child(iloc, strconv.Itoa(i)), kw(kloc, "items"), depth+1)
			u.add(c)
			ev.item(i)
		}
		next = n
		if n > 0 {
			u.annotation(kw(kloc, "items"), w.abs(sch.Location, []string{"items"}), iloc, true)
		}
	case []*v6.Schema:
		for i := 0; i < n && i < len(items); i++ {
			c, _ := w.schema(items[i], arr[i], child(iloc, strconv.Itoa(i)), kw(kloc, "items", strconv.Itoa(i)), depth+1)
			u.add(c)
			ev.item(i)
		}
		next = min(n, len(items))
		if ai, ok := sch.AdditionalItems.(*v6.Schema); ok {
			for i := next; i < n; i++ {
				c, _ := w.schema(ai, arr[i], child(iloc, strconv.Itoa(i)), kw(kloc, "additionalItems"), depth+1)
				u.add(c)
				ev.item(i)
			}
			next = n
		}
	}
	for i := 0; i < n && i < len(sch.PrefixItems); i++ {
		c, _ := w.schema(sch.PrefixItems[i], arr[i], child(iloc, strconv.Itoa(i)), kw(kloc, "prefixItems", strconv.Itoa(i)), depth+1)
		u.add(c)
		ev.item(i)
		next = i + 1
	}
	if len(sch.PrefixItems) > 0 && n > 0 {
		var a any = next - 1
		if next >= n {
			a = true
		}
		u.annotation(kw(kloc, "prefixItems"), w.abs(sch.Location, []string{"prefixItems"}), iloc, a)
	}
	if sch.Items2020 != nil {
		for i := len(sch.PrefixItems); i < n; i++ {
			c, _ := w.schema(sch.Items2020, arr[i], child(iloc, strconv.Itoa(i)), kw(kloc, "items"), depth+1)
			u.add(c)
			ev.item(i)
		}
		if n > len(sch.PrefixItems) {
			u.annotation(kw(kloc, "items"), w.abs(sch.Location, []string{"items"}), iloc, true)
		}
	}
	if sch.Contains != nil {
		g := OutputUnit{Valid: !failed["contains"], KeywordLocation: kw(kloc, "contains"), AbsoluteKeywordLocation: w.abs(sch.Location, []string{"contains"}), InstanceLocation: pointer(iloc)}
		var hits []int
		for i := 0; i < n; i++ {
			c, _ := w.schema(sch.Contains, arr[i], child(iloc, strconv.Itoa(i)), kw(kloc, "contains"), depth+1)
			g.add(c)
			if c.Valid {
				hits = append(hits, i)
				if sch.DraftVersion >= 2020 {
					ev.item(i)
				}
			}
		}
		if !g.Valid {
			g.Error = "no item matches the contains schema"
			if len(hits) > 0 {
				g.Error = fmt.Sprintf("%d items match the contains schema, outside minContains and maxContains", len(hits))
			}
		} else if len(hits) > 0 {
			b, _ := json.Marshal(hits)
			g.Annotation = b
		}
		u.add(g)
	}
	if sch.UnevaluatedItems != nil {
		took := false
		for i := 0; i < n; i++ {
			if ev.items[i] {
				continue
			}
			c, _ := w.schema(sch.UnevaluatedItems, arr[i], child(iloc, strconv.Itoa(i)), kw(kloc, "unevaluatedItems"), depth+1)
			u.add(c)
			took = true
		}
		for i := 0; i < n; i++ {
			ev.item(i)
		}
		if took {
			u.annotation(kw(kloc, "unevaluatedItems"), w.abs(sch.Location, []string{"unevaluatedItems"}), iloc, true)
		}
	}
}

// annotation adds a passing unit carrying a value; an empty list of names
// adds nothing.
func (u *OutputUnit) annotation(kloc, abs string, iloc []string, v any) {
	if names, ok := v.([]string); ok && len(names) == 0 {
		return
	}
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	u.Annotations = append(u.Annotations, OutputUnit{Valid: true, KeywordLocation: kloc, AbsoluteKeywordLocation: abs, InstanceLocation: pointer(iloc), Annotation: b})
}

// annotationKeywords are the keywords whose value annotates the instance
// as written in the schema.
var annotationKeywords = []string{"title", "description", "default", "examples", "deprecated", "readOnly", "writeOnly",
	"format", "contentEncoding", "contentMediaType", "contentSchema"}

// annotate adds the schema's own annotations to a valid unit. Annotations
// from keywords of applicators (properties and so on) are added by the
// walk; they are failed units' children only when the unit fails.
func (w *outWalker) annotate(u *OutputUnit, sch *v6.Schema, raw map[string]any, iloc []string, kloc string) {
	if raw == nil {
		return
	}
	var own []OutputUnit
	for _, k := range annotationKeywords {
		v, ok := raw[k]
		if !ok {
			continue
		}
		if k == "contentSchema" && raw["contentMediaType"] == nil {
			continue
		}
		b, err := json.Marshal(v)
		if err != nil {
			continue
		}
		own = append(own, OutputUnit{Valid: true, KeywordLocation: kw(kloc, k), AbsoluteKeywordLocation: w.abs(sch.Location, []string{k}), InstanceLocation: pointer(iloc), Annotation: b})
	}
	u.Annotations = append(own, u.Annotations...)
}

// topCauses are the failures of the schema object itself: the library's
// result unwrapped of its schema and group wrappers.
func topCauses(e *v6.ValidationError) []*v6.ValidationError {
	switch e.ErrorKind.(type) {
	case *kind.Schema, *kind.Group:
		var out []*v6.ValidationError
		for _, c := range e.Causes {
			if c.SchemaURL == e.SchemaURL || isGroup(c) {
				out = append(out, topCauses(c)...)
			} else {
				out = append(out, c)
			}
		}
		return out
	}
	return []*v6.ValidationError{e}
}

func isGroup(e *v6.ValidationError) bool {
	_, ok := e.ErrorKind.(*kind.Group)
	return ok
}

// message is our wording of one failure of the library, without its
// causes.
func (w *outWalker) message(c *v6.ValidationError, inst any) string {
	leaf := *c
	leaf.Causes = nil
	t := &translator{st: &Style{}, inst: inst, schemaAt: w.s.schemaAt}
	t.walk(&leaf, nil)
	var msgs []string
	for _, f := range flatten(t.out) {
		msgs = append(msgs, f.Message)
	}
	if len(msgs) == 0 {
		return c.ErrorKind.LocalizedString(printer)
	}
	return strings.Join(msgs, "; ")
}

func (w *outWalker) keywordMessage(causes []*v6.ValidationError, name string, inst any) string {
	for _, c := range causes {
		if kp := c.ErrorKind.KeywordPath(); len(kp) > 0 && kp[0] == name {
			return w.message(c, inst)
		}
	}
	return "matches none of the " + name + " schemas"
}

// abs turns a location the library reports ("mem:///a.json#/properties/x")
// and keyword tokens into an absolute keyword location: the URI of the
// innermost schema resource ($id) holding the place, and the pointer from
// that resource.
func (w *outWalker) abs(loc string, toks []string) string {
	u, frag, _ := strings.Cut(loc, "#")
	ptr, err := url.PathUnescape(frag)
	if err != nil {
		ptr = frag
	}
	w.s.mu.Lock()
	root, ok := w.s.byURL[u]
	w.s.mu.Unlock()
	base := u
	rel := ptr
	if ok {
		if b, r, ok := resourceOf(u, root, ptr); ok {
			base, rel = b, r
		}
	}
	var sb strings.Builder
	sb.WriteString(base)
	sb.WriteByte('#')
	sb.WriteString(rel)
	for _, t := range toks {
		sb.WriteByte('/')
		sb.WriteString(escape(t))
	}
	return sb.String()
}

// resourceOf walks a document along a pointer and returns the URI of the
// innermost $id on the way (resolved against the ones above it) and the
// rest of the pointer.
func resourceOf(docURL string, root any, ptr string) (string, string, bool) {
	base, err := url.Parse(docURL)
	if err != nil {
		return "", "", false
	}
	setBase := func(node any) {
		if m, ok := node.(map[string]any); ok {
			if id, ok := m["$id"].(string); ok && id != "" && !strings.HasPrefix(id, "#") {
				if r, err := url.Parse(id); err == nil {
					base = base.ResolveReference(r)
					base.Fragment = ""
				}
			}
		}
	}
	node := root
	setBase(node)
	rest := ""
	if ptr == "" {
		return base.String(), "", true
	}
	toks := strings.Split(strings.TrimPrefix(ptr, "/"), "/")
	for _, raw := range toks {
		tok := strings.ReplaceAll(strings.ReplaceAll(raw, "~1", "/"), "~0", "~")
		switch c := node.(type) {
		case map[string]any:
			node = c[tok]
		case []any:
			i, err := strconv.Atoi(tok)
			if err != nil || i < 0 || i >= len(c) {
				return "", "", false
			}
			node = c[i]
		default:
			return "", "", false
		}
		rest += "/" + raw
		if m, ok := node.(map[string]any); ok {
			if id, ok := m["$id"].(string); ok && id != "" && !strings.HasPrefix(id, "#") {
				setBase(node)
				rest = ""
			}
		}
	}
	return base.String(), rest, true
}

// String is the unit as indented JSON, for logs and tests.
func (u OutputUnit) String() string {
	b, err := json.Marshal(u)
	if err != nil {
		return err.Error()
	}
	var out bytes.Buffer
	if json.Indent(&out, b, "", "  ") != nil {
		return string(b)
	}
	return out.String()
}
