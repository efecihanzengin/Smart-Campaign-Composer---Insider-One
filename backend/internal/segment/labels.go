package segment

import (
	"fmt"
	"strings"
)

// Labels renders a rule as readable Turkish labels: one per child of a
// top-level all, otherwise one for the whole rule.
func Labels(rule Node) []string {
	if all, ok := rule.(All); ok {
		out := make([]string, len(all.Children))
		for i, c := range all.Children {
			out[i] = label(c, false)
		}
		return out
	}
	return []string{label(rule, false)}
}

// label renders one node; nested groups are bracketed so "ve" and "veya"
// never read ambiguously.
func label(n Node, nested bool) string {
	switch n := n.(type) {
	case All:
		return group(n.Children, " ve ", nested)
	case Any:
		return group(n.Children, " veya ", nested)
	case Not:
		return "Hariç: " + label(n.Child, true)
	case ProfileCond:
		return profileLabel(n)
	case EventCond:
		return eventLabel(n)
	}
	panic(fmt.Sprintf("segment: unknown node %T", n))
}

func group(children []Node, sep string, nested bool) string {
	parts := make([]string, len(children))
	for i, c := range children {
		parts[i] = label(c, true)
	}
	s := strings.Join(parts, sep)
	if nested {
		s = "(" + s + ")"
	}
	return s
}

func profileLabel(n ProfileCond) string {
	switch n.Field {
	case "city":
		return "Şehir: " + orList(n.Values)
	case "language":
		names := make([]string, len(n.Values))
		for i, v := range n.Values {
			names[i] = languageName(v)
		}
		return "Dil: " + orList(names)
	case "created_at":
		d := mustDay(n.Values[0])
		when := map[string]string{"before": "öncesi", "after": "sonrası"}[n.Op]
		return fmt.Sprintf("Kayıt tarihi: %d %s %d %s", d.Day(), months[d.Month()-1], d.Year(), when)
	}
	panic(fmt.Sprintf("segment: unknown field %q", n.Field))
}

func eventLabel(n EventCond) string {
	verb := eventVerbs[n.Type]
	v := verb.has
	if !n.Has {
		v = verb.hasNot
	}
	var qualifiers []string
	if n.MinCount > 0 {
		qualifiers = append(qualifiers, fmt.Sprintf("en az %d kez", n.MinCount))
	}
	if n.MinAmount != "" {
		qualifiers = append(qualifiers, "toplam en az "+formatTRY(n.MinAmount)+" TL")
	}
	if len(qualifiers) > 0 {
		v = strings.Join(qualifiers, ", ") + " " + v
	}
	return fmt.Sprintf("Son %d günde %s", n.WithinDays, v)
}

// orList joins values as "a, b veya c".
func orList(values []string) string {
	if len(values) == 1 {
		return values[0]
	}
	return strings.Join(values[:len(values)-1], ", ") + " veya " + values[len(values)-1]
}

func languageName(code string) string {
	if name, ok := languageNames[code]; ok {
		return name
	}
	return strings.ToUpper(code)
}

// formatTRY writes a validated amount the Turkish way: 1.500,50.
func formatTRY(amount string) string {
	whole, frac, _ := strings.Cut(amount, ".")
	whole = strings.TrimLeft(whole, "0")
	if whole == "" {
		whole = "0"
	}
	var b strings.Builder
	for i, r := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(r)
	}
	if strings.Trim(frac, "0") != "" {
		b.WriteString("," + (frac + "0")[:2])
	}
	return b.String()
}

var languageNames = map[string]string{"tr": "Türkçe", "en": "İngilizce"}

var months = [12]string{"Ocak", "Şubat", "Mart", "Nisan", "Mayıs", "Haziran",
	"Temmuz", "Ağustos", "Eylül", "Ekim", "Kasım", "Aralık"}

// eventVerbs is the whitelist of Event types, with how each reads in a label.
var eventVerbs = map[string]struct{ has, hasNot string }{
	"cart_add":  {"sepete ekledi", "sepete eklemedi"},
	"purchase":  {"satın aldı", "satın almadı"},
	"page_view": {"siteyi ziyaret etti", "siteyi ziyaret etmedi"},
}
