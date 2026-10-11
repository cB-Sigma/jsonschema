// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package jsonschema

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// Regexp is an ECMA-262 regular expression, read with the u flag (code
// points, strict escapes), as JSON Schema's "pattern",
// "patternProperties" and format "regex" use them.
//
// A pattern without lookaround, back references, multiline anchors or
// counted repeats above 1000 is translated to Go's regexp (RE2) syntax and
// matched in linear time; the ECMA-262 meaning of \s, ".", "$", \cX,
// empty classes and the rest is kept by the translation. Any other pattern
// is matched by a backtracking matcher that follows ECMA-262's matching
// rules, lookbehind included; a match that takes more than a fixed number
// of steps (see MaxSteps) is reported as no match.
type Regexp struct {
	src  string
	re   *regexp.Regexp // the RE2 translation, or nil
	root *rnode         // the parsed pattern, matched by backtracking when re is nil
	ncap int
}

// MaxSteps bounds the work of the backtracking matcher for one
// MatchString. A pattern that needs more steps does not match.
const MaxSteps = 1 << 22

// CompileRegexp parses an ECMA-262 regular expression (with the u flag)
// and refuses one the specification refuses: unknown escapes such as \a,
// inline flag groups such as (?i), Python's (?P<name>), ranges out of
// order, a quantifier with nothing to repeat, references to groups that do
// not exist, and unknown Unicode properties.
func CompileRegexp(src string) (*Regexp, error) {
	p := &rparser{src: []rune(src), names: map[string]int{}}
	root, err := p.parse()
	if err != nil {
		return nil, fmt.Errorf("jsonschema: pattern %q: %w", src, err)
	}
	r := &Regexp{src: src, ncap: p.ncap, root: root}
	if !p.backtrack {
		var b strings.Builder
		toRE2(root, &b)
		if re, err := regexp.Compile(b.String()); err == nil {
			r.re = re
		}
	}
	return r, nil
}

// MatchString reports whether the pattern matches anywhere in s.
func (r *Regexp) MatchString(s string) bool {
	if r.re != nil {
		return r.re.MatchString(s)
	}
	return r.backtrack(s)
}

func (r *Regexp) backtrack(s string) bool {
	m := &bt{in: []rune(s), caps: make([]int, 2*(r.ncap+1))}
	for start := 0; start <= len(m.in); start++ {
		for i := range m.caps {
			m.caps[i] = -1
		}
		if m.match(r.root, start, 1, func(int) bool { return true }) {
			return true
		}
		if m.steps > MaxSteps {
			return false
		}
	}
	return false
}

// String returns the source of the pattern.
func (r *Regexp) String() string { return r.src }

// rflags are the modifiers in effect: i (ignore case), m (multiline
// anchors), s (dot matches line terminators).
type rflags struct{ i, m, s bool }

type rkind int

const (
	rLit rkind = iota
	rClass
	rDot
	rBegin
	rEnd
	rWordB
	rNotWordB
	rCat
	rAlt
	rGroup
	rRepeat
	rLook
	rBackref
)

type rnode struct {
	kind         rkind
	r            rune
	cls          *rclass
	subs         []*rnode
	fl           rflags
	cap          int // rGroup: capture index (0: none); rBackref: the group
	name         string
	min, max     int // rRepeat; max -1: no limit
	greedy       bool
	behind, neg  bool
	capLo, capHi int // rRepeat: captures inside are capLo+1..capHi
}

type rrange struct{ lo, hi rune }

// rclass is a set of code points; neg inverts it.
type rclass struct {
	rs  []rrange // sorted, merged
	neg bool
}

func (c *rclass) add(lo, hi rune) { c.rs = append(c.rs, rrange{lo, hi}) }

func (c *rclass) addAll(rs []rrange) { c.rs = append(c.rs, rs...) }

func (c *rclass) norm() {
	c.rs = mergeRanges(c.rs)
}

func mergeRanges(rs []rrange) []rrange {
	sort.Slice(rs, func(i, j int) bool { return rs[i].lo < rs[j].lo })
	var out []rrange
	for _, r := range rs {
		if n := len(out); n > 0 && r.lo <= out[n-1].hi+1 {
			if r.hi > out[n-1].hi {
				out[n-1].hi = r.hi
			}
			continue
		}
		out = append(out, r)
	}
	return out
}

func (c *rclass) contains(r rune) bool {
	i := sort.Search(len(c.rs), func(i int) bool { return c.rs[i].hi >= r })
	return i < len(c.rs) && c.rs[i].lo <= r
}

