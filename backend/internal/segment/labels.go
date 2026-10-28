package segment

import "fmt"

// Labels renders a rule as readable Turkish labels, one per condition.
func Labels(rule Node) []string {
	var out []string
	var walk func(Node)
	walk = func(n Node) {
		switch n := n.(type) {
		case All:
			for _, c := range n.Children {
				walk(c)
			}
		case CityEq:
			out = append(out, "Şehir: "+n.Value)
		case EventCond:
			verb := eventVerbs[n.Type]
			v := verb.has
			if !n.Has {
				v = verb.hasNot
			}
			out = append(out, fmt.Sprintf("Son %d günde %s", n.WithinDays, v))
		}
	}
	walk(rule)
	return out
}

// eventVerbs is the whitelist of Event types, with how each reads in a label.
var eventVerbs = map[string]struct{ has, hasNot string }{
	"cart_add":  {"sepete ekledi", "sepete eklemedi"},
	"purchase":  {"satın aldı", "satın almadı"},
	"page_view": {"siteyi ziyaret etti", "siteyi ziyaret etmedi"},
}
