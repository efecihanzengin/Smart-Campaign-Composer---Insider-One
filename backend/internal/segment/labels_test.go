package segment_test

import (
	"slices"
	"testing"

	"campaigncomposer/internal/segment"
)

func TestEveryNodeRendersAsATurkishLabel(t *testing.T) {
	cases := []struct {
		name, rule string
		want       []string
	}{
		{"city", `{"field": "city", "op": "eq", "value": "İstanbul"}`, []string{"Şehir: İstanbul"}},
		{"city list", `{"field": "city", "op": "in", "value": ["İzmir", "Bursa", "Antalya"]}`, []string{"Şehir: İzmir, Bursa veya Antalya"}},
		{"language", `{"field": "language", "op": "eq", "value": "tr"}`, []string{"Dil: Türkçe"}},
		{"language list", `{"field": "language", "op": "in", "value": ["en", "de"]}`, []string{"Dil: İngilizce veya DE"}},
		{"signed up before", `{"field": "created_at", "op": "before", "value": "2025-01-01"}`, []string{"Kayıt tarihi: 1 Ocak 2025 öncesi"}},
		{"signed up after", `{"field": "created_at", "op": "after", "value": "2025-08-15"}`, []string{"Kayıt tarihi: 15 Ağustos 2025 sonrası"}},
		{"had an Event", `{"event": "cart_add", "within_days": 30, "has": true}`, []string{"Son 30 günde sepete ekledi"}},
		{"had no Event", `{"event": "purchase", "within_days": 30, "has": false}`, []string{"Son 30 günde satın almadı"}},
		{"min_count", `{"event": "page_view", "within_days": 7, "has": true, "min_count": 5}`, []string{"Son 7 günde en az 5 kez siteyi ziyaret etti"}},
		{"min_amount", `{"event": "purchase", "within_days": 60, "has": true, "min_amount": 1500.5}`, []string{"Son 60 günde toplam en az 1.500,50 TL satın aldı"}},
		{"both, negated", `{"event": "purchase", "within_days": 90, "has": false, "min_count": 2, "min_amount": 2000}`, []string{"Son 90 günde en az 2 kez, toplam en az 2.000 TL satın almadı"}},
		{"not", `{"not": {"field": "city", "op": "eq", "value": "Ankara"}}`, []string{"Hariç: Şehir: Ankara"}},
		{"any", `{"any": [{"field": "city", "op": "eq", "value": "Ankara"}, {"event": "purchase", "within_days": 7, "has": true}]}`,
			[]string{"Şehir: Ankara veya Son 7 günde satın aldı"}},
		{"nested groups are bracketed",
			`{"any": [{"all": [{"field": "language", "op": "eq", "value": "en"}, {"event": "cart_add", "within_days": 30, "has": true}]},
			          {"event": "purchase", "within_days": 30, "has": true}]}`,
			[]string{"(Dil: İngilizce ve Son 30 günde sepete ekledi) veya Son 30 günde satın aldı"}},
		{"negated group is bracketed",
			`{"not": {"any": [{"field": "city", "op": "eq", "value": "Ankara"}, {"field": "city", "op": "eq", "value": "İzmir"}]}}`,
			[]string{"Hariç: (Şehir: Ankara veya Şehir: İzmir)"}},
		{"top-level all is one label per child",
			`{"all": [
				{"field": "city", "op": "eq", "value": "İstanbul"},
				{"any": [{"field": "language", "op": "eq", "value": "en"}, {"event": "cart_add", "within_days": 30, "has": true}]},
				{"not": {"event": "purchase", "within_days": 30, "has": true}}]}`,
			[]string{
				"Şehir: İstanbul",
				"Dil: İngilizce veya Son 30 günde sepete ekledi",
				"Hariç: Son 30 günde satın aldı",
			}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prop, err := segment.ParseProposal(proposal(tc.rule))
			if err != nil {
				t.Fatalf("rejected: %v", err)
			}
			if got := segment.Labels(prop.Rule); !slices.Equal(got, tc.want) {
				t.Errorf("labels = %q, want %q", got, tc.want)
			}
		})
	}
}
