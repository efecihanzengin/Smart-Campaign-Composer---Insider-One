package content

import (
	"strings"
	"unicode/utf16"
)

// SMS encodings (ADR-0002).
const (
	GSM7 = "gsm7"
	UCS2 = "ucs2"
)

// Single-segment limits: septets in GSM-7, UTF-16 code units in UCS-2.
const (
	MaxGSM7Septets = 160
	MaxUCS2Units   = 70
)

// gsm7Basic is the GSM 03.38 default alphabet: each character costs one
// septet. Lowercase ç is included because 0x09 was ç in the original table
// and SMS centres still map it there, so Turkish ç, ö, ü keep GSM-7.
const gsm7Basic = "@£$¥èéùìòÇç\nØø\rÅåΔ_ΦΓΛΩΠΨΣΘΞÆæßÉ !\"#¤%&'()*+,-./0123456789:;<=>?" +
	"¡ABCDEFGHIJKLMNOPQRSTUVWXYZÄÖÑÜ§¿abcdefghijklmnopqrstuvwxyzäöñüà"

// gsm7Extension holds the characters reached through the escape code: each
// costs two septets.
const gsm7Extension = "\f^{}\\[~]|€"

// Length is how much of a field's limit some Content uses. Encoding is set
// for SMS only.
type Length struct {
	Encoding string `json:"encoding,omitempty"`
	Used     int    `json:"used"`
	Limit    int    `json:"limit"`
}

// MeasureSMS detects the encoding a text needs and counts its cost in that
// encoding: septets in GSM-7, UTF-16 code units in UCS-2.
func MeasureSMS(text string) Length {
	septets := 0
	for _, r := range text {
		switch {
		case strings.ContainsRune(gsm7Basic, r):
			septets++
		case strings.ContainsRune(gsm7Extension, r):
			septets += 2
		default:
			return Length{Encoding: UCS2, Used: len(utf16.Encode([]rune(text))), Limit: MaxUCS2Units}
		}
	}
	return Length{Encoding: GSM7, Used: septets, Limit: MaxGSM7Septets}
}

// turkish transliterates the Turkish letters that are not in GSM-7.
var turkish = strings.NewReplacer("ş", "s", "ğ", "g", "ı", "i", "İ", "I", "Ş", "S", "Ğ", "G")

// Transliterate replaces ş, ğ, ı, İ, Ş and Ğ with s, g, i, I, S and G, the
// rule Claude is instructed to follow for SMS (ADR-0002).
func Transliterate(s string) string {
	return turkish.Replace(s)
}

// circumflex covers the remaining letters of Turkish names outside GSM-7.
var circumflex = strings.NewReplacer("â", "a", "î", "i", "û", "u", "Â", "A", "Î", "I", "Û", "U")

// toGSM7 makes a placeholder value safe for a GSM-7 SMS: Turkish letters are
// transliterated and anything still outside GSM-7 becomes "?", so no
// Recipient's value can switch their SMS to UCS-2.
func toGSM7(value string) string {
	value = circumflex.Replace(Transliterate(value))
	return strings.Map(func(r rune) rune {
		if strings.ContainsRune(gsm7Basic, r) || strings.ContainsRune(gsm7Extension, r) {
			return r
		}
		return '?'
	}, value)
}

// smsEncoding is the encoding an SMS template is sent in, decided by its own
// text alone: placeholder values never change it.
func smsEncoding(body string) string {
	return MeasureSMS(placeholderToken.ReplaceAllString(body, "")).Encoding
}

// RenderSMS fills an SMS template for one Recipient. In a GSM-7 SMS the values
// are made GSM-7 (ADR-0002), so the message keeps its encoding and its limit.
func RenderSMS(body string, r Sample) string {
	if smsEncoding(body) == GSM7 {
		r = Sample{FirstName: toGSM7(r.FirstName), City: toGSM7(r.City)}
	}
	return r.Render(body)
}
