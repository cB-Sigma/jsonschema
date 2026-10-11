// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package jsonschema

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// CheckSchemas resolves references as the library does: against the base
// URI in force where the reference is ($id of the document and of any
// enclosing subschema), to a schema resource (a document or a subschema
// with $id), then by a JSON pointer from that resource or by an anchor
// ($anchor, $dynamicAnchor, or a draft-06/07 "$id": "#name"). When no
// resource has the URL, the set's own fallbacks apply: a document added
// under the whole URL, then one with the same file name.

type place struct {
	d   *doc
	ptr string
}

type refIndex struct {
	res     map[string]place // absolute URL without fragment
	anchors map[string]place // absolute URL "#" anchor
}

// refs builds the index of the set's resources and anchors.
func (s *Set) refs() *refIndex {
	ix := &refIndex{res: map[string]place{}, anchors: map[string]place{}}
	for _, name := range s.Names() {
		d := s.byName[name]
		ix.walk(d, d.root, baseOf(d), "")
	}
	return ix
}

// baseOf is a document's base URI; a relative one (a file name) is made
// absolute under mem:///, so that references between such documents
// resolve as between files of one directory.
func baseOf(d *doc) *url.URL {
	b := memRoot.ResolveReference(d.base)
	b.Fragment = ""
	return b
}

var memRoot = &url.URL{Scheme: "mem", Path: "/"}

func noFragment(u *url.URL) string {
	k := *u
	k.Fragment = ""
	return k.String()
}

func (ix *refIndex) walk(d *doc, node any, base *url.URL, ptr string) {
	switch x := node.(type) {
	case map[string]any:
		if ptr == "" {
			ix.res[noFragment(base)] = place{d, ""}
		}
		if id, ok := x[d.idKey].(string); ok && id != "" {
			if r, err := url.Parse(id); err == nil {
				if strings.HasPrefix(id, "#") { // draft-06/07 plain-name $id
					ix.anchors[noFragment(base)+"#"+r.Fragment] = place{d, ptr}
				} else {
					base = base.ResolveReference(r)
					ix.res[noFragment(base)] = place{d, ptr}
					if base.Fragment != "" && !strings.HasPrefix(base.Fragment, "/") {
						ix.anchors[noFragment(base)+"#"+base.Fragment] = place{d, ptr}
					}
					base = &url.URL{Scheme: base.Scheme, Opaque: base.Opaque, User: base.User, Host: base.Host, Path: base.Path, RawPath: base.RawPath, RawQuery: base.RawQuery}
				}
			}
		}
		for _, k := range []string{"$anchor", "$dynamicAnchor"} {
			if a, ok := x[k].(string); ok && a != "" {
				key := noFragment(base) + "#" + a
				if _, dup := ix.anchors[key]; !dup || k == "$anchor" {
					ix.anchors[key] = place{d, ptr}
				}
			}
		}
		for k, v := range x {
			switch k {
			case "enum", "const", "default", "examples":
				continue
			}
			ix.walk(d, v, base, ptr+"/"+escape(k))
		}
	case []any:
		for i, v := range x {
			ix.walk(d, v, base, ptr+"/"+strconv.Itoa(i))
		}
	}
}

// baseAt is the base URI in force at a pointer of a document.
func baseAt(d *doc, ptr string) *url.URL {
	base := baseOf(d)
	node := d.root
	apply := func(n any) {
		if m, ok := n.(map[string]any); ok {
			if id, ok := m[d.idKey].(string); ok && id != "" && !strings.HasPrefix(id, "#") {
				if r, err := url.Parse(id); err == nil {
					base = base.ResolveReference(r)
					base.Fragment = ""
				}
			}
		}
	}
	apply(node)
	if ptr == "" {
		return base
	}
	for _, tok := range strings.Split(strings.TrimPrefix(ptr, "/"), "/") {
		tok = strings.ReplaceAll(strings.ReplaceAll(tok, "~1", "/"), "~0", "~")
		switch n := node.(type) {
		case map[string]any:
			node = n[tok]
		case []any:
			i, err := strconv.Atoi(tok)
			if err != nil || i < 0 || i >= len(n) {
				return base
			}
			node = n[i]
		default:
			return base
		}
		apply(node)
	}
	return base
}

// resolveAt finds the target of a reference made at pointer from of
// document d, and returns its document, value and pointer there.
func (s *Set) resolveAt(ix *refIndex, d *doc, from, ref string) (*doc, any, string, error) {
	u, err := url.Parse(ref)
	if err != nil {
		return nil, nil, "", fmt.Errorf("$ref %q: %w", ref, err)
	}
	abs := baseAt(d, from).ResolveReference(u)
	key := noFragment(abs)
	frag := u.Fragment
	var at place
	if p, ok := ix.res[key]; ok {
		at = p
	} else if u.Scheme == "" && u.Host == "" && u.Path == "" {
		at = place{d, ""} // a fragment of a document whose base the index lacks
	} else {
		td, _, err := s.resolve(d, noFragment(u))
		if err != nil {
			return nil, nil, "", err
		}
		at = place{td, ""}
		key = noFragment(baseOf(td))
	}
	if frag != "" && !strings.HasPrefix(frag, "/") {
		p, ok := ix.anchors[key+"#"+frag]
		if !ok {
			return nil, nil, "", fmt.Errorf("$ref %q: no anchor %q", ref, frag)
		}
		node, err := lookupPointer(p.d.root, p.ptr)
		if err != nil {
			return nil, nil, "", fmt.Errorf("$ref %q: %w", ref, err)
		}
		return p.d, node, p.ptr, nil
	}
	ptr := at.ptr + frag
	node, err := lookupPointer(at.d.root, ptr)
	if err != nil {
		return nil, nil, "", fmt.Errorf("$ref %q: %w", ref, err)
	}
	return at.d, node, ptr, nil
}
