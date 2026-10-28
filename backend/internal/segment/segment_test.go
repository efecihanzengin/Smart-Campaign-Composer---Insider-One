package segment_test

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"campaigncomposer/internal/segment"
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

		"empty any":                     `{"any": []}`,
		"any with a scalar":             `{"any": {"field": "city", "op": "eq", "value": "Ankara"}}`,
		"not with an array":             `{"not": [{"field": "city", "op": "eq", "value": "Ankara"}]}`,
		"not with null":                 `{"not": null}`,
		"two node kinds in one object":  `{"all": [{"field": "city", "op": "eq", "value": "Ankara"}], "not": {"field": "city", "op": "eq", "value": "İzmir"}}`,
		"deeper than three through not": `{"not": {"not": {"not": {"field": "city", "op": "eq", "value": "Ankara"}}}}`,
		"deeper than three through any": `{"any": [{"not": {"all": [{"field": "city", "op": "eq", "value": "Ankara"}]}}]}`,
		"consent field inside any":      `{"any": [{"field": "sms_consent", "op": "eq", "value": true}]}`,
		"consent field inside not":      `{"not": {"field": "email_consent", "op": "eq", "value": false}}`,
		"unknown field first_name":      `{"field": "first_name", "op": "eq", "value": "Ayşe"}`,
		"unknown field id":              `{"field": "id", "op": "eq", "value": "1"}`,
		"field differs only in case":    `{"field": "City", "op": "eq", "value": "Ankara"}`,
		"operator differs only in case": `{"field": "city", "op": "EQ", "value": "Ankara"}`,
		"date operator on city":         `{"field": "city", "op": "before", "value": "Ankara"}`,
		"list operator on created_at":   `{"field": "created_at", "op": "in", "value": ["2025-01-01"]}`,
		"missing op":                    `{"field": "city", "value": "Ankara"}`,
		"missing value":                 `{"field": "city", "op": "eq"}`,
		"null value":                    `{"field": "city", "op": "eq", "value": null}`,
		"array for eq":                  `{"field": "language", "op": "eq", "value": ["tr"]}`,
		"string for in":                 `{"field": "city", "op": "in", "value": "Ankara"}`,
		"empty in":                      `{"field": "city", "op": "in", "value": []}`,
		"null in":                       `{"field": "city", "op": "in", "value": null}`,
		"number inside in":              `{"field": "city", "op": "in", "value": ["Ankara", 6]}`,
		"NUL inside in":                 `{"field": "city", "op": "in", "value": ["Ankara", "\u0000"]}`,
		"empty string inside in":        `{"field": "city", "op": "in", "value": ["Ankara", ""]}`,
		"too many values in in":         `{"field": "city", "op": "in", "value": [` + strings.TrimSuffix(strings.Repeat(`"Ankara",`, 51), ",") + `]}`,
		"too long city":                 `{"field": "city", "op": "eq", "value": "` + strings.Repeat("a", 201) + `"}`,
		"language name, not code":       `{"field": "language", "op": "eq", "value": "Türkçe"}`,
		"upper-case language code":      `{"field": "language", "op": "eq", "value": "TR"}`,
		"language with SQL":             `{"field": "language", "op": "in", "value": ["tr' OR '1'='1"]}`,
		"number as language":            `{"field": "language", "op": "eq", "value": 1}`,
		"date with a time":              `{"field": "created_at", "op": "before", "value": "2025-01-01T00:00:00Z"}`,
		"impossible date":               `{"field": "created_at", "op": "after", "value": "2025-02-30"}`,
		"date out of range":             `{"field": "created_at", "op": "after", "value": "1900-01-01"}`,
		"date as a number":              `{"field": "created_at", "op": "before", "value": 20250101}`,
		"date with SQL":                 `{"field": "created_at", "op": "before", "value": "now()"}`,
		"zero within_days":              `{"event": "purchase", "within_days": 0, "has": true}`,
		"within_days just over a year":  `{"event": "purchase", "within_days": 366, "has": true}`,
		"null within_days":              `{"event": "purchase", "within_days": null, "has": true}`,
		"missing within_days":           `{"event": "purchase", "has": true}`,
		"event type differs in case":    `{"event": "Purchase", "within_days": 30, "has": true}`,
		"number as event type":          `{"event": 1, "within_days": 30, "has": true}`,
		"zero min_count":                `{"event": "purchase", "within_days": 30, "has": true, "min_count": 0}`,
		"negative min_count":            `{"event": "purchase", "within_days": 30, "has": true, "min_count": -2}`,
		"fractional min_count":          `{"event": "purchase", "within_days": 30, "has": true, "min_count": 2.5}`,
		"string min_count":              `{"event": "purchase", "within_days": 30, "has": true, "min_count": "2"}`,
		"huge min_count":                `{"event": "purchase", "within_days": 30, "has": true, "min_count": 1001}`,
		"null min_count":                `{"event": "purchase", "within_days": 30, "has": true, "min_count": null}`,
		"zero min_amount":               `{"event": "purchase", "within_days": 30, "has": true, "min_amount": 0}`,
		"negative min_amount":           `{"event": "purchase", "within_days": 30, "has": true, "min_amount": -100}`,
		"string min_amount":             `{"event": "purchase", "within_days": 30, "has": true, "min_amount": "100"}`,
		"exponent min_amount":           `{"event": "purchase", "within_days": 30, "has": true, "min_amount": 1e3}`,
		"three-decimal min_amount":      `{"event": "purchase", "within_days": 30, "has": true, "min_amount": 10.005}`,
		"huge min_amount":               `{"event": "purchase", "within_days": 30, "has": true, "min_amount": 1000000000}`,
		"null min_amount":               `{"event": "purchase", "within_days": 30, "has": true, "min_amount": null}`,
		"min_amount on cart_add":        `{"event": "cart_add", "within_days": 30, "has": true, "min_amount": 100}`,
	}
	for name, rule := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := segment.ParseProposal(proposal(rule)); err == nil {
				t.Errorf("accepted %s", rule)
			}
		})
	}
}