func (c *rclass) has(r rune, fold bool) bool {
	found := c.contains(r)
	if !found && fold {
		for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
			if c.contains(f) {
				found = true
				break
			}
		}
	}
	return found != c.neg
}

func complement(rs []rrange) []rrange {
	var out []rrange
	next := rune(0)
	for _, r := range rs {
		if r.lo > next {
			out = append(out, rrange{next, r.lo - 1})
		}
		next = r.hi + 1
	}
	if next <= unicode.MaxRune {
		out = append(out, rrange{next, unicode.MaxRune})
	}
	return out
}

var (
	digitRanges = []rrange{{'0', '9'}}
	wordRanges  = []rrange{{'0', '9'}, {'A', 'Z'}, {'_', '_'}, {'a', 'z'}}
	// spaceRanges: ECMA-262 WhiteSpace and LineTerminator.
	spaceRanges = mergeRanges(append([]rrange{{'\t', '\r'}, {' ', ' '}, {0xA0, 0xA0}, {0xFEFF, 0xFEFF}, {0x2028, 0x2029}},
		tableRanges(unicode.Zs)...))
)

func isLineTerm(r rune) bool { return r == '\n' || r == '\r' || r == 0x2028 || r == 0x2029 }

func isWordRune(r rune) bool { return isAlnum(r) || r == '_' }

// --- parser

type rparser struct {
	src       []rune
	pos       int
	ncap      int
	names     map[string]int
	refs      []*rnode
	backtrack bool // the pattern needs the backtracking matcher
}

var errNothing = errors.New("nothing to repeat")

func (p *rparser) eof() bool { return p.pos >= len(p.src) }

func (p *rparser) peek() rune {
	if p.eof() {
		return -1
	}
	return p.src[p.pos]
}

func (p *rparser) lookingAt(s string) bool {
	rs := []rune(s)
	if p.pos+len(rs) > len(p.src) {
		return false
	}
	for i, r := range rs {
		if p.src[p.pos+i] != r {
			return false
		}
	}
	return true
}

func (p *rparser) parse() (*rnode, error) {
	n, err := p.disjunction(rflags{})
	if err != nil {
		return nil, err
	}
	if !p.eof() {
		return nil, fmt.Errorf("unmatched %q", p.peek())
	}
	for _, r := range p.refs {
		if r.name != "" {
			i, ok := p.names[r.name]
			if !ok {
				return nil, fmt.Errorf("no group named %q", r.name)
			}
			r.cap = i
		} else if r.cap > p.ncap {
			return nil, fmt.Errorf("no group %d", r.cap)
		}
	}
	return n, nil
}

func (p *rparser) disjunction(fl rflags) (*rnode, error) {
	var alts []*rnode
	for {
		a, err := p.alternative(fl)
		if err != nil {
			return nil, err
		}
		alts = append(alts, a)
		if p.peek() != '|' {
			break
		}
		p.pos++
	}
	if len(alts) == 1 {
		return alts[0], nil
	}
	return &rnode{kind: rAlt, subs: alts}, nil
}

func (p *rparser) alternative(fl rflags) (*rnode, error) {
	cat := &rnode{kind: rCat}
	for !p.eof() && p.peek() != '|' && p.peek() != ')' {
		t, err := p.term(fl)
		if err != nil {
			return nil, err
		}
		cat.subs = append(cat.subs, t)
	}
	return cat, nil
}

