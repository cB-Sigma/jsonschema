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

## Other implementations

The validator is santhosh-tekuri/jsonschema/v6 (see NOTICE). The wording
of failures was kept from the earlier validator so that SigmaM2M's error
texts (OCPP CALLERROR descriptions, OSCP and OpenAPI replies) did not
change; the library's own messages are not reproduced.
