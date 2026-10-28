package llm

import (
	"context"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Mock is the scripted LLM used when no Anthropic API key is configured, and
// in tests. Each call runs its script; a nil script falls back to the default,
// which reads a few keywords from the Brief so the example Brief and its close
// variants get a convincing proposal.
type Mock struct {
	SegmentFunc func(ctx context.Context, req SegmentRequest) (json.RawMessage, error)
	ContentFunc func(ctx context.Context, req ContentRequest, emit func(chunk string)) error
}

// NewMock returns a Mock running the default scripts.
func NewMock() *Mock {
	return &Mock{}
}

func (m *Mock) Segment(ctx context.Context, req SegmentRequest) (json.RawMessage, error) {
	if m.SegmentFunc != nil {
		return m.SegmentFunc(ctx, req)
	}
	return defaultSegment(req.Brief)
}

func (m *Mock) Content(ctx context.Context, req ContentRequest, emit func(chunk string)) error {
	if m.ContentFunc != nil {
		return m.ContentFunc(ctx, req, emit)
	}
	return streamChunks(ctx, defaultContent(req.Brief), mockChunkSize, mockChunkDelay, emit)
}

const (
	// The default script streams in small chunks with a short pause, so the
	// zero-setup demo visibly types out like the real model.
	mockChunkSize  = 12
	mockChunkDelay = 25 * time.Millisecond
)

// streamChunks emits s in chunks of size runes, pausing between them.
func streamChunks(ctx context.Context, s string, size int, delay time.Duration, emit func(string)) error {
	runes := []rune(s)
	for i := 0; i < len(runes); i += size {
		if i > 0 && delay > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}
		emit(string(runes[i:min(i+size, len(runes))]))
	}
	return nil
}

// defaultContent is the mock's stand-in for Claude writing Content: it picks
// Channels from the Brief (both unless only one is named) and writes Turkish
// copy for a Turkish Brief, English otherwise.
func defaultContent(brief string) string {
	lower := strings.ToLower(brief)
	wantsSMS := strings.Contains(lower, "sms")
	wantsEmail := strings.Contains(lower, "e-posta") || strings.Contains(lower, "eposta") ||
		strings.Contains(lower, "email") || strings.Contains(lower, "e-mail") || strings.Contains(lower, "mail")
	channels := []string{"sms", "email"}
	switch {
	case wantsSMS && !wantsEmail:
		channels = []string{"sms"}
	case wantsEmail && !wantsSMS:
		channels = []string{"email"}
	}

	discount := ""
	if m := percent.FindString(brief); m != "" {
		discount = m + " "
	}
	var sms, email map[string]any
	if isTurkish(brief) {
		sms = map[string]any{"body": "Merhaba {{first_name}}, sepetindeki ürünler seni bekliyor! Bugüne özel " + discount +
			"indirimle siparisini tamamla."}
		email = map[string]any{
			"subject":   "{{first_name}}, sepetindekiler için " + discount + "indirim seni bekliyor",
			"preheader": "Sepetine eklediğin ürünler hâlâ orada. İndirim kısa süreliğine geçerli.",
			"body": "Merhaba {{first_name}},\n\nSepetine eklediğin ürünleri unutmadık. {{city}} için bugüne özel " + discount +
				"indirimle siparişini şimdi tamamlayabilirsin.\n\nİndirim kısa bir süre geçerli, fırsatı kaçırma!",
			"cta_text": "Siparişi tamamla",
		}
	} else {
		sms = map[string]any{"body": "Hi {{first_name}}, the items in your cart are waiting! Complete your order today with " +
			discount + "off."}
		email = map[string]any{
			"subject":   "{{first_name}}, your cart is waiting with " + discount + "off",
			"preheader": "The items you added are still there. The offer ends soon.",
			"body": "Hi {{first_name}},\n\nWe saved the items in your cart. Complete your order today with " + discount +
				"off.\n\nThe offer is only valid for a short time.",
			"cta_text": "Complete my order",
		}
	}
	// encoding/json sorts map keys; write "channels" first like the real tool call.
	var sb strings.Builder
	sb.WriteString(`{"channels":`)
	sb.Write(mustMarshal(channels))
	for _, ch := range channels {
		v := sms
		if ch == "email" {
			v = email
		}
		sb.WriteString(`,"` + ch + `":`)
		sb.Write(mustMarshal(v))
	}
	sb.WriteString("}")
	return sb.String()
}

var (
	percent      = regexp.MustCompile(`%\d+|\d+%`)
	turkishWords = regexp.MustCompile(`(?i)(^|\s)(ve|için|gönder|olsun|kullanıcılara|indirim|son|günde|yapmayan|hatırlatması|sadece)(\s|[.,!]|$)`)
	englishWords = regexp.MustCompile(`(?i)(^|\s)(the|and|to|who|send|in|with|off|did|not|only)(\s|[.,!]|$)`)
)

func mustMarshal(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

// isTurkish guesses the Brief's language from common words; Turkish letters
// alone are no sign, since an English Brief may name İstanbul.
func isTurkish(s string) bool {
	return len(turkishWords.FindAllString(s, -1)) >= len(englishWords.FindAllString(s, -1))
}

var (
	lastDays = regexp.MustCompile(`(?i)son (\d+) gün`)
	// Turkish city names as they appear in a Brief, possibly with a suffix
	// ("İstanbul'daki", "Ankara'da").
	knownCities = []string{"İstanbul", "Ankara", "İzmir", "Bursa", "Antalya", "Adana", "Konya",
		"Gaziantep", "Kocaeli", "Mersin", "Kayseri", "Eskişehir", "Diyarbakır", "Samsun", "Trabzon"}
)

// defaultSegment is the mock's stand-in for Claude's understanding of a Brief.
func defaultSegment(brief string) (json.RawMessage, error) {
	days := 30
	if m := lastDays.FindStringSubmatch(brief); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			days = n
		}
	}
	lower := strings.ToLower(brief)

	var conds []any
	for _, c := range knownCities {
		if strings.Contains(brief, c) {
			conds = append(conds, map[string]any{"field": "city", "op": "eq", "value": c})
			break
		}
	}
	if strings.Contains(lower, "sepet") {
		conds = append(conds, map[string]any{"event": "cart_add", "within_days": days, "has": true})
	}
	switch {
	case strings.Contains(lower, "alışveriş yapmayan"), strings.Contains(lower, "satın almayan"):
		conds = append(conds, map[string]any{"event": "purchase", "within_days": days, "has": false})
	case strings.Contains(lower, "alışveriş yapan"), strings.Contains(lower, "satın alan"):
		conds = append(conds, map[string]any{"event": "purchase", "within_days": days, "has": true})
	}

	unsupported := []string{}
	if len(conds) == 0 {
		// Nothing recognisable: fall back to recently active Recipients and say so.
		conds = append(conds, map[string]any{"event": "page_view", "within_days": 90, "has": true})
		unsupported = append(unsupported, "Brief'teki hedef kitle tarifi")
	}
	return json.Marshal(map[string]any{"rule": map[string]any{"all": conds}, "unsupported_criteria": unsupported})
}
