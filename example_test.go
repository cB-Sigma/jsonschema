// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package jsonschema_test

import (
	"errors"
	"fmt"

	"github.com/cB-Sigma/jsonschema"
)

// A family of two schema documents that refer to each other by file name;
// every failure comes back as pointer, keyword and message, in order.
func Example() {
	set := jsonschema.NewSet(jsonschema.Options{})
	_ = set.Add("meter.json", []byte(`{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"type": "object",
		"properties": {
			"id": {"type": "string", "minLength": 1},
			"reading": {"$ref": "reading.json"}
		},
		"required": ["id", "reading"]
	}`))
	_ = set.Add("reading.json", []byte(`{
		"type": "object",
		"properties": {"kWh": {"type": "number", "minimum": 0}, "at": {"type": "string", "format": "date-time"}},
		"required": ["kWh"]
	}`))

	fmt.Println(set.ValidateJSON("meter.json", []byte(`{"id": "m1", "reading": {"kWh": 12.5}}`)))

	err := set.ValidateJSON("meter.json", []byte(`{"id": "", "reading": {"kWh": -1, "at": "yesterday"}}`))
	var v jsonschema.ValidationError
	if errors.As(err, &v) {
		for _, e := range v {
			fmt.Println(e.Path, e.Keyword)
		}
	}
	// Output:
	// <nil>
	// /id minLength
	// /reading/at format
	// /reading/kWh minimum
}

// In a 2020-12 document format is an annotation unless the set asserts
// it; patterns are ECMA-262.
func ExampleNewSet_options() {
	schema := []byte(`{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"type": "string",
		"format": "hostname",
		"pattern": "^(?!-)[a-z0-9.-]+$"
	}`)
	for _, assert := range []bool{false, true} {
		set := jsonschema.NewSet(jsonschema.Options{AssertFormat: assert})
		_ = set.Add("host.json", schema)
		fmt.Println(assert, set.ValidateJSON("host.json", []byte(`"gw..example"`)))
	}
	// Output:
	// false <nil>
	// true /: gw..example is not a host name
}

// The specification's basic output format: each failure with its keyword
// location, absolute keyword location and instance location.
func ExampleSet_Output() {
	set := jsonschema.NewSet(jsonschema.Options{})
	_ = set.Add("evse.json", []byte(`{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"$id": "https://example.com/evse",
		"properties": {"evseId": {"type": "integer", "minimum": 0}}
	}`))
	inst, _ := jsonschema.Decode([]byte(`{"evseId": -1}`))
	out, _ := set.Output("evse.json", inst, jsonschema.OutputBasic)
	fmt.Println(out.Valid)
	for _, e := range out.Errors {
		fmt.Println(e.KeywordLocation, e.AbsoluteKeywordLocation, e.InstanceLocation, e.Error)
	}
	// Output:
	// false
	// /properties/evseId/minimum https://example.com/evse#/properties/evseId/minimum /evseId must be >= 0.00000000
}
