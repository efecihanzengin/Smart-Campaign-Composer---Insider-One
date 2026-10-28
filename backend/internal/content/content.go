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

// Validation is the result of validating one Channel's Content. Usage holds,
// per limited field, its length after rendering with the longest placeholder
// values; it drives the live counters.
type Validation struct {
	Valid  bool              `json:"valid"`
	Issues []Issue           `json:"issues"`
	Usage  map[string]Length `json:"usage"`
}

func result(issues []Issue, usage map[string]Length) Validation {
	if issues == nil {
		issues = []Issue{}
	}
	if usage == nil {
		usage = map[string]Length{}
	}
	return Validation{Valid: len(issues) == 0, Issues: issues, Usage: usage}
}

// Longest holds, per Channel, the longest placeholder values in that Channel's
// Audience. Lengths are checked after rendering with them, so no Recipient's
// message overflows (ADR-0002).
type Longest struct {
	SMS, Email Sample
}

// Email length limits, in characters.
const (
	MaxSubject   = 80
	MaxPreheader = 120
	MaxEmailBody = 1500
)

// ValidateSMS checks SMS Content: once rendered with the longest placeholder
// values it must fit in one segment of its encoding, 160 septets in GSM-7 or
// 70 characters in UCS-2 (ADR-0002).
func ValidateSMS(s *SMS, longest Sample) Validation {
	if s == nil {
		return result([]Issue{{Field: "body", Message: "SMS içeriği eksik."}}, nil)
	}
	var issues []Issue
	issues = append(issues, required("body", "SMS metni", s.Body)...)
	issues = append(issues, placeholders("body", s.Body)...)
	length := MeasureSMS(RenderSMS(s.Body, longest))
	switch {
	case length.Used <= length.Limit:
	case length.Encoding == GSM7:
		issues = append(issues, Issue{Field: "body", Message: fmt.Sprintf(
			"SMS tek parçaya sığmalı: GSM-7 ile en fazla %d karakter olabilir (şu an %d).", length.Limit, length.Used)})
	default:
		issues = append(issues, Issue{Field: "body", Message: fmt.Sprintf(
			"SMS tek parçaya sığmalı: ş, ğ, ı, İ gibi harfler mesajı UCS-2'ye çevirir ve sınır %d karakter olur (şu an %d). "+
				"Bu harfleri s, g, i, I olarak yazarsanız sınır 160 olur.", length.Limit, length.Used)})
	}
	return result(issues, map[string]Length{"body": length})
}

// ValidateEmail checks email Content against the field limits, once rendered
// with the longest placeholder values, and the placeholders.
func ValidateEmail(e *Email, longest Sample) Validation {
	if e == nil {
		return result([]Issue{{Field: "subject", Message: "E-posta içeriği eksik."}}, nil)
	}
	var issues []Issue
	usage := map[string]Length{}
	for _, f := range []struct {
		field, name, value string
		max                int
	}{
		{"subject", "Konu", e.Subject, MaxSubject},
		{"preheader", "Ön izleme metni", e.Preheader, MaxPreheader},
		{"body", "Gövde", e.Body, MaxEmailBody},
	} {
		issues = append(issues, required(f.field, f.name, f.value)...)
		n := utf8.RuneCountInString(longest.Render(f.value))
		usage[f.field] = Length{Used: n, Limit: f.max}
		if n > f.max {
			issues = append(issues, Issue{Field: f.field, Message: fmt.Sprintf("%s en fazla %d karakter olabilir (şu an %d).", f.name, f.max, n)})
		}
		issues = append(issues, placeholders(f.field, f.value)...)
	}
	issues = append(issues, placeholders("cta_text", e.CTAText)...)
	return result(issues, usage)
}

// Validate checks the Content of every Channel in p.Channels.
func Validate(p Proposal, longest Longest) map[string]Validation {
	out := map[string]Validation{}
	for _, ch := range p.Channels {
		switch ch {
		case "sms":
			out[ch] = ValidateSMS(p.SMS, longest.SMS)
		case "email":
			out[ch] = ValidateEmail(p.Email, longest.Email)
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
