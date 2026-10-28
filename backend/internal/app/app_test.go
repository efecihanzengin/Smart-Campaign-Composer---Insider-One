package app_test

import (
	"context"
	"testing"
	"time"

	"campaigncomposer/internal/app"
	"campaigncomposer/internal/testenv"

	"github.com/jackc/pgx/v5/pgxpool"
)

// fingerprint hashes every seeded Recipient and Event in a stable order.
func fingerprint(t *testing.T, pool *pgxpool.Pool) (recipients, events string) {
	t.Helper()
	ctx := context.Background()
	err := pool.QueryRow(ctx, `SELECT md5(string_agg(r::text, '|' ORDER BY id)) FROM recipients r`).Scan(&recipients)
	if err != nil {
		t.Fatal(err)
	}
	err = pool.QueryRow(ctx, `SELECT md5(string_agg(
		concat_ws(',', recipient_id, type, occurred_at, amount), '|'
		ORDER BY recipient_id, occurred_at, type, amount)) FROM events`).Scan(&events)
	if err != nil {
		t.Fatal(err)
	}
	return recipients, events
}

func counts(t *testing.T, pool *pgxpool.Pool) (recipients, events int) {
	t.Helper()
	err := pool.QueryRow(context.Background(),
		`SELECT (SELECT count(*) FROM recipients), (SELECT count(*) FROM events)`).Scan(&recipients, &events)
	if err != nil {
		t.Fatal(err)
	}
	return recipients, events
}

// queryRow scans one row and fails the test on error, so a broken query can't
// masquerade as a zero count.
func queryRow(t *testing.T, pool *pgxpool.Pool, sql string, args []any, dest ...any) {
	t.Helper()
	if err := pool.QueryRow(context.Background(), sql, args...).Scan(dest...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

func TestTwoFreshBootstrapsProduceIdenticalData(t *testing.T) {
	now := time.Date(2026, 10, 28, 14, 30, 0, 0, time.UTC)
	a, b := testenv.NewDatabase(t), testenv.NewDatabase(t)
	if err := app.Bootstrap(context.Background(), a, now); err != nil {
		t.Fatal(err)
	}
	// Later the same day: the anchor is the start of the UTC day, so nothing moves.
	if err := app.Bootstrap(context.Background(), b, now.Add(3*time.Hour)); err != nil {
		t.Fatal(err)
	}

	ra, ea := fingerprint(t, a)
	rb, eb := fingerprint(t, b)
	if ra != rb {
		t.Errorf("Recipients differ between fresh runs: %s vs %s", ra, rb)
	}
	if ea != eb {
		t.Errorf("Events differ between fresh runs: %s vs %s", ea, eb)
	}
}

func TestRestartDoesNotReseed(t *testing.T) {
	pool := testenv.NewDatabase(t)
	if err := app.Bootstrap(context.Background(), pool, time.Now()); err != nil {
		t.Fatal(err)
	}
	r1, e1 := counts(t, pool)
	if err := app.Bootstrap(context.Background(), pool, time.Now().Add(48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	r2, e2 := counts(t, pool)
	if r1 != r2 || e1 != e2 {
		t.Fatalf("restart changed data: %d/%d Recipients/Events before, %d/%d after", r1, e1, r2, e2)
	}
}

func TestSeedMatchesTheSpec(t *testing.T) {
	pool := testenv.NewDatabase(t)
	now := time.Now()
	if err := app.Bootstrap(context.Background(), pool, now); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	r, e := counts(t, pool)
	if r != 5000 {
		t.Errorf("Recipients = %d, want 5000", r)
	}
	if e < 45_000 || e > 55_000 {
		t.Errorf("Events = %d, want about 50k", e)
	}

	for _, letter := range []string{"ş", "ğ", "ı", "İ"} {
		var n int
		queryRow(t, pool, `SELECT count(*) FROM recipients WHERE first_name || last_name LIKE '%' || $1 || '%'`, []any{letter}, &n)
		if n == 0 {
			t.Errorf("no Recipient name contains %q", letter)
		}
	}

	rows, err := pool.Query(ctx, `SELECT city FROM recipients GROUP BY city ORDER BY count(*) DESC LIMIT 3`)
	if err != nil {
		t.Fatal(err)
	}
	var top []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			t.Fatal(err)
		}
		top = append(top, c)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(top) != 3 || top[0] != "İstanbul" || top[1] != "Ankara" || top[2] != "İzmir" {
		t.Errorf("top cities = %v, want [İstanbul Ankara İzmir]", top)
	}

	var outside int
	queryRow(t, pool, `SELECT count(*) FROM events WHERE occurred_at > $1 OR occurred_at < $1::timestamptz - interval '90 days'`, []any{now}, &outside)
	if outside != 0 {
		t.Errorf("%d Events fall outside the last 90 days", outside)
	}

	var types int
	queryRow(t, pool, `SELECT count(DISTINCT type) FROM events`, nil, &types)
	if types != 3 {
		t.Errorf("distinct Event types = %d, want 3 (cart_add, purchase, page_view)", types)
	}

	var withSMS, withEmail int
	queryRow(t, pool, `SELECT count(*) FILTER (WHERE sms_consent), count(*) FILTER (WHERE email_consent) FROM recipients`, nil, &withSMS, &withEmail)
	if withSMS == 0 || withSMS == r || withEmail == 0 || withEmail == r || withSMS == withEmail {
		t.Errorf("consent should vary per Channel; got %d SMS and %d email of %d", withSMS, withEmail, r)
	}
}
