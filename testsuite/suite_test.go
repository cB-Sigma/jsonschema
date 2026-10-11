// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

// Package testsuite runs the JSON Schema Test Suite (vendored under
// testdata, MIT licence) through the public API of
// github.com/cB-Sigma/jsonschema. It is a separate module so that the
// suite's files never reach the library's module.
package testsuite

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/cB-Sigma/jsonschema"
)

const (
	suiteDir   = "testdata/JSON-Schema-Test-Suite"
	knownFile  = "testdata/known-failures.txt"
	remoteBase = "http://localhost:1234/"
	schemaName = "suite-schema"
)

// dump, when set, writes every failing test to the file in the format of
// known-failures.txt, with the category "unclassified", for review.
var dump = flag.String("jsts.dump", "", "write the failing tests to this file")

// drafts are the suite directories run, with the meta-schema each
// directory's schemas are written for. The suite leaves the dialect of a
// schema without $schema to the runner; this runner states it by adding
// $schema to such schemas (and to the remotes), which is how a caller of
// this module chooses a draft other than the draft-07 default.
var drafts = []struct{ dir, meta string }{
	{"draft4", "http://json-schema.org/draft-04/schema#"},
	{"draft6", "http://json-schema.org/draft-06/schema#"},
	{"draft7", "http://json-schema.org/draft-07/schema#"},
	{"draft2019-09", "https://json-schema.org/draft/2019-09/schema"},
	{"draft2020-12", "https://json-schema.org/draft/2020-12/schema"},
}

type group struct {
	Description string          `json:"description"`
	Schema      json.RawMessage `json:"schema"`
	Tests       []struct {
		Description string          `json:"description"`
		Data        json.RawMessage `json:"data"`
		Valid       bool            `json:"valid"`
	} `json:"tests"`
}

// kind is where a test file sits in the suite.
type kind int

const (
	required kind = iota
	optional
	format
)

type tally struct{ pass, total [3]int }

type remote struct {
	url string
	raw []byte
}

