// Package content holds Campaign Content: one per Channel (ADR-0008). It
// parses the Content call's tool input, including partial input while it
// streams, validates Content (the single validator for Claude's output and the
// Marketer's edits, ADR-0002) and renders the fixed email HTML template.
package content

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

// Channels lists the Channels in display order.
var Channels = []string{"sms", "email"}

// SMS is the Content of the SMS Channel.
type SMS struct {
	Body string `json:"body"`
}

// Email is the Content of the email Channel. Body is plain paragraphs
// separated by blank lines; Claude never writes HTML.
type Email struct {
	Subject   string `json:"subject"`
	Preheader string `json:"preheader"`
	Body      string `json:"body"`
	CTAText   string `json:"cta_text,omitempty"`
}

// Proposal is the Content call's tool input: the Channels Claude inferred from
// the Brief, and Content for each of them.
type Proposal struct {
	Channels []string `json:"channels"`
	SMS      *SMS     `json:"sms,omitempty"`
	Email    *Email   `json:"email,omitempty"`
}

// ParseProposal decodes a finished Content call. Channels are normalized:
// unknown names are dropped, and if Claude named none, the Channels it wrote
// Content for are used.
func ParseProposal(raw []byte) (Proposal, error) {
	var p Proposal
	if err := json.Unmarshal(raw, &p); err != nil {
		return Proposal{}, fmt.Errorf("Content is not valid JSON: %w", err)
	}
	var chans []string
	for _, ch := range Channels {
		if slices.Contains(p.Channels, ch) {
			chans = append(chans, ch)
		}
	}
	if len(chans) == 0 {
		if p.SMS != nil {
			chans = append(chans, "sms")
		}
		if p.Email != nil {
			chans = append(chans, "email")
		}
	}
	if len(chans) == 0 {
		return Proposal{}, errors.New("Content names no Channel")
	}
	p.Channels = chans
	// Content for a Channel Claude did not choose is dropped.
	if !slices.Contains(chans, "sms") {
		p.SMS = nil
	}
	if !slices.Contains(chans, "email") {
		p.Email = nil
	}
	return p, nil
}

// ParsePartial decodes as much of a streaming Content call's input as has
// arrived. ok is false while nothing usable has arrived yet.
func ParsePartial(partial string) (p Proposal, ok bool) {
	completed, ok := completeJSON(partial)
	if !ok {
		return Proposal{}, false
	}
	if err := json.Unmarshal([]byte(completed), &p); err != nil {
		return Proposal{}, false
	}
	return p, true
}

// completeJSON turns a prefix of a JSON document into a valid document by
// closing the open string and containers, dropping the trailing token when it
// can't be completed (a dangling key, a half-written literal).
func completeJSON(s string) (string, bool) {
	for end := len(s); end > 0; end-- {
		if !utf8.ValidString(s[:end]) {
			continue
		}
		if c, ok := closeJSON(s[:end]); ok && json.Valid([]byte(c)) {
			return c, true
		}
	}
	return "", false
}

func closeJSON(s string) (string, bool) {
	var stack []byte
	inString, escape := false, false
	escapeStart := -1
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inString {
			switch {
			case escape:
				escape = false
			case c == '\\':
				escape, escapeStart = true, i
			case c == '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{':
			stack = append(stack, '}')
		case '[':
			stack = append(stack, ']')
		case '}', ']':
			if len(stack) == 0 || stack[len(stack)-1] != c {
				return "", false
			}
			stack = stack[:len(stack)-1]
		}
	}
	var b strings.Builder
	if inString {
		// Drop a dangling backslash; a half-written \u escape fails json.Valid
		// and is backed off by the caller.
		if escape {
			s = s[:escapeStart]
		}
		b.WriteString(s)
		b.WriteByte('"')
	} else {
		b.WriteString(s)
	}
	for i := len(stack) - 1; i >= 0; i-- {
		b.WriteByte(stack[i])
	}
	return b.String(), true
}

// Issue is one validation failure, on one field of a Channel's Content.
type Issue struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Validation is the result of validating one Channel's Content.
type Validation struct {
	Valid  bool    `json:"valid"`
	Issues []Issue `json:"issues"`
}

func result(issues []Issue) Validation {
	if issues == nil {
		issues = []Issue{}
	}
	return Validation{Valid: len(issues) == 0, Issues: issues}
}

// Email length limits, in characters.
const (
	MaxSubject   = 80
	MaxPreheader = 120
	MaxEmailBody = 1500
)

// ValidateSMS checks SMS Content. Encoding-aware length limits are added with
// the GSM-7 rules (ADR-0002).
func ValidateSMS(s *SMS) Validation {
	if s == nil {
		return result([]Issue{{Field: "body", Message: "SMS içeriği eksik."}})
	}
	var issues []Issue
	issues = append(issues, required("body", "SMS metni", s.Body)...)
	issues = append(issues, placeholders("body", s.Body)...)
	return result(issues)
}

// ValidateEmail checks email Content against the field limits and placeholders.
func ValidateEmail(e *Email) Validation {
	if e == nil {
		return result([]Issue{{Field: "subject", Message: "E-posta içeriği eksik."}})
	}
	var issues []Issue
	for _, f := range []struct {
		field, name, value string
		max                int
	}{
		{"subject", "Konu", e.Subject, MaxSubject},
		{"preheader", "Ön izleme metni", e.Preheader, MaxPreheader},
		{"body", "Gövde", e.Body, MaxEmailBody},
	} {
		issues = append(issues, required(f.field, f.name, f.value)...)
		if n := utf8.RuneCountInString(f.value); n > f.max {
			issues = append(issues, Issue{Field: f.field, Message: fmt.Sprintf("%s en fazla %d karakter olabilir (şu an %d).", f.name, f.max, n)})
		}
		issues = append(issues, placeholders(f.field, f.value)...)
	}
	issues = append(issues, placeholders("cta_text", e.CTAText)...)
	return result(issues)
}

// Validate checks the Content of every Channel in p.Channels.
func Validate(p Proposal) map[string]Validation {
	out := map[string]Validation{}
	for _, ch := range p.Channels {
		switch ch {
		case "sms":
			out[ch] = ValidateSMS(p.SMS)
		case "email":
			out[ch] = ValidateEmail(p.Email)
		}
	}
	return out
}

func required(field, name, value string) []Issue {
	if strings.TrimSpace(value) == "" {
		return []Issue{{Field: field, Message: name + " boş olamaz."}}
	}
	return nil
}

// Placeholders lists the placeholders Content may use.
var Placeholders = []string{"first_name", "city"}

var placeholderToken = regexp.MustCompile(`\{\{[^{}]*\}\}`)

// placeholders reports every {{…}} other than {{first_name}} and {{city}}, and
// stray braces that would reach a Recipient as a raw "{{".
func placeholders(field, value string) []Issue {
	var issues []Issue
	for _, tok := range placeholderToken.FindAllString(value, -1) {
		if !slices.Contains(Placeholders, tok[2:len(tok)-2]) {
			issues = append(issues, Issue{Field: field, Message: fmt.Sprintf("Bilinmeyen yer tutucu %s; yalnızca {{first_name}} ve {{city}} kullanılabilir.", tok)})
		}
	}
	rest := placeholderToken.ReplaceAllString(value, "")
	if strings.Contains(rest, "{{") || strings.Contains(rest, "}}") {
		issues = append(issues, Issue{Field: field, Message: "Kapanmamış yer tutucu; yalnızca {{first_name}} ve {{city}} kullanılabilir."})
	}
	return issues
}
