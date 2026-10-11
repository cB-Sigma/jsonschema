# Changelog

## v0.3.0 (2026-10-11)

Every required and optional test of the JSON Schema Test Suite passes in
every draft, and the suite's output tests pass. Existing functions keep
their signatures and their behaviour with the default options, except
for the fixes to `CheckSchemas` below.

### Added

- `Set.Output`, `OutputFormat` and `OutputUnit`: the specification's
  output formats flag, basic, detailed and verbose, with annotations, our
  messages, and `absoluteKeywordLocation` under the specification's name.
- `Options.DefaultDraft`: the draft of documents without `$schema`
  (draft-07 if unset, as before).
- `Options.Draft4Integer`: in draft-04 documents a number written with a
  fraction or exponent (`1.0`) is not an integer, as draft-04 defines it.
  The suite's last listed failure passes with it.
- `Set.ValidateGo`: validate a Go value as its JSON encoding.
- `Keywords2020`: the 2019-09 and 2020-12 keywords `CheckSchemas` accepts
  in documents of those drafts.
- Tests of `$dynamicRef`, `$recursiveRef` and `$vocabulary` across a
  family (custom meta-schemas in the set), the suite's output tests
  (`TestOutputSuite`), `CheckSchemas` over every schema of the suite
  (`TestCheckSchemasSuite`), every suite test through `Output`, and
  `FuzzOutput`.
- The nested module `bench/`: benchmarks against
  santhosh-tekuri/jsonschema/v6 v6.0.3 and kaptinlin/jsonschema v0.9.10.

### Changed

- idn-hostname maps labels by the UTS 46 mapping table of
  golang.org/x/net/idna instead of an approximation (NFKC, case folding
  and dropping default-ignorables).

### Fixed

- `CheckSchemas` refused 2019-09 and 2020-12 keywords (`$defs`,
  `$dynamicRef`, `prefixItems` and the others) in documents of those
  drafts, ignored siblings of `$ref` there, and did not walk `$defs`,
  `dependentSchemas`, `prefixItems`, `unevaluatedProperties`,
  `unevaluatedItems` or `contentSchema`.
- `CheckSchemas` resolved references only against top-level `$id`s: it
  now uses the base URI in force at the reference (embedded resources
  with their own `$id`, draft-04 `id`), and follows anchors (`$anchor`,
  `$dynamicAnchor`, draft-06/07 `"$id": "#name"`).
- A draft-04 document's `id` now identifies it in the set, as `$id` does
  in later drafts.

## v0.2.0 (2026-10-11)

Every "no" of the v0.1 feature table that the JSON Schema Test Suite
measures is now implemented: all required tests pass in every draft, and
all optional tests but one (below). Existing functions keep their
signatures; see "Changed" for behaviour that moved towards the
specification.

### Added

- `Options` and `NewSetWith`: `Dialect` (what `NewDialectSet` sets),
  `AssertFormat` (assert `format` in every draft), `AssertContent`
  (assert `contentEncoding`, `contentMediaType` and `contentSchema`) and
  `GoRegexp` (Go's RE2 pattern syntax, as in v0.1).
- Format checkers for duration (RFC 3339 appendix A), hostname (RFC 1123,
  with A-labels checked), idn-hostname (IDNA2008: Punycode, derived
  property, CONTEXTJ, CONTEXTO, Bidi rule, after a lookup mapping),
  idn-email (RFC 6531), iri and iri-reference (RFC 3987), uri-template
  (RFC 6570), json-pointer (RFC 6901) and relative-json-pointer.
- `CompileRegexp` and `Regexp`: ECMA-262 regular expressions with the u
  flag, translated to RE2 when they can be and matched by a bounded
  backtracking matcher otherwise (lookahead, lookbehind, back references,
  named groups, `\p{...}`, pattern modifiers); `MaxSteps`.
- A 2020-12 meta-schema that lists the format-assertion vocabulary, as
  required or optional, makes `format` an assertion.
- Our own messages for failed `contentEncoding`, `contentMediaType` and
  `contentSchema`.
- The test-suite runner also runs the optional tests, each file with the
  options its tests are written for, against the known-failures list;
  fuzz tests for patterns and formats.
- New dependency: golang.org/x/net (its idna package, for joining types).

### Changed

- `format` is an annotation by default in drafts 2019-09 and 2020-12, as
  those drafts say; drafts 4, 6 and 7 (and documents without `$schema`)
  still assert it. Use `Options.AssertFormat` to keep v0.1's behaviour.
- Patterns are ECMA-262 by default: `\s`, `.`, `$`, `\cX`, `[]` and `[^]`
  mean what they mean in ECMA-262, and syntax that ECMA-262 refuses
  (`(?i)`, `(?P<name>)`, `\a`, `\z`) is refused. Use `Options.GoRegexp`
  to keep v0.1's behaviour.
- email follows RFC 5321 instead of Go's net/mail (quoted local parts and
  IPv6 literals accepted; non-ASCII, comments and display names refused);
  uri and uri-reference follow RFC 3986's grammar instead of Go's net/url.
- hostname, duration, json-pointer, relative-json-pointer, iri,
  iri-reference and uri-template are now checked where format is
  asserted; v0.1 accepted any string. Unknown formats are still accepted.
- A failed `contentSchema` no longer reports the decoded content's
  failures at the instance's pointers.

### Not changed

- `integer` accepts a number with a zero fraction in every draft, draft-04
  included (the one optional test of the suite this module fails).

## v0.1.0 (2026-10-11)

First release: schema families with references by `$id`, URL and file
name; failures as pointer, keyword and message in a stable order; format
checkers for date-time, date, time, email, uri, uri-reference, ipv4, ipv6
and uuid; keyword dialects with the OCA's OCPP dialect; moved from
SigmaM2M (see PROVENANCE.md).
