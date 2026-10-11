# jsonschema

Validation against families of JSON Schema documents, with every failure as a JSON pointer, a keyword and a short message, in a stable order. A layer over santhosh-tekuri/jsonschema/v6.

[![Go Reference](https://pkg.go.dev/badge/github.com/cB-Sigma/jsonschema.svg)](https://pkg.go.dev/github.com/cB-Sigma/jsonschema) [![test](https://github.com/cB-Sigma/jsonschema/actions/workflows/test.yml/badge.svg)](https://github.com/cB-Sigma/jsonschema/actions/workflows/test.yml) [![Licence: Apache-2.0](https://img.shields.io/badge/licence-Apache--2.0-blue.svg)](LICENSE) [![Go](https://img.shields.io/badge/go-1.26-00ADD8.svg)](go.mod)

Published schema families, such as the Open Charge Alliance's OCPP and
OSCP schemas, are sets of files that refer to each other by file name,
`$id` or JSON pointer, and sometimes name a file differently from its
`$id`. This module keeps such a set in memory as added (corrections can be
applied to a decoded copy first), resolves the references, validates with
santhosh-tekuri/jsonschema/v6, and reports each failure in a form a
protocol reply can carry. A pre-pass (`CheckSchemas`) refuses schemas that
use keywords outside a dialect; the OCA's OCPP dialect is included. No
cgo. Written for SigmaM2M, a oneM2M platform, where it validates OCPP
1.6 to 2.1 and OSCP 2.0 messages and its OpenAPI requests.

## Features

| Feature | Status |
|---|---|
| Drafts 4, 6, 7, 2019-09 and 2020-12, chosen by each document's `$schema` (draft-07 when it names none) | yes (the library's) |
| Schema families: references by `$id`, by a document added under the whole URL, then by file name; nothing fetched from the network | yes |
| Failures as pointer, keyword and our message, in a stable order (`ValidationError`, `Translate`, `Style`) | yes |
| Numbers kept exact (`Decode` with `json.Number`; `integer` accepts any number whose fraction is zero) | yes |
| Formats checked with our messages: date-time, date, time (with ranges and leap seconds), email, uri, uri-reference, ipv4, ipv6, uuid | yes |
| Formats hostname, duration, json-pointer, relative-json-pointer, iri, iri-reference, uri-template | not checked (accepted) |
| Formats idn-hostname, idn-email | not checked (the library has no checker) |
| `format` as an annotation only, as 2020-12 defaults to | no: always asserted |
| Dialects: refuse keywords outside an allowed set before use (`Dialect`, `CheckSchemas`, `OCA()`) | yes |
| `pattern` with ECMA-262 semantics | no: Go RE2 syntax (the library's) |
| Output formats of the specification (flag, basic, detailed) | no: use the library directly |

## Choosing a Go JSON Schema validator

Compared from each project's source or README at the versions named, in
October 2026. Check the projects themselves before relying on a row.

| | this module | santhosh-tekuri/jsonschema v6.0.3 | kaptinlin/jsonschema v0.9.9 | xeipuuv/gojsonschema v1.2.0 |
|---|---|---|---|---|
| What it is | a layer over santhosh-tekuri v6 | a validator | a validator | a validator |
| Drafts | 4, 6, 7, 2019-09, 2020-12 | 4, 6, 7, 2019-09, 2020-12 | 4, 6, 7, 2019-09, 2020-12 | 4, 6, 7 |
| Default draft without `$schema` | 7 | 2020-12 | 2020-12 | not compared |
| JSON Schema Test Suite | run here (below) | run in its CI | not stated in its README | not compared |
| Families with mismatched file names and `$id` | yes | through a custom loader | not compared | not compared |
| Failure list in a stable order, own wording | yes | output formats of the specification | its own | its own |
| Keyword dialect refusal before use | yes | vocabularies (2019-09 and later) | not compared | no |
| Last release | v0.x | 2026-08 | 2026-08 | 2019-10 |

Pick this module when you validate protocol messages against a family of
published schemas and need failures you can put in a reply, in the same
order every time. Pick santhosh-tekuri/jsonschema/v6 directly for
everything else: it is the engine here, and gives the specification's
output formats. Pick kaptinlin/jsonschema to validate Go structs or to
build schemas in code.

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

`Example` in example_test.go runs this and checks its output.

## Documentation

API reference: https://pkg.go.dev/github.com/cB-Sigma/jsonschema

| Package | What it does |
|---|---|
| `jsonschema` (module root) | `NewSet`, `NewDialectSet`, `Set.Add`, `AddValue`, `Compile`, `ValidateJSON`, `Validate`, `Decode`; `ValidationError`, `Translate` and `Style` (the wording of the library's failures, for callers that compile with the library themselves); `Dialect`, `CheckSchemas`, `OCA()` |
| `testsuite` (nested module, tests only) | runs the JSON Schema Test Suite against this module |

## Testing

```
go test -race ./...
```

Tests carry `// Covers: SIG-...` comments: these are requirement IDs of
the SigmaM2M project's compliance report and have no meaning outside it.

### JSON Schema Test Suite

The [JSON-Schema-Test-Suite](https://github.com/json-schema-org/JSON-Schema-Test-Suite)
(MIT) is vendored in the test-only nested module `testsuite/`, at commit
`7de0e6a06031ede80028583dd45d0cd41105ae5b` (2026-10-09). Its test runs
every case of draft4, draft6, draft7, draft2019-09 and draft2020-12
through `NewSet`, `Set.Add`, `Set.Compile`, `Decode` and `Set.Validate`,
with the suite's remotes added to each set under their
`http://localhost:1234/` URLs. A schema without `$schema` is given the
directory's draft before `Add`, which is how a caller picks a draft other
than the draft-07 default.

| Draft | Required | Optional | Optional, format |
|---|---|---|---|
| draft4 | 618/618 | 85/100 | 253/291 |
| draft6 | 841/841 | 92/106 | 336/414 |
| draft7 | 929/929 | 100/118 | 607/804 |
| draft2019-09 | 1261/1261 | 144/158 | 663/885 |
| draft2020-12 | 1292/1301 | 148/162 | 663/885 |

Run it with `cd testsuite && go test -v ./...`. The test fails when a test
outside `testsuite/testdata/known-failures.txt` fails, when a listed test
passes, or when a listed test is no longer in the suite. The 841 listed
failures, by cause:

| Cause | Tests | What it is |
|---|---|---|
| format not checked | 408 | the formats listed as not checked above |
| own format checker | 133 | email follows Go's net/mail (no IPv6 literals or quoted local parts with spaces; accepts some invalid domains and non-ASCII); uri and uri-reference follow Go's net/url and accept characters the suite refuses |
| format unknown | 189 | idn-hostname and idn-email |
| RE2 patterns | 97 | `pattern`, `patternProperties` and format `regex` use Go's RE2 syntax, not ECMA-262 |
| format asserted | 9 | draft2020-12's required `format.json` expects `format` to be an annotation |
| content not asserted | 4 | draft7 optional: `contentEncoding` and `contentMediaType` carry no check |
| integer with zero fraction | 1 | draft4 optional: `1.0` counts as an integer in every draft |

Every required test passes except the nine on `format` as an annotation.
The run found and fixed two bugs in this module: a document added under
a full URL without an `$id` (as the suite's remotes are) was not found
by that URL, and the time and date-time checks accepted hours, minutes
and offsets out of range and refused leap seconds.

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

v0.x: the API may change before v1.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## Security

See [SECURITY.md](SECURITY.md) to report a vulnerability privately.

## Licence

Apache-2.0 (see [LICENSE](LICENSE) and [NOTICE](NOTICE)). Copyright 2026
Abhinav Gautam.
