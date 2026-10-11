// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package jsonschema

import (
	"encoding/json"
	"math/big"
	"strings"

	v6 "github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
)

// Options.Draft4Integer: draft-04 defines an integer as a JSON number
// written without a fraction or an exponent, so 1.0 is a number but not an
// integer there. The library (like every later draft) reads integer as
// "a number whose value is whole". With the option, the loader marks each
// schema object of a draft-04 document whose "type" allows integer but not
// number, and a vocabulary refuses at those objects a json.Number whose
// text has a fraction or an exponent. The documents as added are not
// changed.

// draft4IntKey marks a schema object for the check. It is not a JSON
// Schema keyword; draft-04 ignores keywords it does not know.
const draft4IntKey = "x-cb-sigma-draft4-integer"

const draft4Meta = "http://json-schema.org/draft-04/schema"

func isDraft4(root any) bool {
	m, ok := root.(map[string]any)
	if !ok {
		return false
	}
	s, _ := m["$schema"].(string)
	return strings.TrimSuffix(s, "#") == draft4Meta
}

// markDraft4Integers returns a copy of a draft-04 document with the marker
// on each schema object whose type allows integer but not number. Values
// that are data, not schemas (enum, default, examples, const), are copied
// unchanged.
func markDraft4Integers(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x)+1)
		for k, val := range x {
			switch k {
			case "enum", "default", "examples", "const", "required":
				out[k] = val
			default:
				out[k] = markDraft4Integers(val)
			}
		}
		if typeIsIntegerOnly(x["type"]) {
			out[draft4IntKey] = true
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = markDraft4Integers(val)
		}
		return out
	}
	return v
}

func typeIsIntegerOnly(t any) bool {
	switch x := t.(type) {
	case string:
		return x == "integer"
	case []any:
		integer := false
		for _, y := range x {
			switch y {
			case "number":
				return false
			case "integer":
				integer = true
			}
		}
		return integer
	}
	return false
}

// draft4IntVocab is the vocabulary that applies the marker.
var draft4IntVocab = &v6.Vocabulary{
	URL: "urn:cb-sigma:jsonschema:draft4-integer",
	Compile: func(_ *v6.CompilerContext, obj map[string]any) (v6.SchemaExt, error) {
		if obj[draft4IntKey] != true {
			return nil, nil
		}
		return draft4Int{}, nil
	},
}

type draft4Int struct{}

func (draft4Int) Validate(ctx *v6.ValidatorContext, v any) {
	n, ok := v.(json.Number)
	if !ok || !strings.ContainsAny(string(n), ".eE") {
		return
	}
	// a number with a non-zero fraction is refused by "type" already
	if r, ok := new(big.Rat).SetString(string(n)); !ok || !r.IsInt() {
		return
	}
	ctx.AddError(&kind.Type{Got: "number", Want: []string{"integer"}})
}
