package segment

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Compile turns a validated rule into a boolean SQL condition over the
// recipients table aliased as r. The SQL text is built only from fixed
// fragments; every value from the rule is a bound parameter in args.
func Compile(rule Node) (where string, args []any) {
	c := compiler{}
	where = c.node(rule)
	return where, c.args
}

// profileSQL maps each whitelisted profile field and operator to the fixed
// SQL around its bound value, and optionally how the value is bound. City
// compares Turkish-casefolded on both sides.
var profileSQL = map[string]map[string]struct {
	prefix, suffix string
	value          func(string) any
}{
	"city": {
		"eq": {"tr_fold(r.city) = tr_fold(", ")", nil},
		"in": {"tr_fold(r.city) IN (SELECT tr_fold(unnest(", "::text[])))", nil},
	},
	"language": {
		"eq": {"r.language = ", "", nil},
		"in": {"r.language = ANY (", "::text[])", nil},
	},
	// before a day: signed up before it starts; after a day: once it has ended.
	"created_at": {
		"before": {"r.created_at < ", "", func(s string) any { return mustDay(s) }},
		"after":  {"r.created_at >= ", "", func(s string) any { return mustDay(s).AddDate(0, 0, 1) }},
	},
}

type compiler struct{ args []any }

func (c *compiler) bind(v any) string {
	c.args = append(c.args, v)
	return fmt.Sprintf("$%d", len(c.args))
}

func (c *compiler) node(n Node) string {
	switch n := n.(type) {
	case All:
		return c.join(n.Children, " AND ")
	case Any:
		return c.join(n.Children, " OR ")
	case Not:
		return "NOT (" + c.node(n.Child) + ")"
	case ProfileCond:
		frag, ok := profileSQL[n.Field][n.Op]
		if !ok {
			break
		}
		v := n.value()
		if frag.value != nil {
			v = frag.value(n.Values[0])
		}
		return frag.prefix + c.bind(v) + frag.suffix
	case EventCond:
		exists := "EXISTS"
		if !n.Has {
			exists = "NOT EXISTS"
		}
		sql := exists + " (SELECT 1 FROM events e WHERE e.recipient_id = r.id AND e.type = " + c.bind(n.Type) +
			" AND e.occurred_at >= now() - make_interval(days => " + c.bind(n.WithinDays) + ")"
		var having []string
		if n.MinCount > 0 {
			having = append(having, "count(*) >= "+c.bind(n.MinCount))
		}
		if n.MinAmount != "" {
			having = append(having, "sum(e.amount) >= "+c.bind(n.MinAmount)+"::numeric")
		}
		if len(having) > 0 {
			sql += " HAVING " + strings.Join(having, " AND ")
		}
		return sql + ")"
	}
	panic(fmt.Sprintf("segment: unknown node %T", n))
}

func (c *compiler) join(children []Node, op string) string {
	parts := make([]string, len(children))
	for i, child := range children {
		parts[i] = c.node(child)
	}
	return "(" + strings.Join(parts, op) + ")"
}

// Audience is the number of Recipients a rule reaches on each Channel.
type Audience struct {
	SMS   int `json:"sms"`
	Email int `json:"email"`
}

// Querier is satisfied by a pool, a connection or a transaction.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Longest is the longest first name and city among one Channel's Audience,
// empty when the Audience is.
type Longest struct {
	FirstName, City string
}

// LongestValues finds, per Channel, the longest placeholder values in the
// rule's Audience (consent applied), the values Content lengths are checked
// with (ADR-0002).
func LongestValues(ctx context.Context, q Querier, rule Node) (sms, email Longest, err error) {
	where, args := Compile(rule)
	longest := func(column, consent string) string {
		return `coalesce((SELECT r.` + column + ` FROM recipients r WHERE r.` + consent + ` AND ` + where +
			` ORDER BY length(r.` + column + `) DESC, r.` + column + ` LIMIT 1), '')`
	}
	err = q.QueryRow(ctx, `SELECT `+longest("first_name", "sms_consent")+`, `+longest("city", "sms_consent")+`, `+
		longest("first_name", "email_consent")+`, `+longest("city", "email_consent"), args...).
		Scan(&sms.FirstName, &sms.City, &email.FirstName, &email.City)
	return sms, email, err
}

// CountAudience counts the Recipients the rule reaches on each Channel,
// limited to those who consented to that Channel (ADR-0004).
func CountAudience(ctx context.Context, q Querier, rule Node) (Audience, error) {
	where, args := Compile(rule)
	var a Audience
	err := q.QueryRow(ctx, `SELECT count(*) FILTER (WHERE r.sms_consent), count(*) FILTER (WHERE r.email_consent)
		FROM recipients r WHERE `+where, args...).Scan(&a.SMS, &a.Email)
	return a, err
}

// Member is one Recipient of an Audience, with what a Delivery to them needs.
type Member struct {
	ID                     int64
	FirstName, City, Phone string
	Email                  string
}

// consentColumn is the consent flag of each Channel (ADR-0004).
var consentColumn = map[string]string{"sms": "r.sms_consent", "email": "r.email_consent"}

// Members lists the rule's Audience on one Channel, consent applied, as it is
// at this moment: approval freezes it from this.
func Members(ctx context.Context, tx pgx.Tx, rule Node, channel string) ([]Member, error) {
	consent, ok := consentColumn[channel]
	if !ok {
		return nil, fmt.Errorf("segment: unknown Channel %q", channel)
	}
	where, args := Compile(rule)
	rows, err := tx.Query(ctx, `SELECT r.id, r.first_name, r.city, r.phone, r.email FROM recipients r
		WHERE `+consent+` AND `+where+` ORDER BY r.id`, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Member, error) {
		var m Member
		err := row.Scan(&m.ID, &m.FirstName, &m.City, &m.Phone, &m.Email)
		return m, err
	})
}