func (p *rparser) term(fl rflags) (*rnode, error) {
	c := p.peek()
	capBefore := p.ncap
	var atom *rnode
	switch c {
	case '^', '$':
		p.pos++
		if fl.m {
			p.backtrack = true
		}
		k := rBegin
		if c == '$' {
			k = rEnd
		}
		return p.noQuantifier(&rnode{kind: k, fl: fl})
	case '\\':
		if p.lookingAt(`\b`) || p.lookingAt(`\B`) {
			p.pos += 2
			k := rWordB
			if c := p.src[p.pos-1]; c == 'B' {
				k = rNotWordB
			}
			return p.noQuantifier(&rnode{kind: k, fl: fl})
		}
		p.pos++
		a, err := p.atomEscape(fl)
		if err != nil {
			return nil, err
		}
		atom = a
	case '(':
		switch {
		case p.lookingAt("(?=") || p.lookingAt("(?!") || p.lookingAt("(?<=") || p.lookingAt("(?<!"):
			behind := p.src[p.pos+2] == '<'
			neg := p.src[p.pos+2] == '!' || behind && p.src[p.pos+3] == '!'
			p.pos += 3
			if behind {
				p.pos++
			}
			sub, err := p.group(fl)
			if err != nil {
				return nil, err
			}
			p.backtrack = true
			return p.noQuantifier(&rnode{kind: rLook, behind: behind, neg: neg, subs: []*rnode{sub}})
		case p.lookingAt("(?:"):
			p.pos += 3
			sub, err := p.group(fl)
			if err != nil {
				return nil, err
			}
			atom = &rnode{kind: rGroup, subs: []*rnode{sub}}
		case p.lookingAt("(?<"):
			p.pos += 3
			name, err := p.groupName()
			if err != nil {
				return nil, err
			}
			if _, dup := p.names[name]; dup {
				return nil, fmt.Errorf("group name %q used twice", name)
			}
			p.ncap++
			idx := p.ncap
			p.names[name] = idx
			sub, err := p.group(fl)
			if err != nil {
				return nil, err
			}
			atom = &rnode{kind: rGroup, cap: idx, subs: []*rnode{sub}}
		case p.lookingAt("(?"):
			p.pos += 2
			nfl, err := p.modifiers(fl)
			if err != nil {
				return nil, err
			}
			sub, err := p.group(nfl)
			if err != nil {
				return nil, err
			}
			atom = &rnode{kind: rGroup, subs: []*rnode{sub}}
		default:
			p.pos++
			p.ncap++
			idx := p.ncap
			sub, err := p.group(fl)
			if err != nil {
				return nil, err
			}
			atom = &rnode{kind: rGroup, cap: idx, subs: []*rnode{sub}}
		}
	case '.':
		p.pos++
		atom = &rnode{kind: rDot, fl: fl}
	case '[':
		p.pos++
		cls, err := p.class()
		if err != nil {
			return nil, err
		}
		atom = &rnode{kind: rClass, cls: cls, fl: fl}
	case '*', '+', '?', '{':
		return nil, errNothing
	case ']', '}':
		return nil, fmt.Errorf("lone %q", c)
	default:
		p.pos++
		atom = &rnode{kind: rLit, r: c, fl: fl}
	}
	return p.quantifier(atom, capBefore)
}

// noQuantifier refuses a quantifier after an assertion (u flag).
func (p *rparser) noQuantifier(n *rnode) (*rnode, error) {
	switch p.peek() {
	case '*', '+', '?', '{':
		return nil, errNothing
	}
	return n, nil
}

func (p *rparser) group(fl rflags) (*rnode, error) {
	sub, err := p.disjunction(fl)
	if err != nil {
		return nil, err
	}
	if p.peek() != ')' {
		return nil, errors.New("missing )")
	}
	p.pos++
	return sub, nil
}

// modifiers reads "ims-ims:" after "(?" (pattern modifiers).
func (p *rparser) modifiers(fl rflags) (rflags, error) {
	seen := map[rune]bool{}
	on := true
	any := false
	for !p.eof() {
		c := p.peek()
		p.pos++
		switch c {
		case 'i', 'm', 's':
			if seen[c] {
				return fl, fmt.Errorf("flag %c twice", c)
			}
			seen[c] = true
			any = true
			switch c {
			case 'i':
				fl.i = on
			case 'm':
				fl.m = on
			case 's':
				fl.s = on
			}
		case '-':
			if !on {
				return fl, errors.New("invalid group")
			}
			on = false
		case ':':
			if !any {
				return fl, errors.New("invalid group")
			}
			return fl, nil
		default:
			return fl, errors.New("invalid group")
		}
	}
	return fl, errors.New("invalid group")
}

func (p *rparser) groupName() (string, error) {
	var b strings.Builder
	for !p.eof() {
		c := p.peek()
		p.pos++
		if c == '>' {
			if b.Len() == 0 {
				return "", errors.New("empty group name")
			}
			return b.String(), nil
		}
		first := b.Len() == 0
		if !(c == '$' || c == '_' || unicode.IsLetter(c) || unicode.Is(unicode.Nl, c) ||
			!first && (unicode.In(c, unicode.Mn, unicode.Mc, unicode.Nd, unicode.Pc) || c == 0x200C || c == 0x200D)) {
			return "", fmt.Errorf("invalid group name character %q", c)
		}
		b.WriteRune(c)
	}
	return "", errors.New("unterminated group name")
}

