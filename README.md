# jsonschema

Validation against families of JSON Schema documents, with every failure as a JSON pointer, a keyword and a short message, in a stable order. A layer over santhosh-tekuri/jsonschema/v6 with its own format checkers and an ECMA-262 pattern engine.

[![Go Reference](https://pkg.go.dev/badge/github.com/cB-Sigma/jsonschema.svg)](https://pkg.go.dev/github.com/cB-Sigma/jsonschema) [![test](https://github.com/cB-Sigma/jsonschema/actions/workflows/test.yml/badge.svg)](https://github.com/cB-Sigma/jsonschema/actions/workflows/test.yml) [![Licence: Apache-2.0](https://img.shields.io/badge/licence-Apache--2.0-blue.svg)](LICENSE) [![Go](https://img.shields.io/badge/go-1.26-00ADD8.svg)](go.mod)

Published schema families, such as the Open Charge Alliance's OCPP and
OSCP schemas, are sets of files that refer to each other by file name,
`$id` or JSON pointer, and sometimes name a file differently from its
`$id`. This module keeps such a set in memory as added (corrections can be
applied to a decoded copy first), resolves the references, validates with
santhosh-tekuri/jsonschema/v6, and reports each failure in a form a
protocol reply can carry. Every format of JSON Schema 2020-12 is checked
by this module's own code, and patterns are read as ECMA-262 regular
expressions. A pre-pass (`CheckSchemas`) refuses schemas that use keywords
outside a dialect; the OCA's OCPP dialect is included. No cgo. Written for
SigmaM2M, a oneM2M platform, where it validates OCPP 1.6 to 2.1 and OSCP
2.0 messages and its OpenAPI requests.

## Features

| Feature | Status |
|---|---|
| Drafts 4, 6, 7, 2019-09 and 2020-12, chosen by each document's `$schema`; `Options.DefaultDraft` for documents that name none (draft-07 if unset) | yes (the library's) |
| Schema families: references by `$id`, by a document added under the whole URL, then by file name; nothing fetched from the network | yes |
| Failures as pointer, keyword and our message, in a stable order (`ValidationError`, `Translate`, `Style`) | yes |
| Numbers kept exact (`Decode` with `json.Number`; `integer` accepts any number whose fraction is zero) | yes |
| `format` as each draft says: an assertion in drafts 4, 6 and 7, an annotation in 2019-09 and 2020-12; `Options.AssertFormat` asserts it everywhere; the 2020-12 format-assertion vocabulary is honoured | yes |
| Formats date-time, date, time (ranges, offsets, leap seconds), duration, email, idn-email, hostname, idn-hostname, ipv4, ipv6, uri, uri-reference, iri, iri-reference, uri-template, json-pointer, relative-json-pointer, uuid, regex, all with our messages | yes |
| idn-hostname by IDNA2008 (RFC 5891 to 5893): Punycode round trip, derived property, CONTEXTJ and CONTEXTO rules, Bidi rule, after a lookup mapping | yes, see the notes below |
| `pattern`, `patternProperties` and format `regex` as ECMA-262 with the u flag: lookahead, lookbehind, back references, named groups, `\p{...}`, pattern modifiers (`CompileRegexp`); `Options.GoRegexp` keeps Go's RE2 syntax | yes |
| `contentEncoding`, `contentMediaType` and `contentSchema` as assertions (`Options.AssertContent`); annotations by default | yes |
| Dialects: refuse keywords outside an allowed set before use (`Dialect`, `CheckSchemas`, `OCA()`); 2019-09 and 2020-12 keywords, embedded `$id` resources, `$anchor`, `$dynamicRef` and `$recursiveRef` followed | yes |
| `$dynamicRef`, `$recursiveRef` and `$vocabulary` across a family of documents (custom meta-schemas in the set) | yes (the library's), tested here |
| Output formats of the specification: flag, basic, detailed, verbose, with annotations (`Set.Output`, `OutputUnit`) | yes, see the notes below |
| A number written with a fraction or exponent (`1.0`) refused as `integer` in draft-04 | yes, with `Options.Draft4Integer`; accepted by default |
| Validate a Go value (struct tags and `MarshalJSON` apply) | yes (`Set.ValidateGo`) |
| Build schemas in code | as decoded values (`AddValue`); no builder API |

Notes on the checkers:

- **idn-hostname** maps each label by the UTS 46 mapping table
  (nontransitional) as golang.org/x/net/idna carries it, then checks it
  by IDNA2008. The IDNA2008 derived property is computed from Go's
  Unicode tables and golang.org/x/text, not read from the IANA registry,
  so it follows the Unicode version of the Go release used. The joining
  types for the CONTEXTJ rules come from golang.org/x/net/idna.
- **Patterns** that need no lookaround, back reference, multiline anchor
  or counted repeat above 1000 are translated to Go's RE2 syntax and match
  in linear time. Others run on a backtracking matcher that follows the
  ECMA-262 matching rules and gives up, as no match, after `MaxSteps`
  steps (about 40 ms on the laptop it was measured on), so a hostile input cannot hold a validator. Case
  folding under `i` is simple case folding. `\p{Script=...}` takes script
  names (`Greek`), not four-letter codes (`Grek`), and Script_Extensions
  is read as Script. Binary properties are those in Go's tables plus
  Any, ASCII, Assigned, Alphabetic, Lowercase, Uppercase, Math, ID_Start,
  ID_Continue and Default_Ignorable_Code_Point; emoji properties are
  refused at compile.
- **email** is an RFC 5321 mailbox: dot-string or quoted local part of at
  most 64 octets, a host name or an IPv4 or IPv6 address literal. Comments,
  display names and other address literals are refused.

Notes on the output formats:

- Failures in basic and detailed are the library's, found in context,
  with our messages in `error`. The verbose tree evaluates each subschema
  on the part of the instance it applies to and records valid branches
  too; a subschema reached through `$dynamicRef` or `$recursiveRef` is
  shown at its static target. When that alone cannot explain a failure
  (the dynamic scope changed the target), verbose shows the library's
  failure tree instead. The result's `valid` is always that of `Validate`.
- Annotations come from title, description, default, examples,
  deprecated, readOnly, writeOnly, format, the content keywords, and the
  applicators (properties, patternProperties, additionalProperties,
  unevaluatedProperties, prefixItems, items, contains, unevaluatedItems).
  Unknown keywords are not collected as annotations.
- `absoluteKeywordLocation` is the `$id` of the enclosing schema resource
  and a pointer in it; a document without `$id` is `mem:///` and its name
  in the set.
- The library's own `OutputUnit` names the property
  `AbsoluteKeywordLocation` (capital A) in JSON; this module's uses the
  specification's `absoluteKeywordLocation`.
- Verbose evaluates each subschema again, so it costs several times a
  validation (see Benchmarks).

## Choosing a Go JSON Schema validator

Compared from each project's source or README at the versions named, in
October 2026. Check the projects themselves before relying on a row.

| | this module v0.3.0 | santhosh-tekuri/jsonschema v6.0.3 | kaptinlin/jsonschema v0.9.10 | xeipuuv/gojsonschema v1.2.0 |
|---|---|---|---|---|
| What it is | a layer over santhosh-tekuri v6 | a validator | a validator | a validator |
| Drafts | 4, 6, 7, 2019-09, 2020-12 | 4, 6, 7, 2019-09, 2020-12 | 4, 6, 7, 2019-09, 2020-12 | 4, 6, 7 |
| Default draft without `$schema` | 7, settable | 2020-12, settable | 2020-12 | not compared |
| JSON Schema Test Suite | every required and optional test passes, and the output tests (below) | run in its CI | vendored as a git submodule | not compared |
| Formats idn-hostname, idn-email | yes | no checker | not compared | not compared |
| Patterns | ECMA-262 (u flag), RE2 where possible | Go RE2, or an engine you plug in | Go RE2 | Go RE2 |
| Families with mismatched file names and `$id` | yes | through a custom loader | not compared | not compared |
| Failures | a list in a stable order with our wording; the specification's flag, basic, detailed and verbose | flag, basic and detailed (no verbose, no annotations) | flag and list (the newer output proposal) | its own |
| Keyword dialect refusal before use | yes | vocabularies (2019-09 and later) | dialects | no |
| Go structs | as their JSON (`ValidateGo`) | no | validated directly, and unmarshalled with defaults | no |
| Schemas built in code | as decoded values | no | a constructor API | no |
| Last release | v0.x | 2026-08 | 2026-10 | 2019-10 |

Pick this module when you validate protocol messages against a family of
published schemas and need failures you can put in a reply, in the same
order every time, or when patterns and formats must follow the
specification exactly. Pick santhosh-tekuri/jsonschema/v6 directly when
you want your own extension vocabularies or the fastest failure path (it
is the engine here; see Benchmarks). Pick kaptinlin/jsonschema to build
schemas in code with a typed API or to fill struct defaults while
unmarshalling.

### Benchmarks

The nested module `bench/` (Go 1.27, which kaptinlin v0.9.10 requires)
validates one OCPP-shaped 2020-12 schema (nested objects and arrays,
`$ref`, enums, bounds, a pattern, a format) with each validator, after
`TestAgree` checks that they agree. On an Apple M4 Pro, Go 1.27.0,
`go test -bench . -benchmem` gave:

| | this module | santhosh-tekuri v6.0.3 | kaptinlin v0.9.10 |
|---|---|---|---|
| Compile the schema | 137 µs | 143 µs | 41 µs |
| Validate a decoded valid instance | 4.7 µs | 4.9 µs | 23 µs |
| Validate a decoded instance with four failures | 6.5 µs (and the failure list) | 3.6 µs | 18 µs |
| Decode and validate, valid | 10.8 µs | 10.5 µs | 28 µs |
| Decode and validate, four failures | 10.6 µs | 7.5 µs | 22 µs |

This module adds nothing to a valid instance; on a failure it spends
about 3 µs building the ordered list. `Output` on the failing instance
took 15 µs (basic, detailed) and 72 µs (verbose).

## Install

```
go get github.com/cB-Sigma/jsonschema@latest
```

Requires Go 1.26 or later.

## Quick start

```go
import "github.com/cB-Sigma/jsonschema"

set := jsonschema.NewSet()
err := set.Add("meter.json", meterSchema)    // refers to "reading.json"
err = set.Add("reading.json", readingSchema)

err = set.ValidateJSON("meter.json", body)
var v jsonschema.ValidationError
if errors.As(err, &v) {
	for _, e := range v {
		fmt.Println(e.Path, e.Keyword, e.Message) // "/reading/kWh minimum ..."
	}
}
```

To assert `format` in 2019-09 and 2020-12 documents too:

```go
set := jsonschema.NewSetWith(jsonschema.Options{AssertFormat: true})
```

`Example` and `ExampleNewSetWith` in example_test.go run these and check
their output.

## Documentation

API reference: https://pkg.go.dev/github.com/cB-Sigma/jsonschema

| Package | What it does |
|---|---|
| `jsonschema` (module root) | `NewSet`, `NewSetWith` and `Options`, `NewDialectSet`, `Set.Add`, `AddValue`, `Compile`, `ValidateJSON`, `Validate`, `ValidateGo`, `Decode`; `Output`, `OutputFormat` and `OutputUnit` (the specification's output formats); `ValidationError`, `Translate` and `Style` (the wording of the library's failures, for callers that compile with the library themselves); `CompileRegexp` and `Regexp` (ECMA-262 patterns, usable as the library's regexp engine); `Dialect`, `CheckSchemas`, `OCA()` |
| `testsuite` (nested module, tests only) | runs the JSON Schema Test Suite against this module |
| `bench` (nested module, benchmarks only) | compares this module with santhosh-tekuri/jsonschema/v6 and kaptinlin/jsonschema |

## Testing

```
go test -race ./...
```

Tests carry `// Covers: SIG-...` comments: these are requirement IDs of
the SigmaM2M project's compliance report and have no meaning outside it.
`FuzzRegexp` checks that a translated pattern matches as the backtracking
matcher does; `FuzzFormats` runs every checker on arbitrary strings.

### JSON Schema Test Suite

The [JSON-Schema-Test-Suite](https://github.com/json-schema-org/JSON-Schema-Test-Suite)
(MIT) is vendored in the test-only nested module `testsuite/`, at commit
`7de0e6a06031ede80028583dd45d0cd41105ae5b` (2026-10-09). Its test runs
every case of draft4, draft6, draft7, draft2019-09 and draft2020-12,
required and optional, through `NewSetWith`, `Set.Add`, `Set.Compile`,
`Decode` and `Set.Validate`, with the suite's remotes added to each set
under their `http://localhost:1234/` URLs. A schema without `$schema` is
given the directory's draft before `Add`, which is how a caller picks a
draft other than the draft-07 default.

Each file runs with the options its tests are written for: the files
under `optional/format/` with `AssertFormat`, draft7's
`optional/content` with `AssertContent`, draft4's
`optional/zeroTerminatedFloats` with `Draft4Integer`, and every other
file, the required `format.json` tests included, with the defaults
(ECMA-262 patterns). Every test is also run through `Set.Output` in the
basic, detailed and verbose formats, which must agree with `Validate`
and list at least one error for a failure.

`TestOutputSuite` runs the suite's output tests (`output-tests/`, 4 for
2019-09 and 4 for 2020-12, all passing): the basic output must match each
test's schema, and all four formats must match the specification's
output schema. `TestCheckSchemasSuite` runs `CheckSchemas` over all 1,408
schemas of the required tests of every draft; it must follow their
references (embedded `$id` resources, anchors, `$dynamicRef`,
`$recursiveRef`, draft-04 `id`) without refusing any.

| Draft | Required | Optional | Optional, format |
|---|---|---|---|
| draft4 | 618/618 | 100/100 | 291/291 |
| draft6 | 841/841 | 106/106 | 414/414 |
| draft7 | 929/929 | 118/118 | 804/804 |
| draft2019-09 | 1261/1261 | 158/158 | 885/885 |
| draft2020-12 | 1301/1301 | 162/162 | 885/885 |

v0.1.0 passed 1292/1301 required tests of draft2020-12 (format was always
asserted) and failed 841 optional tests (formats not checked, laxer email
and URI checkers, Go RE2 patterns, content not asserted).

Run it with `cd testsuite && go test -v ./...`. The test fails when a test
outside `testsuite/testdata/known-failures.txt` fails, when a listed test
passes, or when a listed test is no longer in the suite, for required and
optional tests alike. Since v0.3.0 the list is empty; v0.2.0 listed one
test (draft4 optional: `1.0` as an integer), now passed with
`Draft4Integer`.

The run found and fixed, in v0.1.0, two bugs in this module: a document
added under a full URL without an `$id` (as the suite's remotes are) was
not found by that URL, and the time and date-time checks accepted hours,
minutes and offsets out of range and refused leap seconds. In v0.2.0 it
found that the translation of a failed `contentSchema` reported the
decoded content's failures at the instance's pointers. In v0.3.0 it found
that `CheckSchemas` refused every 2019-09 and 2020-12 schema using `$defs`
and could not follow anchors, embedded `$id` resources or draft-04 `id`.

### Interop tested against

- The JSON Schema Test Suite, above.
- The OCA OSCP 2.0 schemas and OCPP 1.6, 2.0.1 and 2.1 schemas, through
  SigmaM2M's tests. On 2026-10-06 SigmaM2M's earlier hand-written
  validator and this module gave identical failure lists, in order, for
  146,000 generated OCPP messages (68,113 of them failing) across all OCA
  schemas.

## Related repositories

None. This module has no simulator.

## Status

v0.x: the API may change before v1. See [CHANGELOG.md](CHANGELOG.md).

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## Security

See [SECURITY.md](SECURITY.md) to report a vulnerability privately.

## Licence

Apache-2.0 (see [LICENSE](LICENSE) and [NOTICE](NOTICE)). Copyright 2026
Abhinav Gautam.
