package llm

import "strings"

// ExampleBrief is the case's example Brief, the one the mock LLM is scripted
// for and the UI shows as the placeholder.
const ExampleBrief = "Son 30 günde sepetine ürün ekleyip alışveriş yapmayan İstanbul'daki kullanıcılara %30 indirim hatırlatması gönder. Samimi ama kısa olsun."

// quoteFolder turns typographic quotes into ASCII ones, so a Brief pasted from
// a document or chat still matches.
var quoteFolder = strings.NewReplacer(
	"‘", "'", "’", "'", "‚", "'", "‛", "'", "ʼ", "'", "´", "'", "`", "'",
	"“", `"`, "”", `"`, "„", `"`, "‟", `"`, "«", `"`, "»", `"`,
)

// IsExampleBrief reports whether brief is the example Brief, ignoring
// differences in whitespace and quote characters, and quotes around the whole
// Brief.
func IsExampleBrief(brief string) bool {
	return normalizeBrief(brief) == normalizeBrief(ExampleBrief)
}

func normalizeBrief(s string) string {
	s = strings.Join(strings.Fields(quoteFolder.Replace(s)), " ")
	for len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		s = strings.TrimSpace(s[1 : len(s)-1])
	}
	return s
}
