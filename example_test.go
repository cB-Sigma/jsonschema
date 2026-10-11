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
	set := jsonschema.NewSet()
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
