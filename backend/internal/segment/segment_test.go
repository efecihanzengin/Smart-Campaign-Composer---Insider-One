package segment_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"campaigncomposer/internal/app"
	"campaigncomposer/internal/segment"
	"campaigncomposer/internal/testenv"
)

func proposal(rule string) json.RawMessage {
	return json.RawMessage(`{"rule": ` + rule + `, "unsupported_criteria": []}`)
}

func TestInvalidRulesAreRejected(t *testing.T) {
	cases := map[string]string{
		"unknown field email":      `{"field": "email", "op": "eq", "value": "a@b.c"}`,
		"unknown field phone":      `{"field": "phone", "op": "eq", "value": "1"}`,
		"unknown field password":   `{"field": "password", "op": "eq", "value": "x"}`,
		"unknown operator sql":     `{"field": "city", "op": "sql", "value": "true"}`,
		"consent field sms":        `{"field": "sms_consent", "op": "eq", "value": false}`,
		"consent field email":      `{"field": "email_consent", "op": "eq", "value": false}`,
		"qualified column":         `{"field": "recipients.id", "op": "eq", "value": "1"}`,
		"tautology as field":       `{"field": "1=1", "op": "eq", "value": "x"}`,
		"unknown operator like":    `{"field": "city", "op": "like", "value": "%"}`,
		"unknown operator raw":     `{"field": "city", "op": "raw", "value": "true"}`,
		"extra key on condition":   `{"field": "city", "op": "eq", "value": "Ankara", "sql": "true"}`,
		"consent key on event":     `{"event": "purchase", "within_days": 30, "has": true, "sms_consent": true}`,
		"number as city":           `{"field": "city", "op": "eq", "value": 34}`,
		"array as city":            `{"field": "city", "op": "eq", "value": ["Ankara"]}`,
		"empty city":               `{"field": "city", "op": "eq", "value": ""}`,
		"NUL in city":              `{"field": "city", "op": "eq", "value": "Ank\u0000ara"}`,
		"unknown event type":       `{"event": "login", "within_days": 30, "has": true}`,
		"string within_days":       `{"event": "purchase", "within_days": "30", "has": true}`,
		"fractional within_days":   `{"event": "purchase", "within_days": 1.5, "has": true}`,
		"negative within_days":     `{"event": "purchase", "within_days": -1, "has": true}`,
		"huge within_days":         `{"event": "purchase", "within_days": 100000, "has": true}`,
		"missing has":              `{"event": "purchase", "within_days": 30}`,
		"null has":                 `{"event": "purchase", "within_days": 30, "has": null}`,
		"string has":               `{"event": "purchase", "within_days": 30, "has": "false"}`,
		"unknown node kind":        `{"or": [{"field": "city", "op": "eq", "value": "Ankara"}]}`,
		"empty all":                `{"all": []}`,
		"all with a scalar":        `{"all": "true"}`,
		"rule is a string":         `"SELECT * FROM recipients"`,
		"deeper than three levels": `{"all": [{"all": [{"all": [{"field": "city", "op": "eq", "value": "Ankara"}]}]}]}`,
	}
	for name, rule := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := segment.ParseProposal(proposal(rule)); err == nil {
				t.Errorf("accepted %s", rule)
			}
		})
	}
}

func TestProposalWithoutRuleIsRejected(t *testing.T) {
	if _, err := segment.ParseProposal(json.RawMessage(`{"unsupported_criteria": []}`)); err == nil {
		t.Error("accepted a proposal without a rule")
	}
}

// allowedSQL are the only fragments the compiled SQL may consist of, once
// placeholders are removed. They are stripped in order, longest first.
var allowedSQL = []string{
	"NOT EXISTS (SELECT 1 FROM events e WHERE e.recipient_id = r.id AND e.type = ",
	"EXISTS (SELECT 1 FROM events e WHERE e.recipient_id = r.id AND e.type = ",
	" AND e.occurred_at >= now() - make_interval(days => ",
	"r.city = ", " AND ", "(", ")",
}

func TestInjectionAttemptsOnlyEverBecomeBoundParameters(t *testing.T) {
	pool := testenv.NewDatabase(t)
	if err := app.Bootstrap(context.Background(), pool, time.Now()); err != nil {
		t.Fatal(err)
	}
	payloads := []string{
		`İstanbul' OR '1'='1`,
		`'; DROP TABLE recipients; --`,
		`$1`,
		`İstanbul’ OR ‘1’=‘1`, // Unicode quote look-alikes
		`") OR TRUE --`,
		strings.Repeat("İ", 200),
	}
	for _, p := range payloads {
		t.Run(p, func(t *testing.T) {
			value, _ := json.Marshal(p)
			raw := proposal(fmt.Sprintf(`{"all": [{"field": "city", "op": "eq", "value": %s},
				{"event": "purchase", "within_days": 30, "has": false}]}`, value))
			prop, err := segment.ParseProposal(raw)
			if err != nil {
				t.Fatalf("rejected a plain string value: %v", err)
			}

			where, args := segment.Compile(prop.Rule)
			// "$1" is itself a placeholder; the whitelist check below covers it.
			if (p != "$1" && strings.Contains(where, p)) || strings.ContainsAny(where, "'’‘;") || strings.Contains(where, "--") {
				t.Errorf("value leaked into SQL text: %s", where)
			}
			if args[0] != p {
				t.Errorf("first bound parameter = %v, want the payload", args[0])
			}
			rest := where
			for i := len(args); i >= 1; i-- {
				rest = strings.ReplaceAll(rest, fmt.Sprintf("$%d", i), "")
			}
			for _, f := range allowedSQL {
				rest = strings.ReplaceAll(rest, f, "")
			}
			if rest != "" {
				t.Errorf("SQL has fragments outside the whitelist: %q (from %s)", rest, where)
			}

			audience, err := segment.CountAudience(context.Background(), pool, prop.Rule)
			if err != nil {
				t.Fatalf("query failed: %v", err)
			}
			if audience != (segment.Audience{}) {
				t.Errorf("audience = %+v, want empty", audience)
			}
			var recipients int
			if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM recipients`).Scan(&recipients); err != nil || recipients != 5000 {
				t.Errorf("recipients after query = %d (%v), want 5000", recipients, err)
			}
		})
	}
}
