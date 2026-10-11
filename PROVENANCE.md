# Provenance

## How the code was written

Written for SigmaM2M. Until 2026-10-06 this module was a hand-written JSON
Schema draft-07 validator (from the IETF drafts
draft-handrews-json-schema-01 and draft-handrews-json-schema-validation-01;
no code was ported from another validator). On 2026-10-06 the validator
was replaced by github.com/santhosh-tekuri/jsonschema/v6, used unmodified
as a dependency. What remains ours: keeping a family of documents in memory
and resolving references by `$id` then file name (the loader), the format
checks and their messages, the translation of the library's failures into
pointer, keyword and message in a stable order (`Translate`), and the
`CheckSchemas` pre-pass with its dialects.

The OCA dialect (`oca.go`) describes what the Open Charge Alliance's OCPP
schemas use (draft-04 and draft-06 forms with annotations), read from
those schemas as facts; no schema is included here.

The code came with SigmaM2M's first commit (2026-09-27) and later
SigmaM2M commits, and became this module on 2026-10-06. SigmaM2M's OCPP,
OSCP and OpenAPI code use it.

On 2026-10-11 the JSON Schema Test Suite was run against it (the nested
module `testsuite`, which vendors the suite under its MIT licence; see
NOTICE). That run led to two fixes: documents added under a full URL are
found by that URL, and the time and date-time formats check ranges,
offsets and leap seconds (RFC 3339 section 5.6 and 5.7).

For v0.2.0 (2026-10-11) the rest of the formats and the pattern engine
were written for this module from the specifications, with no code ported
from another validator or regular-expression engine: RFC 3339 (duration),
RFC 1123 (hostname), RFC 5890, 5891, 5892 and 5893 (IDNA2008), RFC 3492
(Punycode), RFC 5321 and 6531 (email, idn-email), RFC 3986 and 3987 (URI,
IRI), RFC 6570 (URI templates), RFC 6901 and the relative JSON pointer
draft, and ECMA-262 (pattern syntax and matching, read with the u flag).
The suite's optional tests were the check. The IDNA2008 exception list
(RFC 5892 section 2.6) and the CONTEXTO rules are facts taken from that
RFC; the derived property is computed from Go's Unicode tables; the
CONTEXTJ rules are applied by golang.org/x/net/idna, used unmodified as a
dependency. SigmaM2M's OpenAPI code had a smaller ECMA-262 to RE2
translator (one top-level lookahead); `CompileRegexp` replaces that
approach and shares no code with it.

For v0.3.0 (2026-10-11) the output formats were written from the JSON
Schema 2020-12 core specification (section 12) and checked against the
suite's output tests and output schema; the walk over the library's
compiled schema uses its exported fields only. Reference resolution in
`CheckSchemas` follows RFC 3986 and the 2020-12 core specification's
rules for base URIs, `$anchor` and `$dynamicAnchor`. The draft-04 integer
rule is from the draft-04 core specification (draft-zyp-json-schema-04).
No code was ported from another validator; santhosh-tekuri's output
code was read to match its keyword locations for failures, and
kaptinlin/jsonschema was read (v0.9.10) only for the comparison in the
README and is used, unmodified, in the benchmarks.

## Other implementations

The validator is santhosh-tekuri/jsonschema/v6 (see NOTICE). The wording
of failures was kept from the earlier validator so that SigmaM2M's error
texts (OCPP CALLERROR descriptions, OSCP and OpenAPI replies) did not
change; the library's own messages are not reproduced.
