// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

// Package bench compares github.com/cB-Sigma/jsonschema with
// github.com/santhosh-tekuri/jsonschema/v6 used directly and with
// github.com/kaptinlin/jsonschema, on one schema and two instances (one
// valid, one with four failures). It is a separate module so that the
// library's module does not depend on the validators it is compared with.
//
//	go test -bench . -benchmem
package bench

import (
	"bytes"
	"encoding/json"
	"testing"

	sigma "github.com/cB-Sigma/jsonschema"
	kaptin "github.com/kaptinlin/jsonschema"
	santhosh "github.com/santhosh-tekuri/jsonschema/v6"
)

// schema is shaped like an OCPP 2.0.1 MeterValues request: nested
// objects, arrays, enums, a $ref, numeric bounds, a pattern and a format.
const schema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://bench.example/meter-values",
  "type": "object",
  "required": ["evseId", "meterValue"],
  "additionalProperties": false,
  "properties": {
    "evseId": {"type": "integer", "minimum": 0},
    "meterValue": {"type": "array", "minItems": 1, "items": {"$ref": "#/$defs/MeterValue"}}
  },
  "$defs": {
    "MeterValue": {
      "type": "object",
      "required": ["timestamp", "sampledValue"],
      "additionalProperties": false,
      "properties": {
        "timestamp": {"type": "string", "format": "date-time"},
        "sampledValue": {"type": "array", "minItems": 1, "items": {"$ref": "#/$defs/SampledValue"}}
      }
    },
    "SampledValue": {
      "type": "object",
      "required": ["value"],
      "additionalProperties": false,
      "properties": {
        "value": {"type": "number"},
        "context": {"enum": ["Interruption.Begin", "Interruption.End", "Other", "Sample.Clock", "Sample.Periodic", "Transaction.Begin", "Transaction.End", "Trigger"]},
        "measurand": {"type": "string", "maxLength": 50},
        "phase": {"enum": ["L1", "L2", "L3", "N", "L1-N", "L2-N", "L3-N", "L1-L2", "L2-L3", "L3-L1"]},
        "unit": {"type": "string", "pattern": "^[A-Za-z]{1,8}$"}
      }
    }
  }
}`

var valid = []byte(`{"evseId":1,"meterValue":[
 {"timestamp":"2026-10-11T10:00:00Z","sampledValue":[{"value":12.5,"context":"Sample.Periodic","measurand":"Energy.Active.Import.Register","phase":"L1","unit":"kWh"},{"value":230.1,"measurand":"Voltage","phase":"L1-N","unit":"V"}]},
 {"timestamp":"2026-10-11T10:05:00Z","sampledValue":[{"value":12.9,"context":"Sample.Periodic","unit":"kWh"},{"value":16,"measurand":"Current.Import","phase":"L2","unit":"A"}]}]}`)

var invalid = []byte(`{"evseId":-1,"meterValue":[
 {"timestamp":"2026-10-11T10:00:00Z","sampledValue":[{"value":"12.5","context":"Sample.Periodic","phase":"L4","unit":"kWh"}]},
 {"timestamp":"2026-10-11T10:05:00Z","sampledValue":[{"value":12.9,"extra":true}]}]}`)

func decode(b []byte) any {
	v, err := sigma.Decode(b)
	if err != nil {
		panic(err)
	}
	return v
}

func sigmaSchema(b testing.TB) *sigma.Set {
	s := sigma.NewSet(sigma.Options{})
	if err := s.Add("meter-values.json", []byte(schema)); err != nil {
		b.Fatal(err)
	}
	if err := s.Compile("meter-values.json"); err != nil {
		b.Fatal(err)
	}
	return s
}

func santhoshSchema(b testing.TB) *santhosh.Schema {
	doc, err := santhosh.UnmarshalJSON(bytes.NewReader([]byte(schema)))
	if err != nil {
		b.Fatal(err)
	}
	c := santhosh.NewCompiler()
	if err := c.AddResource("https://bench.example/meter-values", doc); err != nil {
		b.Fatal(err)
	}
	sch, err := c.Compile("https://bench.example/meter-values")
	if err != nil {
		b.Fatal(err)
	}
	return sch
}

func kaptinSchema(b testing.TB) *kaptin.Schema {
	sch, err := kaptin.NewCompiler().Compile([]byte(schema))
	if err != nil {
		b.Fatal(err)
	}
	return sch
}

// TestAgree checks that the three validators agree on both instances, so
// that the benchmarks compare the same work.
func TestAgree(t *testing.T) {
	s, sa, k := sigmaSchema(t), santhoshSchema(t), kaptinSchema(t)
	for _, c := range []struct {
		name string
		data []byte
		ok   bool
	}{{"valid", valid, true}, {"invalid", invalid, false}} {
		if got := s.Validate("meter-values.json", decode(c.data)) == nil; got != c.ok {
			t.Errorf("%s: cB-Sigma says %v", c.name, got)
		}
		if got := sa.Validate(decode(c.data)) == nil; got != c.ok {
			t.Errorf("%s: santhosh-tekuri says %v", c.name, got)
		}
		var inst any
		if err := json.Unmarshal(c.data, &inst); err != nil {
			t.Fatal(err)
		}
		if got := k.Validate(inst).IsValid(); got != c.ok {
			t.Errorf("%s: kaptinlin says %v", c.name, got)
		}
	}
	// cB-Sigma reports each of the four failures
	err := s.Validate("meter-values.json", decode(invalid))
	if ve, ok := err.(sigma.ValidationError); !ok || len(ve) != 4 {
		t.Errorf("cB-Sigma failures: %v", err)
	}
}

func BenchmarkCompile(b *testing.B) {
	b.Run("cB-Sigma", func(b *testing.B) {
		for b.Loop() {
			sigmaSchema(b)
		}
	})
	b.Run("santhosh-tekuri", func(b *testing.B) {
		for b.Loop() {
			santhoshSchema(b)
		}
	})
	b.Run("kaptinlin", func(b *testing.B) {
		for b.Loop() {
			kaptinSchema(b)
		}
	})
}

// BenchmarkValidate validates an instance decoded once (each library's
// own decoding is excluded).
func BenchmarkValidate(b *testing.B) {
	s, sa, k := sigmaSchema(b), santhoshSchema(b), kaptinSchema(b)
	for _, c := range []struct {
		name string
		data []byte
	}{{"valid", valid}, {"invalid", invalid}} {
		num := decode(c.data)
		var plain any
		if err := json.Unmarshal(c.data, &plain); err != nil {
			b.Fatal(err)
		}
		b.Run(c.name+"/cB-Sigma", func(b *testing.B) {
			for b.Loop() {
				_ = s.Validate("meter-values.json", num)
			}
		})
		b.Run(c.name+"/santhosh-tekuri", func(b *testing.B) {
			for b.Loop() {
				_ = sa.Validate(num)
			}
		})
		b.Run(c.name+"/kaptinlin", func(b *testing.B) {
			for b.Loop() {
				_ = k.Validate(plain).IsValid()
			}
		})
	}
}

// BenchmarkValidateJSON includes decoding the bytes.
func BenchmarkValidateJSON(b *testing.B) {
	s, sa, k := sigmaSchema(b), santhoshSchema(b), kaptinSchema(b)
	for _, c := range []struct {
		name string
		data []byte
	}{{"valid", valid}, {"invalid", invalid}} {
		b.Run(c.name+"/cB-Sigma", func(b *testing.B) {
			for b.Loop() {
				_ = s.ValidateJSON("meter-values.json", c.data)
			}
		})
		b.Run(c.name+"/santhosh-tekuri", func(b *testing.B) {
			for b.Loop() {
				v, _ := santhosh.UnmarshalJSON(bytes.NewReader(c.data))
				_ = sa.Validate(v)
			}
		})
		b.Run(c.name+"/kaptinlin", func(b *testing.B) {
			for b.Loop() {
				_ = k.ValidateJSON(c.data).IsValid()
			}
		})
	}
}

// BenchmarkOutput is the cost of this module's output formats on the
// invalid instance.
func BenchmarkOutput(b *testing.B) {
	s := sigmaSchema(b)
	num := decode(invalid)
	for _, f := range []struct {
		name string
		f    sigma.OutputFormat
	}{{"basic", sigma.OutputBasic}, {"detailed", sigma.OutputDetailed}, {"verbose", sigma.OutputVerbose}} {
		b.Run(f.name, func(b *testing.B) {
			for b.Loop() {
				if _, err := s.Output("meter-values.json", num, f.f); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
