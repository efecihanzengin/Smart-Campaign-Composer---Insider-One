// Package segment validates Segment Rules proposed by Claude, compiles them to
// parameterized SQL, renders them as Turkish labels and resolves them to
// per-Channel Audiences.
//
// A Segment Rule is a JSON AST, never SQL (ADR-0003), at most 3 levels deep:
//
//	{"all": [node, …]}                                   every child matches
//	{"any": [node, …]}                                   some child matches
//	{"not": node}                                        the child does not match
//	{"field": "city", "op": "eq", "value": "İstanbul"}   profile condition over
//	                                                     city, language or created_at
//	{"event": "cart_add", "within_days": 30, "has": true} Event condition, optionally
//	                                                     with min_count and min_amount
//
// Consent is not part of the language (ADR-0004); it is applied when counting.
package segment

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"time"
	_ "time/tzdata"
	"unicode/utf8"
)

const (
	maxDepth      = 3
	maxWithinDays = 365
	maxValueLen   = 200
	maxListLen    = 50
	maxMinCount   = 1000
)

// Proposal is a validated Segment call result.
type Proposal struct {
	Rule                Node
	UnsupportedCriteria []string
}

// Node is one node of a validated Segment Rule.
type Node interface{ node() }

// All matches Recipients that match every child.
type All struct{ Children []Node }

// Any matches Recipients that match at least one child.
type Any struct{ Children []Node }

// Not matches Recipients that do not match Child.
type Not struct{ Child Node }

// ProfileCond matches Recipients by one whitelisted profile field. Values
// holds the one value of eq, or the list of in.
type ProfileCond struct {
	Field, Op string
	Values    []string
}

// EventCond matches Recipients who had (Has) or did not have (!Has) Events of
// Type in the last WithinDays days: at least MinCount of them (when set), with
// amounts totalling at least MinAmount (when set; purchases only).
type EventCond struct {
	Type       string
	WithinDays int
	Has        bool
	MinCount   int    // 0 when not set
	MinAmount  string // decimal TRY amount, "" when not set
}

func (All) node()         {}
func (Any) node()         {}
func (Not) node()         {}
func (ProfileCond) node() {}

// value is the condition's value as written: one string, or the list of in.
func (n ProfileCond) value() any {
	if profileFields[n.Field][n.Op].many {
		return n.Values
	}
	return n.Values[0]
}
func (EventCond) node() {}

// ParseProposal validates the raw Segment call output.
func ParseProposal(raw json.RawMessage) (Proposal, error) {
	var top map[string]json.RawMessage
	if err := decodeStrict(raw, &top); err != nil {
		return Proposal{}, fmt.Errorf("proposal: must be an object: %w", err)
	}
	if err := onlyKeys(top, "proposal", "rule", "unsupported_criteria"); err != nil {
		return Proposal{}, err
	}
	ruleRaw, ok := top["rule"]
	if !ok {
		return Proposal{}, fmt.Errorf("proposal: missing rule")
	}
	rule, err := parseNode(ruleRaw, "rule", 1)
	if err != nil {
		return Proposal{}, err
	}
	unsupported := []string{}
	if u, ok := top["unsupported_criteria"]; ok && !isNull(u) {
		if err := decodeStrict(u, &unsupported); err != nil {
			return Proposal{}, fmt.Errorf("unsupported_criteria: must be an array of strings")
		}
	}
	return Proposal{Rule: rule, UnsupportedCriteria: unsupported}, nil
}