func TestCanonicalJSONParsesBackToTheSameRule(t *testing.T) {
	rule := `{"all": [
		{"any": [{"field": "city", "op": "in", "value": ["istanbul", "Ankara"]}, {"field": "language", "op": "eq", "value": "en"}]},
		{"not": {"event": "purchase", "within_days": 90, "has": true, "min_count": 2, "min_amount": 1500.50}},
		{"field": "created_at", "op": "before", "value": "2025-06-01"},
		{"event": "cart_add", "within_days": 30, "has": true}]}`
	first, err := segment.ParseProposal(proposal(rule))
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := json.Marshal(first.Rule)
	if err != nil {
		t.Fatal(err)
	}
	second, err := segment.ParseProposal(proposal(string(canonical)))
	if err != nil {
		t.Fatalf("canonical JSON %s rejected: %v", canonical, err)
	}
	if !reflect.DeepEqual(first.Rule, second.Rule) {
		t.Errorf("round trip changed the rule:\n%#v\n%#v", first.Rule, second.Rule)
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
	"tr_fold(r.city) IN (SELECT tr_fold(unnest(", "::text[])))",
	"tr_fold(r.city) = tr_fold(",
	"r.language = ANY (", "::text[])", "r.language = ",
	"r.created_at < ", "r.created_at >= ",
	" HAVING ", "count(*) >= ", "sum(e.amount) >= ", "::numeric",
	"NOT (", " AND ", " OR ", "(", ")",
}

// assertOnlyAllowedSQL fails unless where is made of allowedSQL fragments and
// one placeholder per bound argument.
func assertOnlyAllowedSQL(t *testing.T, where string, args []any) {
	t.Helper()
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
}

func TestInjectionAttemptsOnlyEverBecomeBoundParameters(t *testing.T) {
	pool := seededPool(t)
	var seededEvents int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM events`).Scan(&seededEvents); err != nil {
		t.Fatal(err)
	}
	// What the rule below reaches when no Recipient's city equals the payload.
	var istanbul segment.Audience
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FILTER (WHERE r.sms_consent), count(*) FILTER (WHERE r.email_consent)
		FROM recipients r WHERE r.city = 'İstanbul' AND NOT EXISTS (
			SELECT 1 FROM events e WHERE e.recipient_id = r.id AND e.type = 'purchase' AND e.occurred_at >= now() - interval '30 days'
			HAVING count(*) >= 2 AND sum(e.amount) >= 100)`).Scan(&istanbul.SMS, &istanbul.Email); err != nil || istanbul.SMS == 0 {
		t.Fatalf("oracle: %+v, %v", istanbul, err)
	}
	payloads := []string{
		`İstanbul' OR '1'='1`,
		`'; DROP TABLE recipients; --`,
		`$1`,
		`İstanbul’ OR ‘1’=‘1`, // Unicode quote look-alikes
		`") OR TRUE --`,
		`\'; DELETE FROM events; --`,
		`İstanbul\u0027 OR 1=1`,
		"İstanbul\u02bc OR \u02bc1\u02bc=\u02bc1", // modifier-letter apostrophe
		"İstanbul\uff07 OR \uff071\uff07=\uff071", // full-width apostrophe
		`İstanbul$$ OR TRUE $$`,
		`x)) OR (1=1`,
		strings.Repeat("İ", 200),
		strings.Repeat(`' OR '1'='1`, 18),
	}
	for _, p := range payloads {
		t.Run(p, func(t *testing.T) {
			value, _ := json.Marshal(p)
			// The payload fills every free-text slot of a rule that exercises
			// each node kind; İstanbul keeps the expected Audience non-empty.
			raw := proposal(fmt.Sprintf(`{"all": [
				{"any": [{"field": "city", "op": "eq", "value": %[1]s}, {"field": "city", "op": "in", "value": [%[1]s]},
				         {"field": "city", "op": "eq", "value": "İstanbul"}]},
				{"not": {"event": "purchase", "within_days": 30, "has": true, "min_count": 2, "min_amount": 100}},
				{"field": "language", "op": "in", "value": ["tr", "en"]},
				{"field": "created_at", "op": "after", "value": "2000-01-01"}]}`, value))
			prop, err := segment.ParseProposal(raw)
			if err != nil {
				t.Fatalf("rejected a plain string value: %v", err)
			}

			where, args := segment.Compile(prop.Rule)
			// "$1" is itself a placeholder; the whitelist check below covers it.
			if (p != "$1" && strings.Contains(where, p)) || strings.ContainsAny(where, "'’‘;") || strings.Contains(where, "--") {
				t.Errorf("value leaked into SQL text: %s", where)
			}
			if args[0] != p || !slices.Equal(args[1].([]string), []string{p}) {
				t.Errorf("bound parameters = %q, want the payload first in both city slots", args[:2])
			}
			assertOnlyAllowedSQL(t, where, args)

			audience, err := segment.CountAudience(context.Background(), pool, prop.Rule)
			if err != nil {
				t.Fatalf("query failed: %v", err)
			}
			if audience != istanbul {
				t.Errorf("audience = %+v, want the İstanbul Audience %+v", audience, istanbul)
			}
			var recipients, events int
			if err := pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM recipients), (SELECT count(*) FROM events)`).Scan(&recipients, &events); err != nil ||
				recipients != 5000 || events != seededEvents {
				t.Errorf("rows after query = %d Recipients, %d Events (%v), want 5000 and %d", recipients, events, err, seededEvents)
			}
		})
	}
}
