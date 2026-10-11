# Changelog

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