func (p *rparser) quantifier(atom *rnode, capBefore int) (*rnode, error) {
	min, max := 0, 0
	switch p.peek() {
	case '*':
		p.pos++
		min, max = 0, -1
	case '+':
		p.pos++
		min, max = 1, -1
	case '?':
		p.pos++
		min, max = 0, 1
	case '{':
		p.pos++
		var ok bool
		if min, ok = p.number(); !ok {
			return nil, errors.New("incomplete quantifier")
		}
		max = min
		if p.peek() == ',' {
			p.pos++
			max = -1
			if p.peek() != '}' {
				if max, ok = p.number(); !ok {
					return nil, errors.New("incomplete quantifier")
				}
				if max < min {
					return nil, errors.New("numbers out of order in quantifier")
				}
			}
		}
		if p.peek() != '}' {
			return nil, errors.New("incomplete quantifier")
		}
		p.pos++
	default:
		return atom, nil
	}
	greedy := true
	if p.peek() == '?' {
		p.pos++
		greedy = false
	}
	switch p.peek() {
	case '*', '+', '?', '{':
		return nil, errNothing
	}
	if min > 1000 || max > 1000 {
		p.backtrack = true
	}
	return &rnode{kind: rRepeat, subs: []*rnode{atom}, min: min, max: max, greedy: greedy, capLo: capBefore, capHi: p.ncap}, nil
}

// number reads decimal digits, saturating at a large value.
func (p *rparser) number() (int, bool) {
	n, digits := 0, 0
	for !p.eof() && p.peek() >= '0' && p.peek() <= '9' {
		if n < 1<<30 {
			n = n*10 + int(p.peek()-'0')
		}
		p.pos++
		digits++
	}
	return n, digits > 0
}

// atomEscape reads what follows a backslash outside a class.
func (p *rparser) atomEscape(fl rflags) (*rnode, error) {
	if p.eof() {
		return nil, errors.New("trailing backslash")
	}
	c := p.peek()
	switch {
	case c >= '1' && c <= '9':
		n, _ := p.number()
		ref := &rnode{kind: rBackref, cap: n, fl: fl}
		p.refs = append(p.refs, ref)
		p.backtrack = true
		return ref, nil
	case c == 'k':
		p.pos++
		if p.peek() != '<' {
			return nil, errors.New(`invalid \k`)
		}
		p.pos++
		name, err := p.groupName()
		if err != nil {
			return nil, err
		}
		ref := &rnode{kind: rBackref, name: name, fl: fl}
		p.refs = append(p.refs, ref)
		p.backtrack = true
		return ref, nil
	}
	if cls, ok, err := p.classEscape(); err != nil {
		return nil, err
	} else if ok {
		return &rnode{kind: rClass, cls: cls, fl: fl}, nil
	}
	r, err := p.charEscape(false)
	if err != nil {
		return nil, err
	}
	return &rnode{kind: rLit, r: r, fl: fl}, nil
}

// classEscape reads \d \D \s \S \w \W \p{..} \P{..} (after the
// backslash), reporting false when the escape is not one of them.
func (p *rparser) classEscape() (*rclass, bool, error) {
	c := p.peek()
	cls := &rclass{}
	switch c {
	case 'd', 'D':
		cls.addAll(digitRanges)
	case 's', 'S':
		cls.addAll(spaceRanges)
	case 'w', 'W':
		cls.addAll(wordRanges)
	case 'p', 'P':
		p.pos++
		if p.peek() != '{' {
			return nil, false, errors.New(`\p needs {name}`)
		}
		end := -1
		for i := p.pos; i < len(p.src); i++ {
			if p.src[i] == '}' {
				end = i
				break
			}
		}
		if end < 0 {
			return nil, false, errors.New(`unterminated \p{`)
		}
		rs, err := propertyRanges(string(p.src[p.pos+1 : end]))
		if err != nil {
			return nil, false, err
		}
		cls.addAll(rs)
		p.pos = end
	default:
		return nil, false, nil
	}
	p.pos++
	cls.norm()
	cls.neg = c >= 'A' && c <= 'Z'
	return cls, true, nil
}

