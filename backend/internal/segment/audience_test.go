package segment_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"campaigncomposer/internal/app"
	"campaigncomposer/internal/segment"
	"campaigncomposer/internal/testenv"
)

func seededPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := testenv.NewDatabase(t)
	if err := app.Bootstrap(context.Background(), pool, time.Now()); err != nil {
		t.Fatal(err)
	}
	return pool
}

// TestAudienceMatchesHandWrittenSQL resolves each rule over the seed and
// compares it with the same Audience written out by hand as plain SQL.
func TestAudienceMatchesHandWrittenSQL(t *testing.T) {
	pool := seededPool(t)
	// Seeded Recipients signed up between about 2 years and 90 days ago.
	yearAgo := time.Now().AddDate(-1, 0, 0).Format(time.DateOnly)
	cases := []struct {
		name, rule, oracle string
	}{
		{
			"any of two cities",
			`{"any": [{"field": "city", "op": "eq", "value": "Ankara"}, {"field": "city", "op": "eq", "value": "İzmir"}]}`,
			`r.city IN ('Ankara', 'İzmir')`,
		},
		{
			"city casefolded the Turkish way",
			`{"field": "city", "op": "eq", "value": "istanbul"}`,
			`r.city = 'İstanbul'`,
		},
		{
			"upper-case city with Turkish letters",
			`{"field": "city", "op": "eq", "value": "ESKİŞEHİR"}`,
			`r.city = 'Eskişehir'`,
		},
		{
			"dotless I folds to ı, not i",
			`{"field": "city", "op": "eq", "value": "DİYARBAKIR"}`,
			`r.city = 'Diyarbakır'`,
		},
		{
			"decomposed İ (I + combining dot above)",
			"{\"field\": \"city\", \"op\": \"eq\", \"value\": \"I\u0307zmir\"}",
			`r.city = 'İzmir'`,
		},
		{
			"city in a list",
			`{"field": "city", "op": "in", "value": ["izmir", "BURSA", "Antalya"]}`,
			`r.city IN ('İzmir', 'Bursa', 'Antalya')`,
		},
		{
			"language",
			`{"field": "language", "op": "eq", "value": "en"}`,
			`r.language = 'en'`,
		},
		{
			"language in a list",
			`{"field": "language", "op": "in", "value": ["tr", "de"]}`,
			`r.language = 'tr'`,
		},
		{
			"signed up before a day",
			`{"field": "created_at", "op": "before", "value": "` + yearAgo + `"}`,
			`r.created_at < '` + yearAgo + `'::timestamp AT TIME ZONE 'Europe/Istanbul'`,
		},
		{
			"signed up after a day",
			`{"field": "created_at", "op": "after", "value": "` + yearAgo + `"}`,
			`r.created_at >= ('` + yearAgo + `'::timestamp + interval '1 day') AT TIME ZONE 'Europe/Istanbul'`,
		},
		{
			"at least three cart adds",
			`{"event": "cart_add", "within_days": 30, "has": true, "min_count": 3}`,
			`(SELECT count(*) FROM events e WHERE e.recipient_id = r.id AND e.type = 'cart_add' AND e.occurred_at >= now() - interval '30 days') >= 3`,
		},
		{
			"spent at least 2000 TL in total",
			`{"event": "purchase", "within_days": 60, "has": true, "min_amount": 2000}`,
			`(SELECT sum(e.amount) FROM events e WHERE e.recipient_id = r.id AND e.type = 'purchase' AND e.occurred_at >= now() - interval '60 days') >= 2000`,
		},
		{
			"fewer than two purchases or under 1500.50 TL",
			`{"event": "purchase", "within_days": 90, "has": false, "min_count": 2, "min_amount": 1500.50}`,
			`NOT EXISTS (SELECT 1 FROM events e WHERE e.recipient_id = r.id AND e.type = 'purchase' AND e.occurred_at >= now() - interval '90 days'
			   HAVING count(*) >= 2 AND sum(e.amount) >= 1500.50)`,
		},
		{
			"not a city",
			`{"not": {"field": "city", "op": "eq", "value": "İstanbul"}}`,
			`r.city <> 'İstanbul'`,
		},
		{
			"all with nested any and not",
			`{"all": [
				{"any": [{"field": "city", "op": "eq", "value": "Ankara"}, {"event": "purchase", "within_days": 7, "has": true}]},
				{"not": {"event": "cart_add", "within_days": 30, "has": true}}]}`,
			`(r.city = 'Ankara' OR EXISTS (SELECT 1 FROM events e WHERE e.recipient_id = r.id AND e.type = 'purchase' AND e.occurred_at >= now() - interval '7 days'))
			 AND NOT EXISTS (SELECT 1 FROM events e WHERE e.recipient_id = r.id AND e.type = 'cart_add' AND e.occurred_at >= now() - interval '30 days')`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prop, err := segment.ParseProposal(proposal(tc.rule))
			if err != nil {
				t.Fatalf("rejected: %v", err)
			}
			where, args := segment.Compile(prop.Rule)
			assertOnlyAllowedSQL(t, where, args)
			got, err := segment.CountAudience(context.Background(), pool, prop.Rule)
			if err != nil {
				t.Fatal(err)
			}
			var want segment.Audience
			if err := pool.QueryRow(context.Background(), `SELECT count(*) FILTER (WHERE r.sms_consent), count(*) FILTER (WHERE r.email_consent)
				FROM recipients r WHERE `+tc.oracle).Scan(&want.SMS, &want.Email); err != nil {
				t.Fatalf("oracle: %v", err)
			}
			if want.SMS == 0 || want.Email == 0 {
				t.Fatalf("oracle Audience %+v is empty; pick a rule the seed actually reaches", want)
			}
			if got != want {
				t.Errorf("audience = %+v, want %+v", got, want)
			}
		})
	}
}
