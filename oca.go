package jsonschema

// OCA is the dialect of the Open Charge Alliance's OCPP schemas: OCPP 1.6
// (draft-04), OCPP 2.0.1 and 2.1 (draft-06). What they use is a subset of
// draft-07 with the same meaning, plus annotations: "id" (the draft-04
// document identifier), "javaType" and "comment". Any other keyword is
// refused by CheckSchemas, so a later schema that relies on a keyword this
// package does not implement cannot be checked only halfway. The draft-04
// boolean form of exclusiveMinimum/exclusiveMaximum means something else
// in draft-07 and is refused too.
func OCA() *Dialect {
	shape := func(k string) Keyword {
		return Keyword{Check: func(c *CheckContext, _ map[string]any, v any) {
			if _, isBool := v.(bool); isBool {
				c.Fail("the draft-04 boolean form of %s is not supported", k)
			}
		}}
	}
	return &Dialect{
		Keywords: map[string]Keyword{"exclusiveMinimum": shape("exclusiveMinimum"), "exclusiveMaximum": shape("exclusiveMaximum")},
		Allowed: func(k string) bool {
			switch k {
			case "id", "javaType", "comment":
				return true
			}
			return keywords[k]
		},
	}
}
