// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package testsuite

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/cB-Sigma/jsonschema"
)

// TestCheckSchemasSuite runs CheckSchemas, with no dialect, over every
// schema of the suite's required 2019-09 and 2020-12 tests: it must walk
// their keywords and references ($defs, anchors, $dynamicRef,
// $recursiveRef, prefixItems, dependentSchemas, unevaluated*) without
// refusing them. What it may refuse: keywords no draft defines (the suite
// tests that unknown keywords are ignored), references to meta-schemas
// that are not in the set, and schemas that loop without reaching a value.
func TestCheckSchemasSuite(t *testing.T) {
	for _, dr := range drafts {
		remotes, err := loadRemotes(dr.meta)
		if err != nil {
			t.Fatal(err)
		}
		root := filepath.Join(suiteDir, "tests", dr.dir)
		var files []string
		_ = filepath.WalkDir(root, func(p string, e fs.DirEntry, err error) error {
			if err == nil && !e.IsDir() && strings.HasSuffix(p, ".json") && !strings.Contains(p, "optional") {
				files = append(files, p)
			}
			return err
		})
		sort.Strings(files)
		checked := 0
		for _, f := range files {
			raw, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			var groups []group
			if err := json.Unmarshal(raw, &groups); err != nil {
				t.Fatal(err)
			}
			for _, g := range groups {
				set := jsonschema.NewSet()
				for _, r := range remotes {
					if err := set.Add(r.url, r.raw); err != nil {
						t.Fatal(err)
					}
				}
				sch, err := withDialect(g.Schema, dr.meta)
				if err != nil {
					t.Fatal(err)
				}
				if err := set.Add(schemaName, sch); err != nil {
					t.Fatal(err)
				}
				checked++
				_, err = set.CheckSchemas(schemaName, "")
				var ve jsonschema.ValidationError
				if err == nil {
					continue
				}
				if !asVE(err, &ve) {
					t.Errorf("%s: %s: %v", f, g.Description, err)
					continue
				}
				for _, e := range ve {
					switch {
					case strings.Contains(e.Message, "is not supported") && !jsonschema.Keywords[e.Keyword] && !jsonschema.Keywords2020[e.Keyword]:
					case strings.Contains(e.Message, "no such schema in the set") && strings.Contains(e.Message, "json-schema.org"):
					case strings.Contains(e.Message, "refers back to itself"):
					default:
						t.Errorf("%s: %s: %s %s: %s", filepath.Base(f), g.Description, e.Path, e.Keyword, e.Message)
					}
				}
			}
		}
		t.Logf("%s: %d schemas checked", dr.dir, checked)
	}
}

func asVE(err error, ve *jsonschema.ValidationError) bool {
	v, ok := err.(jsonschema.ValidationError)
	if ok {
		*ve = v
	}
	return ok
}