func parseNode(raw json.RawMessage, path string, depth int) (Node, error) {
	if depth > maxDepth {
		return nil, fmt.Errorf("%s: rule is deeper than %d levels", path, maxDepth)
	}
	var obj map[string]json.RawMessage
	if err := decodeStrict(raw, &obj); err != nil || obj == nil {
		return nil, fmt.Errorf("%s: must be an object", path)
	}
	switch {
	case has(obj, "all"):
		children, err := parseChildren(obj, path, "all", depth)
		return All{Children: children}, err

	case has(obj, "any"):
		children, err := parseChildren(obj, path, "any", depth)
		return Any{Children: children}, err

	case has(obj, "not"):
		if err := onlyKeys(obj, path, "not"); err != nil {
			return nil, err
		}
		child, err := parseNode(obj["not"], path+".not", depth+1)
		if err != nil {
			return nil, err
		}
		return Not{Child: child}, nil

	case has(obj, "field"):
		if err := onlyKeys(obj, path, "field", "op", "value"); err != nil {
			return nil, err
		}
		var c ProfileCond
		if err := decodeStrict(obj["field"], &c.Field); err != nil || profileFields[c.Field] == nil {
			return nil, fmt.Errorf("%s.field: unknown field; supported: %s", path, strings.Join(slices.Sorted(maps.Keys(profileFields)), ", "))
		}
		ops := profileFields[c.Field]
		err := decodeStrict(obj["op"], &c.Op)
		spec, ok := ops[c.Op]
		if err != nil || !ok {
			return nil, fmt.Errorf("%s.op: unknown operator for %s; supported: %s", path, c.Field, strings.Join(slices.Sorted(maps.Keys(ops)), ", "))
		}
		values, err := spec.parse(obj["value"])
		if err != nil {
			return nil, fmt.Errorf("%s.value: %w", path, err)
		}
		c.Values = values
		return c, nil

	case has(obj, "event"):
		if err := onlyKeys(obj, path, "event", "within_days", "has", "min_count", "min_amount"); err != nil {
			return nil, err
		}
		var c EventCond
		if err := decodeStrict(obj["event"], &c.Type); err != nil || !knownEvent(c.Type) {
			return nil, fmt.Errorf("%s.event: unknown Event type; supported: %s", path, strings.Join(slices.Sorted(maps.Keys(eventVerbs)), ", "))
		}
		if err := decodeStrict(obj["within_days"], &c.WithinDays); err != nil || c.WithinDays < 1 || c.WithinDays > maxWithinDays {
			return nil, fmt.Errorf("%s.within_days: must be an integer from 1 to %d", path, maxWithinDays)
		}
		if err := decodeStrict(obj["has"], &c.Has); err != nil || isNull(obj["has"]) {
			return nil, fmt.Errorf("%s.has: must be true or false", path)
		}
		if raw, ok := obj["min_count"]; ok {
			if err := decodeStrict(raw, &c.MinCount); err != nil || c.MinCount < 1 || c.MinCount > maxMinCount {
				return nil, fmt.Errorf("%s.min_count: must be an integer from 1 to %d", path, maxMinCount)
			}
		}
		if raw, ok := obj["min_amount"]; ok {
			if c.Type != "purchase" {
				return nil, fmt.Errorf("%s.min_amount: only purchase Events have an amount", path)
			}
			amount, err := parseAmount(raw)
			if err != nil {
				return nil, fmt.Errorf("%s.min_amount: %w", path, err)
			}
			c.MinAmount = amount
		}
		return c, nil
	}
	return nil, fmt.Errorf("%s: unknown node; expected all, any, not, a profile condition or an Event condition", path)
}

// parseChildren parses the non-empty child list of a logical node.
func parseChildren(obj map[string]json.RawMessage, path, key string, depth int) ([]Node, error) {
	if err := onlyKeys(obj, path, key); err != nil {
		return nil, err
	}
	var raws []json.RawMessage
	if err := decodeStrict(obj[key], &raws); err != nil {
		return nil, fmt.Errorf("%s.%s: must be an array", path, key)
	}
	if len(raws) == 0 {
		return nil, fmt.Errorf("%s.%s: must not be empty", path, key)
	}
	children := make([]Node, len(raws))
	for i, raw := range raws {
		n, err := parseNode(raw, fmt.Sprintf("%s.%s[%d]", path, key, i), depth+1)
		if err != nil {
			return nil, err
		}
		children[i] = n
	}
	return children, nil
}

// valueSpec is the value a profile operator takes: one string or a list of
// them, each passing check.
type valueSpec struct {
	many  bool
	check func(string) error
}

// profileFields is the whitelist of profile fields and their typed operators.
var profileFields = map[string]map[string]valueSpec{
	"city":       {"eq": {false, checkText}, "in": {true, checkText}},
	"language":   {"eq": {false, checkLanguage}, "in": {true, checkLanguage}},
	"created_at": {"before": {false, checkDay}, "after": {false, checkDay}},
}