// charEscape reads a character escape (after the backslash); inClass
// allows \b (backspace) and \-.
func (p *rparser) charEscape(inClass bool) (rune, error) {
	c := p.peek()
	p.pos++
	switch c {
	case 'f':
		return '\f', nil
	case 'n':
		return '\n', nil
	case 'r':
		return '\r', nil
	case 't':
		return '\t', nil
	case 'v':
		return '\v', nil
	case 'c':
		l := p.peek()
		if l >= 'a' && l <= 'z' || l >= 'A' && l <= 'Z' {
			p.pos++
			return l % 32, nil
		}
		return 0, errors.New(`\c needs a letter`)
	case '0':
		if d := p.peek(); d >= '0' && d <= '9' {
			return 0, errors.New("octal escapes are not allowed")
		}
		return 0, nil
	case 'x':
		return p.hex(2)
	case 'u':
		if p.peek() == '{' {
			p.pos++
			start := p.pos
			for !p.eof() && p.peek() != '}' {
				p.pos++
			}
			if p.eof() || p.pos == start {
				return 0, errors.New(`invalid \u{...}`)
			}
			v, err := strconv.ParseUint(string(p.src[start:p.pos]), 16, 32)
			p.pos++
			if err != nil || v > unicode.MaxRune {
				return 0, errors.New(`invalid \u{...}`)
			}
			return rune(v), nil
		}
		r, err := p.hex(4)
		if err != nil {
			return 0, err
		}
		if r >= 0xD800 && r <= 0xDBFF && p.lookingAt(`\u`) {
			save := p.pos
			p.pos += 2
			if lo, err := p.hex(4); err == nil && lo >= 0xDC00 && lo <= 0xDFFF {
				return (r-0xD800)<<10 + (lo - 0xDC00) + 0x10000, nil
			}
			p.pos = save
		}
		if r >= 0xD800 && r <= 0xDFFF {
			p.backtrack = true // a lone surrogate never matches a Go string
		}
		return r, nil
	case 'b':
		if inClass {
			return '\b', nil
		}
	case '-':
		if inClass {
			return '-', nil
		}
	case -1:
		return 0, errors.New("trailing backslash")
	}
	if strings.ContainsRune(`^$\.*+?()[]{}|/`, c) {
		return c, nil
	}
	return 0, fmt.Errorf(`invalid escape \%c`, c)
}

func (p *rparser) hex(n int) (rune, error) {
	if p.pos+n > len(p.src) {
		return 0, errors.New("incomplete hexadecimal escape")
	}
	v := rune(0)
	for i := 0; i < n; i++ {
		c := p.src[p.pos+i]
		if c >= 0x80 || !isHex(byte(c)) {
			return 0, errors.New("incomplete hexadecimal escape")
		}
		d, _ := strconv.ParseUint(string(c), 16, 8)
		v = v*16 + rune(d)
	}
	p.pos += n
	return v, nil
}

// class reads a character class after "[".
func (p *rparser) class() (*rclass, error) {
	cls := &rclass{}
	if p.peek() == '^' {
		p.pos++
		cls.neg = true
	}
	for {
		if p.eof() {
			return nil, errors.New("missing ]")
		}
		if p.peek() == ']' {
			p.pos++
			break
		}
		lo, set, err := p.classAtom()
		if err != nil {
			return nil, err
		}
		if p.peek() == '-' && p.pos+1 < len(p.src) && p.src[p.pos+1] != ']' {
			p.pos++
			hi, set2, err := p.classAtom()
			if err != nil {
				return nil, err
			}
			if set != nil || set2 != nil {
				return nil, errors.New("a class escape cannot bound a range")
			}
			if hi < lo {
				return nil, errors.New("range out of order in character class")
			}
			cls.add(lo, hi)
			continue
		}
		if set != nil {
			if set.neg {
				cls.addAll(complement(set.rs))
			} else {
				cls.addAll(set.rs)
			}
		} else {
			cls.add(lo, lo)
		}
	}
	cls.norm()
	return cls, nil
}

func (p *rparser) classAtom() (rune, *rclass, error) {
	c := p.peek()
	p.pos++
	if c != '\\' {
		return c, nil, nil
	}
	if p.eof() {
		return 0, nil, errors.New("trailing backslash")
	}
	if set, ok, err := p.classEscape(); err != nil || ok {
		return 0, set, err
	}
	if d := p.peek(); d >= '1' && d <= '9' {
		return 0, nil, errors.New("back reference in a character class")
	}
	r, err := p.charEscape(true)
	return r, nil, err
}

// --- Unicode properties

