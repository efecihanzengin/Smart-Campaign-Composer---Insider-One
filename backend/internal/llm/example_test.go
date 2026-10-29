package llm_test

import (
	"testing"

	"campaigncomposer/internal/llm"
)

func TestIsExampleBriefIgnoresWhitespaceAndQuotes(t *testing.T) {
	for _, b := range []string{
		llm.ExampleBrief,
		"  Son 30 günde sepetine ürün ekleyip alışveriş yapmayan İstanbul'daki kullanıcılara %30 indirim hatırlatması gönder.\n\nSamimi ama kısa olsun.\t",
		"Son  30 günde sepetine ürün ekleyip alışveriş yapmayan İstanbul’daki kullanıcılara %30 indirim hatırlatması gönder. Samimi ama kısa olsun.",
		"“Son 30 günde sepetine ürün ekleyip alışveriş yapmayan İstanbul'daki kullanıcılara %30 indirim hatırlatması gönder. Samimi ama kısa olsun.”",
		`"Son 30 günde sepetine ürün ekleyip alışveriş yapmayan İstanbul` + "`" + `daki kullanıcılara %30 indirim hatırlatması gönder. Samimi ama kısa olsun."`,
	} {
		if !llm.IsExampleBrief(b) {
			t.Errorf("IsExampleBrief(%q) = false, want true", b)
		}
	}
	for _, b := range []string{
		"",
		"Son 30 günde sepetine ürün ekleyip alışveriş yapmayan Ankara'daki kullanıcılara %30 indirim hatırlatması gönder. Samimi ama kısa olsun.",
		"Son 30 günde sepetine ürün ekleyip alışveriş yapmayan İstanbul'daki kullanıcılara %30 indirim hatırlatması gönder.",
		"Tüm kullanıcılara yeni yıl kutlaması gönder.",
	} {
		if llm.IsExampleBrief(b) {
			t.Errorf("IsExampleBrief(%q) = true, want false", b)
		}
	}
}
