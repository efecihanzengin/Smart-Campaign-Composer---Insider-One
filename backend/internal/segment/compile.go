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

type compiler struct{ args []any }

func (c *compiler) bind(v any) string {
	c.args = append(c.args, v)
	return fmt.Sprintf("$%d", len(c.args))
}

func (c *compiler) node(n Node) string {
	switch n := n.(type) {
	case All:
		parts := make([]string, len(n.Children))
		for i, child := range n.Children {
			parts[i] = c.node(child)
		}
		return "(" + strings.Join(parts, " AND ") + ")"
	case CityEq:
		return "r.city = " + c.bind(n.Value)
	case EventCond:
		exists := "EXISTS"
		if !n.Has {
			exists = "NOT EXISTS"
		}
		return exists + " (SELECT 1 FROM events e WHERE e.recipient_id = r.id AND e.type = " + c.bind(n.Type) +
			" AND e.occurred_at >= now() - make_interval(days => " + c.bind(n.WithinDays) + "))"
	}
	panic(fmt.Sprintf("segment: unknown node %T", n))
}

// Audience is the number of Recipients a rule reaches on each Channel.
type Audience struct {
	SMS   int `json:"sms"`
	Email int `json:"email"`
}

// Querier is satisfied by a pool, a connection or a transaction.
type Querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
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