func TestSuite(t *testing.T) {
	known, err := readKnown(knownFile)
	if err != nil {
		t.Fatal(err)
	}
	failing := map[string]string{} // id: what happened
	seen := map[string]bool{}
	tallies := map[string]*tally{}

	for _, dr := range drafts {
		remotes, err := loadRemotes(dr.meta)
		if err != nil {
			t.Fatal(err)
		}
		tl := &tally{}
		tallies[dr.dir] = tl
		root := filepath.Join(suiteDir, "tests", dr.dir)
		var files []string
		err = filepath.WalkDir(root, func(p string, e fs.DirEntry, err error) error {
			if err == nil && !e.IsDir() && strings.HasSuffix(p, ".json") {
				files = append(files, p)
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		sort.Strings(files)
		for _, f := range files {
			rel, _ := filepath.Rel(root, f)
			rel = strings.TrimSuffix(filepath.ToSlash(rel), ".json")
			k := required
			switch {
			case strings.HasPrefix(rel, "optional/format/"):
				k = format
			case strings.HasPrefix(rel, "optional/"):
				k = optional
			}
			raw, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			var groups []group
			if err := json.Unmarshal(raw, &groups); err != nil {
				t.Fatalf("%s: %v", f, err)
			}
			for _, g := range groups {
				results := runGroup(dr.meta, remotes, g)
				for i, tc := range g.Tests {
					id := dr.dir + "/" + rel + "\t" + g.Description + "\t" + tc.Description
					for n := 2; seen[id]; n++ {
						id = fmt.Sprintf("%s/%s\t%s\t%s (%d)", dr.dir, rel, g.Description, tc.Description, n)
					}
					seen[id] = true
					tl.total[k]++
					if results[i] == "" {
						tl.pass[k]++
					} else {
						failing[id] = results[i]
					}
				}
			}
		}
	}

	// The list of known failures must match exactly.
	var unexpected, fixed, stale []string
	for id, why := range failing {
		if _, ok := known[id]; !ok {
			unexpected = append(unexpected, id+"\t-> "+why)
		}
	}
	for id := range known {
		if !seen[id] {
			stale = append(stale, id)
		} else if _, ok := failing[id]; !ok {
			fixed = append(fixed, id)
		}
	}
	sort.Strings(unexpected)
	sort.Strings(fixed)
	sort.Strings(stale)
	for i, s := range unexpected {
		if i == 25 {
			t.Errorf("and %d more unexpected failures (-jsts.dump lists them)", len(unexpected)-i)
			break
		}
		t.Errorf("unexpected failure: %s", s)
	}
	for _, s := range fixed {
		t.Errorf("listed in %s but passes now (remove the line): %s", knownFile, s)
	}
	for _, s := range stale {
		t.Errorf("listed in %s but not in the suite: %s", knownFile, s)
	}

	if *dump != "" {
		ids := make([]string, 0, len(failing))
		for id := range failing {
			cat := "unclassified"
			if c, ok := known[id]; ok {
				cat = c
			}
			ids = append(ids, cat+"\t"+id+"\t# "+strings.ReplaceAll(failing[id], "\n", " "))
		}
		sort.Strings(ids)
		if err := os.WriteFile(*dump, []byte(strings.Join(ids, "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "\n%-14s %-16s %-16s %-16s\n", "draft", "required", "optional", "optional/format")
	for _, dr := range drafts {
		tl := tallies[dr.dir]
		fmt.Fprintf(&b, "%-14s %-16s %-16s %-16s\n", dr.dir,
			fmt.Sprintf("%d/%d", tl.pass[required], tl.total[required]),
			fmt.Sprintf("%d/%d", tl.pass[optional], tl.total[optional]),
			fmt.Sprintf("%d/%d", tl.pass[format], tl.total[format]))
	}
	cats := map[string]int{}
	for id := range failing {
		cats[known[id]]++
	}
	names := make([]string, 0, len(cats))
	for c := range cats {
		names = append(names, c)
	}
	sort.Strings(names)
	fmt.Fprintf(&b, "failures by category:\n")
	for _, c := range names {
		name := c
		if name == "" {
			name = "(not listed)"
		}
		fmt.Fprintf(&b, "  %-28s %d\n", name, cats[c])
	}
	t.Log(b.String())
}

// runGroup validates each test's data against the group's schema in a new
// set holding the remotes, and returns for each test "" when the outcome
// is the expected one, or what happened.
func runGroup(meta string, remotes []remote, g group) (out []string) {
	out = make([]string, len(g.Tests))
	defer func() {
		if r := recover(); r != nil {
			for i := range out {
				out[i] = fmt.Sprintf("panic: %v", r)
			}
		}
	}()
	set := jsonschema.NewSet()
	for _, r := range remotes {
		if err := set.Add(r.url, r.raw); err != nil {
			return fill(out, "remote: "+err.Error())
		}
	}
	sch, err := withDialect(g.Schema, meta)
	if err != nil {
		return fill(out, "schema: "+err.Error())
	}
	if err := set.Add(schemaName, sch); err != nil {
		return fill(out, "schema: "+err.Error())
	}
	if _, err := set.Compile(schemaName); err != nil {
		return fill(out, "compile: "+err.Error())
	}
	for i, tc := range g.Tests {
		inst, err := jsonschema.Decode(tc.Data)
		if err != nil {
			out[i] = "data: " + err.Error()
			continue
		}
		err = set.Validate(schemaName, inst)
		switch {
		case tc.Valid && err != nil:
			out[i] = "refused valid data: " + err.Error()
		case !tc.Valid && err == nil:
			out[i] = "accepted invalid data"
		}
	}
	return out
}

func fill(out []string, why string) []string {
	for i := range out {
		out[i] = why
	}
	return out
}

// withDialect adds $schema to an object schema that names none.
func withDialect(raw []byte, meta string) ([]byte, error) {
	v, err := jsonschema.Decode(raw)
	if err != nil {
		return nil, err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return raw, nil
	}
	if _, has := m["$schema"]; has {
		return raw, nil
	}
	m["$schema"] = meta
	return json.Marshal(m)
}

// loadRemotes reads the suite's remotes, each under the URL the suite
// serves it at.
func loadRemotes(meta string) ([]remote, error) {
	dir := filepath.Join(suiteDir, "remotes")
	var out []remote
	err := filepath.WalkDir(dir, func(p string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() || !strings.HasSuffix(p, ".json") {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if raw, err = withDialect(raw, meta); err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		out = append(out, remote{url: remoteBase + path.Clean(filepath.ToSlash(rel)), raw: raw})
		return nil
	})
	return out, err
}

// readKnown reads known-failures.txt: lines of category, draft/file, group
// and test separated by tabs; "#" starts a comment line.
func readKnown(name string) (map[string]string, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string]string{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if i := strings.Index(line, "\t# "); i >= 0 {
			line = line[:i]
		}
		parts := strings.Split(line, "\t")
		if len(parts) != 4 {
			return nil, fmt.Errorf("%s:%d: want category, draft/file, group and test separated by tabs", name, n)
		}
		id := strings.Join(parts[1:], "\t")
		if _, dup := out[id]; dup {
			return nil, fmt.Errorf("%s:%d: listed twice", name, n)
		}
		out[id] = parts[0]
	}
	return out, sc.Err()
}