var categoryAliases = map[string]string{
	"Other": "C", "Control": "Cc", "cntrl": "Cc", "Format": "Cf", "Unassigned": "Cn", "Private_Use": "Co", "Surrogate": "Cs",
	"Letter": "L", "Cased_Letter": "LC", "Lowercase_Letter": "Ll", "Modifier_Letter": "Lm", "Other_Letter": "Lo",
	"Titlecase_Letter": "Lt", "Uppercase_Letter": "Lu", "Mark": "M", "Combining_Mark": "M", "Spacing_Mark": "Mc",
	"Enclosing_Mark": "Me", "Nonspacing_Mark": "Mn", "Number": "N", "Decimal_Number": "Nd", "digit": "Nd",
	"Letter_Number": "Nl", "Other_Number": "No", "Punctuation": "P", "punct": "P", "Connector_Punctuation": "Pc",
	"Dash_Punctuation": "Pd", "Close_Punctuation": "Pe", "Final_Punctuation": "Pf", "Initial_Punctuation": "Pi",
	"Other_Punctuation": "Po", "Open_Punctuation": "Ps", "Symbol": "S", "Currency_Symbol": "Sc", "Modifier_Symbol": "Sk",
	"Math_Symbol": "Sm", "Other_Symbol": "So", "Separator": "Z", "Line_Separator": "Zl", "Paragraph_Separator": "Zp",
	"Space_Separator": "Zs",
}

func tableRanges(t *unicode.RangeTable) []rrange {
	var out []rrange
	for _, r := range t.R16 {
		if r.Stride == 1 {
			out = append(out, rrange{rune(r.Lo), rune(r.Hi)})
			continue
		}
		for c := rune(r.Lo); c <= rune(r.Hi); c += rune(r.Stride) {
			out = append(out, rrange{c, c})
		}
	}
	for _, r := range t.R32 {
		if r.Stride == 1 {
			out = append(out, rrange{rune(r.Lo), rune(r.Hi)})
			continue
		}
		for c := rune(r.Lo); c <= rune(r.Hi); c += rune(r.Stride) {
			out = append(out, rrange{c, c})
		}
	}
	return out
}

func tablesRanges(ts ...*unicode.RangeTable) []rrange {
	var out []rrange
	for _, t := range ts {
		out = append(out, tableRanges(t)...)
	}
	return mergeRanges(out)
}

func minus(a, b []rrange) []rrange {
	c := &rclass{rs: mergeRanges(append([]rrange(nil), b...))}
	var keep []rrange
	rest := complement(c.rs)
	for _, r := range a {
		keep = append(keep, intersect(r, rest)...)
	}
	return mergeRanges(keep)
}

func intersect(r rrange, rs []rrange) []rrange {
	var out []rrange
	for _, x := range rs {
		lo, hi := max(r.lo, x.lo), min(r.hi, x.hi)
		if lo <= hi {
			out = append(out, rrange{lo, hi})
		}
	}
	return out
}

func assignedRanges() []rrange {
	return tablesRanges(unicode.L, unicode.M, unicode.N, unicode.P, unicode.S, unicode.Z, unicode.C)
}

func categoryRanges(name string) ([]rrange, bool) {
	if short, ok := categoryAliases[name]; ok {
		name = short
	}
	switch name {
	case "LC":
		return tablesRanges(unicode.Lu, unicode.Ll, unicode.Lt), true
	case "Cn":
		return complement(assignedRanges()), true
	case "C":
		return mergeRanges(append(tablesRanges(unicode.C), complement(assignedRanges())...)), true
	}
	if t, ok := unicode.Categories[name]; ok && name != "" {
		return tablesRanges(t), true
	}
	return nil, false
}

// binaryProperty resolves the binary properties ECMA-262 lists that Go's
// tables hold or that derive from them.
func binaryProperty(name string) ([]rrange, bool) {
	switch name {
	case "Any":
		return []rrange{{0, unicode.MaxRune}}, true
	case "ASCII":
		return []rrange{{0, 0x7F}}, true
	case "Assigned":
		return assignedRanges(), true
	case "Alphabetic", "Alpha":
		return tablesRanges(unicode.Lu, unicode.Ll, unicode.Lt, unicode.Lm, unicode.Lo, unicode.Nl, unicode.Other_Alphabetic), true
	case "Lowercase", "Lower":
		return tablesRanges(unicode.Ll, unicode.Other_Lowercase), true
	case "Uppercase", "Upper":
		return tablesRanges(unicode.Lu, unicode.Other_Uppercase), true
	case "Math":
		return tablesRanges(unicode.Sm, unicode.Other_Math), true
	case "ID_Start", "IDS":
		return minus(tablesRanges(unicode.L, unicode.Nl, unicode.Other_ID_Start), tablesRanges(unicode.Pattern_Syntax, unicode.Pattern_White_Space)), true
	case "ID_Continue", "IDC":
		return minus(tablesRanges(unicode.L, unicode.Nl, unicode.Other_ID_Start, unicode.Mn, unicode.Mc, unicode.Nd, unicode.Pc, unicode.Other_ID_Continue),
			tablesRanges(unicode.Pattern_Syntax, unicode.Pattern_White_Space)), true
	case "Default_Ignorable_Code_Point", "DI":
		var out []rrange
		for _, r := range tablesRanges(unicode.Other_Default_Ignorable_Code_Point, unicode.Cf, unicode.Variation_Selector) {
			for c := r.lo; c <= r.hi; c++ {
				if defaultIgnorable(c) {
					out = append(out, rrange{c, c})
				}
			}
		}
		return mergeRanges(out), true
	}
	if t, ok := unicode.Properties[name]; ok {
		return tablesRanges(t), true
	}
	return nil, false
}

