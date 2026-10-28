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
	instruction := ""
	if req.Regenerate != nil {
		instruction = req.Regenerate.Instruction
	}
	return defaultSegment(req.Brief, instruction)
}

func (m *Mock) Content(ctx context.Context, req ContentRequest, emit func(chunk string)) error {
	if m.ContentFunc != nil {
		return m.ContentFunc(ctx, req, emit)
	}
	return streamChunks(ctx, defaultContent(req.Brief, req.Regenerate), mockChunkSize, mockChunkDelay, emit)
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
// Content for a Turkish Brief, English otherwise. A regeneration writes only the
// asked Channel, in a different wording, or a shorter one when the
// instruction asks for it.
func defaultContent(brief string, regen *Regeneration) string {
	lower := strings.ToLower(brief)
	wantsSMS := strings.Contains(lower, "sms")
	wantsEmail := strings.Contains(lower, "e-posta") || strings.Contains(lower, "eposta") ||
		strings.Contains(lower, "email") || strings.Contains(lower, "e-mail") || strings.Contains(lower, "mail")
	channels := []string{"sms", "email"}
	switch {
	case regen != nil && regen.Channel != "":
		channels = []string{regen.Channel}
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
	switch {
	case regen != nil && shorter.MatchString(regen.Instruction):
		if isTurkish(brief) {
			sms = map[string]any{"body": "{{first_name}}, sepetin seni bekliyor! Bugün " + discount + "indirim."}
			email = map[string]any{
				"subject":   "{{first_name}}, " + discount + "indirim seni bekliyor",
				"preheader": "Sepetin hâlâ orada.",
				"body":      "Merhaba {{first_name}},\n\nSepetindekiler bugün " + discount + "indirimli. Kaçırma!",
				"cta_text":  "Sepete git",
			}
		} else {
			sms = map[string]any{"body": "{{first_name}}, your cart is waiting: " + discount + "off today."}
			email = map[string]any{
				"subject":   "{{first_name}}, " + discount + "off your cart",
				"preheader": "Your cart is still there.",
				"body":      "Hi {{first_name}},\n\nYour cart is " + discount + "off today. Don't miss it!",
				"cta_text":  "Go to cart",
			}
		}
	case regen != nil:
		if isTurkish(brief) {
			sms = map[string]any{"body": "Selam {{first_name}}! Sepetine ekledigin ürünler hala orada. " + discount +
				"indirim bugün bitiyor, kacirma."}
			email = map[string]any{
				"subject":   "Sepetin seni özledi, {{first_name}}: " + discount + "indirim",
				"preheader": "Beğendiğin ürünler hâlâ sepetinde, indirim bugün bitiyor.",
				"body": "Selam {{first_name}},\n\nSepetine eklediğin ürünler hâlâ seni bekliyor. {{city}} için bugüne özel " +
					discount + "indirimi kaçırma.\n\nSiparişini birkaç tıkla tamamlayabilirsin.",
				"cta_text": "Sepetime dön",
			}
		} else {
			sms = map[string]any{"body": "Hey {{first_name}}! Your cart misses you. Get " + discount + "off, today only."}
			email = map[string]any{
				"subject":   "Your cart misses you, {{first_name}}: " + discount + "off",
				"preheader": "The items you picked are still waiting. The offer ends today.",
				"body": "Hey {{first_name}},\n\nThe items you picked are still in your cart. Get " + discount +
					"off today only.\n\nIt only takes a few clicks.",
				"cta_text": "Back to my cart",
			}
		}
	case isTurkish(brief):
		sms = map[string]any{"body": "Merhaba {{first_name}}, sepetindeki ürünler seni bekliyor! Bugüne özel " + discount +
			"indirimle siparisini tamamla."}
		email = map[string]any{
			"subject":   "{{first_name}}, sepetindekiler için " + discount + "indirim seni bekliyor",
			"preheader": "Sepetine eklediğin ürünler hâlâ orada. İndirim kısa süreliğine geçerli.",
			"body": "Merhaba {{first_name}},\n\nSepetine eklediğin ürünleri unutmadık. {{city}} için bugüne özel " + discount +
				"indirimle siparişini şimdi tamamlayabilirsin.\n\nİndirim kısa bir süre geçerli, fırsatı kaçırma!",
			"cta_text": "Siparişi tamamla",
		}
	default:
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
	shorter      = regexp.MustCompile(`(?i)kısa|kisa|short`)
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
// A regeneration's instruction is read like more Brief, except that a city or
// a period it names replaces the Brief's.
func defaultSegment(brief, instruction string) (json.RawMessage, error) {
	days := 30
	for _, s := range []string{brief, instruction} {
		if m := lastDays.FindStringSubmatch(s); m != nil {
			if n, err := strconv.Atoi(m[1]); err == nil {
				days = n
			}
		}
	}
	lower := strings.ToLower(brief + " " + instruction)

	var conds []any
	city := ""
	for _, s := range []string{brief, instruction} {
		for _, c := range knownCities {
			if strings.Contains(s, c) {
				city = c
				break
			}
		}
	}
	if city != "" {
		conds = append(conds, map[string]any{"field": "city", "op": "eq", "value": city})
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
	// Recipients have no gender, so the Segment Rule can't express it.
	switch {
	case strings.Contains(lower, "kadın"):
		unsupported = append(unsupported, "Cinsiyet: kadın")
	case strings.Contains(lower, "erkek"):
		unsupported = append(unsupported, "Cinsiyet: erkek")
	}
	if len(conds) == 0 {
		// Nothing recognisable: fall back to recently active Recipients and say so.
		conds = append(conds, map[string]any{"event": "page_view", "within_days": 90, "has": true})
		unsupported = append(unsupported, "Brief'teki hedef kitle tarifi")
	}
	return json.Marshal(map[string]any{"rule": map[string]any{"all": conds}, "unsupported_criteria": unsupported})
}
