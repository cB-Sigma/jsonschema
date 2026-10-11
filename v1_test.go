package jsonschema

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestKeywordsAreCopies(t *testing.T) {
	k := Keywords()
	if !slices.IsSorted(k) || !slices.Contains(k, "type") || slices.Contains(k, "$defs") {
		t.Fatalf("Keywords: %v", k)
	}
	k[0] = "vendorKeyword"
	if slices.Contains(Keywords(), "vendorKeyword") {
		t.Error("changing the returned slice changed the package's list")
	}
	k2 := Keywords2020()
	if !slices.IsSorted(k2) || !slices.Contains(k2, "$defs") || slices.Contains(k2, "type") {
		t.Fatalf("Keywords2020: %v", k2)
	}
}

func TestCompileReportsOnlyAnError(t *testing.T) {
	s := NewSet(Options{})
	if err := s.Add("a.json", []byte(`{"type":"string"}`)); err != nil {
		t.Fatal(err)
	}
	if err := s.Compile("a.json"); err != nil {
		t.Fatal(err)
	}
	err := s.Compile("missing.json")
	if err == nil || !strings.HasPrefix(err.Error(), "jsonschema: ") {
		t.Fatalf("missing document: %v", err)
	}
	if err := s.Add("b.json", []byte(`{}`)); err == nil || !strings.HasPrefix(err.Error(), "jsonschema: ") {
		t.Errorf("Add after Compile: %v", err)
	}
}

func TestValidationErrorHasNoPrefix(t *testing.T) {
	s := NewSet(Options{})
	if err := s.Add("a.json", []byte(`{"type":"string"}`)); err != nil {
		t.Fatal(err)
	}
	err := s.ValidateJSON("a.json", []byte(`1`))
	var ve ValidationError
	if !errors.As(err, &ve) || len(ve) != 1 || ve[0].Keyword != "type" {
		t.Fatalf("got %v", err)
	}
	if strings.HasPrefix(err.Error(), "jsonschema:") {
		t.Errorf("validation failures carry the package prefix: %q", err)
	}
}

func TestOtherErrorsCarryThePrefix(t *testing.T) {
	if _, err := Decode([]byte(`{`)); err == nil || !strings.HasPrefix(err.Error(), "jsonschema: ") {
		t.Errorf("Decode: %v", err)
	}
	if _, err := CompileRegexp(`(?i)x`); err == nil || !strings.HasPrefix(err.Error(), "jsonschema: ") {
		t.Errorf("CompileRegexp: %v", err)
	}
	if _, err := NewSet(Options{}).CheckSchemas("none.json"); err == nil || !strings.HasPrefix(err.Error(), "jsonschema: ") {
		t.Errorf("CheckSchemas: %v", err)
	}
}
