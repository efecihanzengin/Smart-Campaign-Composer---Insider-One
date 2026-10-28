// Package segment validates Segment Rules proposed by Claude, compiles them to
// parameterized SQL, renders them as Turkish labels and resolves them to
// per-Channel Audiences.
//
// A Segment Rule is a JSON AST, never SQL (ADR-0003). This slice supports:
//
//	{"all": [node, …]}                                   every child matches
//	{"field": "city", "op": "eq", "value": "İstanbul"}   profile condition
//	{"event": "cart_add", "within_days": 30, "has": true} Event condition
//
// Consent is not part of the language (ADR-0004); it is applied when counting.
package segment

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode/utf8"
)

const (
	maxDepth      = 3
	maxWithinDays = 365
	maxValueLen   = 200
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

// CityEq matches Recipients whose city is exactly Value.
type CityEq struct{ Value string }

// EventCond matches Recipients who had (Has) or did not have (!Has) an Event of
// Type in the last WithinDays days.
type EventCond struct {
	Type       string
	WithinDays int
	Has        bool
}

func (All) node()       {}
func (CityEq) node()    {}
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
		if err := onlyKeys(obj, path, "all"); err != nil {
			return nil, err
		}
		var children []json.RawMessage
		if err := decodeStrict(obj["all"], &children); err != nil {
			return nil, fmt.Errorf("%s.all: must be an array", path)
		}
		if len(children) == 0 {
			return nil, fmt.Errorf("%s.all: must not be empty", path)
		}
		all := All{}
		for i, c := range children {
			n, err := parseNode(c, fmt.Sprintf("%s.all[%d]", path, i), depth+1)
			if err != nil {
				return nil, err
			}
			all.Children = append(all.Children, n)
		}
		return all, nil

	case has(obj, "field"):
		if err := onlyKeys(obj, path, "field", "op", "value"); err != nil {
			return nil, err
		}
		var field, op string
		if err := decodeStrict(obj["field"], &field); err != nil || field != "city" {
			return nil, fmt.Errorf("%s.field: unknown field; supported: city", path)
		}
		if err := decodeStrict(obj["op"], &op); err != nil || op != "eq" {
			return nil, fmt.Errorf("%s.op: unknown operator for city; supported: eq", path)
		}
		var value string
		if err := decodeStrict(obj["value"], &value); err != nil {
			return nil, fmt.Errorf("%s.value: must be a string", path)
		}
		if err := checkText(value); err != nil {
			return nil, fmt.Errorf("%s.value: %w", path, err)
		}
		return CityEq{Value: value}, nil

	case has(obj, "event"):
		if err := onlyKeys(obj, path, "event", "within_days", "has"); err != nil {
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
		return c, nil
	}
	return nil, fmt.Errorf("%s: unknown node; expected all, a profile condition or an Event condition", path)
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

func (n CityEq) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]string{"field": "city", "op": "eq", "value": n.Value})
}

func (n EventCond) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"event": n.Type, "within_days": n.WithinDays, "has": n.Has})
}