func (s valueSpec) parse(raw json.RawMessage) ([]string, error) {
	var values []string
	if !s.many {
		var v string
		if err := decodeStrict(raw, &v); err != nil {
			return nil, fmt.Errorf("must be a string")
		}
		values = []string{v}
	} else {
		if err := decodeStrict(raw, &values); err != nil || values == nil {
			return nil, fmt.Errorf("must be an array of strings")
		}
		if len(values) == 0 || len(values) > maxListLen {
			return nil, fmt.Errorf("must have 1 to %d values", maxListLen)
		}
	}
	for _, v := range values {
		if err := s.check(v); err != nil {
			return nil, err
		}
	}
	return values, nil
}

var languageCode = regexp.MustCompile(`^[a-z]{2}$`)

func checkLanguage(s string) error {
	if !languageCode.MatchString(s) {
		return fmt.Errorf("must be a two-letter lower-case language code such as tr")
	}
	return nil
}

// turkeyTime is Türkiye's time zone: days in a rule are calendar days there.
// The zone data is embedded (time/tzdata), so it does not depend on the host.
var turkeyTime = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		panic(err)
	}
	return loc
}()

// day parses a YYYY-MM-DD value as the start of that day in Türkiye.
func day(s string) (time.Time, error) {
	return time.ParseInLocation(time.DateOnly, s, turkeyTime)
}

// mustDay is day for a value that has already been validated.
func mustDay(s string) time.Time {
	d, err := day(s)
	if err != nil {
		panic(fmt.Sprintf("segment: unvalidated day %q", s))
	}
	return d
}

func checkDay(s string) error {
	d, err := day(s)
	if err != nil || d.Year() < 2000 || d.Year() > 2100 {
		return fmt.Errorf("must be a date from 2000-01-01 to 2100-12-31 written as YYYY-MM-DD")
	}
	return nil
}

// amountLiteral is a positive TRY amount with at most two decimals, written
// as a plain JSON number.
var amountLiteral = regexp.MustCompile(`^[0-9]{1,9}(\.[0-9]{1,2})?$`)

func parseAmount(raw json.RawMessage) (string, error) {
	lit := string(bytes.TrimSpace(raw))
	if !amountLiteral.MatchString(lit) || strings.Trim(lit, "0.") == "" {
		return "", fmt.Errorf("must be a positive number with at most two decimals, below 1,000,000,000")
	}
	return lit, nil
}

func knownEvent(t string) bool {
	_, ok := eventVerbs[t]
	return ok
}

func checkText(s string) error {
	switch {
	case s == "":
		return fmt.Errorf("must not be empty")
	case !utf8.ValidString(s), strings.ContainsRune(s, 0):
		return fmt.Errorf("must be valid text")
	case utf8.RuneCountInString(s) > maxValueLen:
		return fmt.Errorf("must be at most %d characters", maxValueLen)
	}
	return nil
}

// decodeStrict decodes JSON without silently accepting a missing value: a
// missing key arrives as nil and fails here.
func decodeStrict(raw json.RawMessage, into any) error {
	if raw == nil {
		return fmt.Errorf("missing")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	return dec.Decode(into)
}

func isNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func has(obj map[string]json.RawMessage, key string) bool {
	_, ok := obj[key]
	return ok
}

func onlyKeys(obj map[string]json.RawMessage, path string, allowed ...string) error {
	for k := range obj {
		if !slices.Contains(allowed, k) {
			return fmt.Errorf("%s: unknown key %q", path, k)
		}
	}
	return nil
}

// The canonical JSON of a validated rule, which is what gets stored and shown.

func (n All) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string][]Node{"all": n.Children})
}

func (n Any) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string][]Node{"any": n.Children})
}

func (n Not) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]Node{"not": n.Child})
}

func (n ProfileCond) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"field": n.Field, "op": n.Op, "value": n.value()})
}

func (n EventCond) MarshalJSON() ([]byte, error) {
	m := map[string]any{"event": n.Type, "within_days": n.WithinDays, "has": n.Has}
	if n.MinCount > 0 {
		m["min_count"] = n.MinCount
	}
	if n.MinAmount != "" {
		m["min_amount"] = json.Number(n.MinAmount)
	}
	return json.Marshal(m)
}