// propertyRanges resolves the name in \p{name}: a general category, a
// binary property, or name=value for General_Category (gc) and Script
// (sc; Script_Extensions, scx, is read as Script).
func propertyRanges(name string) ([]rrange, error) {
	if k, v, ok := strings.Cut(name, "="); ok {
		switch k {
		case "General_Category", "gc":
			if rs, ok := categoryRanges(v); ok {
				return rs, nil
			}
		case "Script", "sc", "Script_Extensions", "scx":
			if t, ok := unicode.Scripts[v]; ok {
				return tablesRanges(t), nil
			}
		}
		return nil, fmt.Errorf("unknown Unicode property %q", name)
	}
	if rs, ok := categoryRanges(name); ok {
		return rs, nil
	}
	if rs, ok := binaryProperty(name); ok {
		return rs, nil
	}
	return nil, fmt.Errorf("unknown Unicode property %q", name)
}

// --- translation to RE2

func toRE2(n *rnode, b *strings.Builder) {
	switch n.kind {
	case rLit:
		if n.fl.i {
			b.WriteString("(?i:" + regexp.QuoteMeta(string(n.r)) + ")")
		} else {
			b.WriteString(regexp.QuoteMeta(string(n.r)))
		}
	case rClass:
		if n.fl.i {
			b.WriteString("(?i:")
		}
		writeClass(n.cls, b)
		if n.fl.i {
			b.WriteString(")")
		}
	case rDot:
		if n.fl.s {
			b.WriteString(`(?s:.)`)
		} else {
			b.WriteString(`[^\n\r\x{2028}\x{2029}]`)
		}
	case rBegin:
		b.WriteString(`^`)
	case rEnd:
		b.WriteString(`\z`)
	case rWordB:
		b.WriteString(`\b`)
	case rNotWordB:
		b.WriteString(`\B`)
	case rCat:
		b.WriteString("(?:")
		for _, s := range n.subs {
			toRE2(s, b)
		}
		b.WriteString(")")
	case rAlt:
		b.WriteString("(?:")
		for i, s := range n.subs {
			if i > 0 {
				b.WriteString("|")
			}
			toRE2(s, b)
		}
		b.WriteString(")")
	case rGroup:
		if n.cap > 0 {
			b.WriteString("(")
		} else {
			b.WriteString("(?:")
		}
		toRE2(n.subs[0], b)
		b.WriteString(")")
	case rRepeat:
		b.WriteString("(?:")
		toRE2(n.subs[0], b)
		b.WriteString(")")
		switch {
		case n.max < 0:
			fmt.Fprintf(b, "{%d,}", n.min)
		default:
			fmt.Fprintf(b, "{%d,%d}", n.min, n.max)
		}
		if !n.greedy {
			b.WriteString("?")
		}
	}
}

func writeClass(c *rclass, b *strings.Builder) {
	if len(c.rs) == 0 {
		if c.neg {
			b.WriteString(`[\x{0}-\x{10FFFF}]`)
		} else {
			b.WriteString(`[^\x{0}-\x{10FFFF}]`)
		}
		return
	}
	b.WriteString("[")
	if c.neg {
		b.WriteString("^")
	}
	for _, r := range c.rs {
		fmt.Fprintf(b, `\x{%x}`, r.lo)
		if r.hi != r.lo {
			fmt.Fprintf(b, `-\x{%x}`, r.hi)
		}
	}
	b.WriteString("]")
}

// --- backtracking matcher

type bt struct {
	in    []rune
	caps  []int
	steps int
}

func (m *bt) next(i, dir int) (int, rune, bool) {
	if dir > 0 {
		if i >= len(m.in) {
			return 0, 0, false
		}
		return i + 1, m.in[i], true
	}
	if i <= 0 {
		return 0, 0, false
	}
	return i - 1, m.in[i-1], true
}

func foldEq(a, b rune) bool {
	if a == b {
		return true
	}
	for f := unicode.SimpleFold(a); f != a; f = unicode.SimpleFold(f) {
		if f == b {
			return true
		}
	}
	return false
}

