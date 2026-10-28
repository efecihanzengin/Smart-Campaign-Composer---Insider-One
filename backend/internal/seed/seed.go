// Package seed fills an empty database with fake Recipients and their Events.
//
// The data is generated from a fixed random seed, so it is identical on every
// fresh run. Timestamps are offsets back from an anchor (the start of the UTC
// day the seed runs on), so "last 30 days" Audiences stay meaningful no matter
// when the stack is first started.
package seed

import (
	"context"
	"fmt"
	"math/big"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	recipientCount = 5000
	// Events fall within 89 days before the anchor, so they stay inside "the
	// last 90 days" for the whole day the seed runs on.
	eventWindow = 89 * 24 * time.Hour
	// seedLock is the advisory lock key that serializes concurrent seeders.
	seedLock = 7_301_002
)

// PCG output is specified by math/rand/v2, so it does not change across Go versions.
const rngSeed1, rngSeed2 = 20261028, 3

// Anchor returns the time every seeded timestamp is measured back from.
func Anchor(now time.Time) time.Time {
	return now.UTC().Truncate(24 * time.Hour)
}

// Run seeds the database if the recipients table is empty, and reports
// whether it did.
func Run(ctx context.Context, pool *pgxpool.Pool, anchor time.Time) (bool, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", seedLock); err != nil {
		return false, err
	}
	var hasRecipients bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM recipients)").Scan(&hasRecipients); err != nil {
		return false, err
	}
	if hasRecipients {
		return false, nil
	}

	recipients, events := generate(anchor)
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"recipients"},
		[]string{"id", "first_name", "last_name", "email", "phone", "city", "language", "sms_consent", "email_consent", "created_at"},
		pgx.CopyFromSlice(len(recipients), func(i int) ([]any, error) {
			r := recipients[i]
			return []any{r.id, r.firstName, r.lastName, r.email, r.phone, r.city, r.language, r.smsConsent, r.emailConsent, r.createdAt}, nil
		})); err != nil {
		return false, fmt.Errorf("copy recipients: %w", err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"events"},
		[]string{"recipient_id", "type", "occurred_at", "amount"},
		pgx.CopyFromSlice(len(events), func(i int) ([]any, error) {
			e := events[i]
			return []any{e.recipientID, e.typ, e.occurredAt, e.amount}, nil
		})); err != nil {
		return false, fmt.Errorf("copy events: %w", err)
	}
	// Ids were supplied explicitly; move the identity past them.
	if _, err := tx.Exec(ctx, "SELECT setval(pg_get_serial_sequence('recipients', 'id'), (SELECT max(id) FROM recipients))"); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

type recipient struct {
	id                       int64
	firstName, lastName      string
	email, phone             string
	city, language           string
	smsConsent, emailConsent bool
	createdAt                time.Time
}

type event struct {
	recipientID int64
	typ         string
	occurredAt  time.Time
	amount      pgtype.Numeric
}

var firstNames = []string{
	"Ahmet", "Mehmet", "Mustafa", "Ayşe", "Fatma", "Zeynep", "Elif", "Emre", "Hüseyin", "Şükrü",
	"Çağla", "Ömer", "İsmail", "İpek", "Ilgın", "Doğan", "Buğra", "Özge", "Ümit", "Şeyma",
	"Ebru", "Hakan", "Burak", "Can", "Deniz", "Ece", "Cem", "Kübra", "Merve", "Tuğba",
	"Gökhan", "Yağmur", "Barış", "Işıl", "Irmak", "Çiğdem", "Şule", "Oğuz", "Serkan", "Selin",
	"Yusuf", "Kerem", "Eylül", "Defne", "Arda", "Berk", "Duygu", "Aslı", "Neslihan", "Sıla",
}

var lastNames = []string{
	"Yılmaz", "Kaya", "Demir", "Şahin", "Çelik", "Yıldız", "Yıldırım", "Öztürk", "Aydın", "Özdemir",
	"Arslan", "Doğan", "Kılıç", "Aslan", "Çetin", "Kara", "Koç", "Kurt", "Özkan", "Şimşek",
	"Polat", "Erdoğan", "Güneş", "Akın", "Aktaş", "Işık", "İnce", "Uğur", "Bulut", "Tekin",
}

// cities are weighted towards İstanbul, Ankara and İzmir; weights sum to 100.
var cities = []struct {
	name   string
	weight int
}{
	{"İstanbul", 35}, {"Ankara", 15}, {"İzmir", 12}, {"Bursa", 6}, {"Antalya", 6},
	{"Adana", 4}, {"Konya", 4}, {"Gaziantep", 3}, {"Kocaeli", 3}, {"Mersin", 3},
	{"Kayseri", 2}, {"Eskişehir", 2}, {"Diyarbakır", 2}, {"Samsun", 2}, {"Trabzon", 1},
}

func generate(anchor time.Time) ([]recipient, []event) {
	rng := rand.New(rand.NewPCG(rngSeed1, rngSeed2))
	recipients := make([]recipient, 0, recipientCount)
	events := make([]event, 0, recipientCount*10)

	for i := 1; i <= recipientCount; i++ {
		first := firstNames[rng.IntN(len(firstNames))]
		last := lastNames[rng.IntN(len(lastNames))]
		language := "tr"
		if rng.IntN(10) == 0 {
			language = "en"
		}
		r := recipient{
			id:           int64(i),
			firstName:    first,
			lastName:     last,
			email:        fmt.Sprintf("%s.%s.%d@example.com", asciiLower(first), asciiLower(last), i),
			phone:        fmt.Sprintf("+905%02d%07d", 30+rng.IntN(70), rng.IntN(10_000_000)),
			city:         pickCity(rng),
			language:     language,
			smsConsent:   rng.IntN(100) < 65,
			emailConsent: rng.IntN(100) < 85,
			createdAt:    anchor.Add(-eventWindow - time.Duration(rng.Int64N(int64(640*24*time.Hour)))).Truncate(time.Second),
		}
		recipients = append(recipients, r)

		// 2–18 Events each, about 10 on average: mostly page views, some cart adds
		// and fewer purchases, spread over the last 90 days.
		n := 2 + rng.IntN(17)
		for range n {
			e := event{
				recipientID: r.id,
				occurredAt:  anchor.Add(-time.Duration(rng.Int64N(int64(eventWindow)))).Truncate(time.Second),
			}
			switch roll := rng.IntN(100); {
			case roll < 70:
				e.typ = "page_view"
			case roll < 90:
				e.typ = "cart_add"
			default:
				e.typ = "purchase"
				// Amount in kuruş, 50.00–5,000.00 TRY.
				e.amount = pgtype.Numeric{Int: big.NewInt(5_000 + rng.Int64N(495_001)), Exp: -2, Valid: true}
			}
			events = append(events, e)
		}
	}
	return recipients, events
}

func pickCity(rng *rand.Rand) string {
	roll := rng.IntN(100)
	for _, c := range cities {
		if roll < c.weight {
			return c.name
		}
		roll -= c.weight
	}
	return cities[0].name
}

var asciiFold = strings.NewReplacer(
	"ş", "s", "Ş", "s", "ğ", "g", "Ğ", "g", "ı", "i", "I", "i", "İ", "i",
	"ç", "c", "Ç", "c", "ö", "o", "Ö", "o", "ü", "u", "Ü", "u",
)

func asciiLower(s string) string {
	return strings.ToLower(asciiFold.Replace(s))
}