func (m *bt) word(i int) bool { return i >= 0 && i < len(m.in) && isWordRune(m.in[i]) }

func (m *bt) match(n *rnode, i, dir int, k func(int) bool) bool {
	m.steps++
	if m.steps > MaxSteps {
		return false
	}
	switch n.kind {
	case rLit:
		j, c, ok := m.next(i, dir)
		if ok && (c == n.r || n.fl.i && foldEq(c, n.r)) {
			return k(j)
		}
		return false
	case rClass:
		j, c, ok := m.next(i, dir)
		if ok && n.cls.has(c, n.fl.i) {
			return k(j)
		}
		return false
	case rDot:
		j, c, ok := m.next(i, dir)
		if ok && (n.fl.s || !isLineTerm(c)) {
			return k(j)
		}
		return false
	case rBegin:
		if i == 0 || n.fl.m && isLineTerm(m.in[i-1]) {
			return k(i)
		}
		return false
	case rEnd:
		if i == len(m.in) || n.fl.m && isLineTerm(m.in[i]) {
			return k(i)
		}
		return false
	case rWordB, rNotWordB:
		if (m.word(i-1) != m.word(i)) == (n.kind == rWordB) {
			return k(i)
		}
		return false
	case rCat:
		return m.cat(n.subs, 0, i, dir, k)
	case rAlt:
		for _, s := range n.subs {
			if m.match(s, i, dir, k) {
				return true
			}
		}
		return false
	case rGroup:
		if n.cap == 0 {
			return m.match(n.subs[0], i, dir, k)
		}
		c := n.cap
		o0, o1 := m.caps[2*c], m.caps[2*c+1]
		ok := m.match(n.subs[0], i, dir, func(j int) bool {
			s0, s1 := m.caps[2*c], m.caps[2*c+1]
			if dir > 0 {
				m.caps[2*c], m.caps[2*c+1] = i, j
			} else {
				m.caps[2*c], m.caps[2*c+1] = j, i
			}
			if k(j) {
				return true
			}
			m.caps[2*c], m.caps[2*c+1] = s0, s1
			return false
		})
		if !ok {
			m.caps[2*c], m.caps[2*c+1] = o0, o1
		}
		return ok
	case rRepeat:
		return m.rep(n, 0, i, dir, k)
	case rLook:
		saved := append([]int(nil), m.caps...)
		ldir := 1
		if n.behind {
			ldir = -1
		}
		found := m.match(n.subs[0], i, ldir, func(int) bool { return true })
		if n.neg || !found {
			copy(m.caps, saved)
			if found == n.neg {
				return false
			}
			return k(i)
		}
		if k(i) {
			return true
		}
		copy(m.caps, saved)
		return false
	case rBackref:
		s0, s1 := m.caps[2*n.cap], m.caps[2*n.cap+1]
		if s0 < 0 || s1 < 0 {
			return k(i)
		}
		l := s1 - s0
		start := i
		if dir < 0 {
			start = i - l
		}
		if start < 0 || start+l > len(m.in) {
			return false
		}
		for x := 0; x < l; x++ {
			a, b := m.in[s0+x], m.in[start+x]
			if a != b && !(n.fl.i && foldEq(a, b)) {
				return false
			}
		}
		if dir > 0 {
			return k(i + l)
		}
		return k(i - l)
	}
	return false
}

func (m *bt) cat(subs []*rnode, idx, i, dir int, k func(int) bool) bool {
	if idx == len(subs) {
		return k(i)
	}
	n := subs[idx]
	if dir < 0 {
		n = subs[len(subs)-1-idx]
	}
	return m.match(n, i, dir, func(j int) bool { return m.cat(subs, idx+1, j, dir, k) })
}

func (m *bt) rep(n *rnode, count, i, dir int, k func(int) bool) bool {
	more := func() bool {
		if n.max >= 0 && count >= n.max {
			return false
		}
		lo, hi := 2*(n.capLo+1), 2*(n.capHi+1)
		saved := append([]int(nil), m.caps[lo:hi]...)
		for x := lo; x < hi; x++ {
			m.caps[x] = -1
		}
		ok := m.match(n.subs[0], i, dir, func(j int) bool {
			if j == i && count >= n.min {
				return false
			}
			return m.rep(n, count+1, j, dir, k)
		})
		if !ok {
			copy(m.caps[lo:hi], saved)
		}
		return ok
	}
	if count < n.min {
		return more()
	}
	if n.greedy {
		return more() || k(i)
	}
	return k(i) || more()
}
