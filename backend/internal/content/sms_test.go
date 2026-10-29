package content_test

import (
	"strings"
	"testing"

	"campaigncomposer/internal/content"
)

func TestSMSLengthCountsSeptetsInGSM7AndCharactersInUCS2(t *testing.T) {
	for _, tc := range []struct {
		name     string
		text     string
		encoding string
		used     int
	}{
		{"empty", "", content.GSM7, 0},
		{"ascii", "Hello, World!", content.GSM7, 13},
		{"basic table letters and symbols", "@£$¥èéùìòÇØøÅåΔ_ΦΓΛΩΠΨΣΘΞÆæßÉ¤¡ÄÖÑÜ§¿äöñüà", content.GSM7, 42},
		{"line feed and carriage return are basic", "a\nb\rc", content.GSM7, 5},
		{"Turkish letters already in GSM-7", "çÇöÖüÜ", content.GSM7, 6},
		{"each extension character costs two", "^{}\\[~]|€", content.GSM7, 18},
		{"form feed is an extension character", "\f", content.GSM7, 2},
		{"mixed basic and extension", "Fiyat: 10€ [indirim]", content.GSM7, 23},
		{"ş makes the message UCS-2", "Merhaba Ayşe", content.UCS2, 12},
		{"ğ", "doğru", content.UCS2, 5},
		{"dotless ı", "ılık", content.UCS2, 4},
		{"dotted İ", "İstanbul", content.UCS2, 8},
		{"Ş and Ğ", "ŞĞ", content.UCS2, 2},
		{"circumflex â", "hâlâ", content.UCS2, 4},
		{"extension characters cost one in UCS-2", "ş€{", content.UCS2, 3},
		{"astral characters take two UTF-16 units", "ş😀", content.UCS2, 3},
		{"backtick is not GSM-7", "`", content.UCS2, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := content.MeasureSMS(tc.text)
			if got.Encoding != tc.encoding || got.Used != tc.used {
				t.Errorf("MeasureSMS(%q) = %s %d, want %s %d", tc.text, got.Encoding, got.Used, tc.encoding, tc.used)
			}
			wantLimit := map[string]int{content.GSM7: 160, content.UCS2: 70}[tc.encoding]
			if got.Limit != wantLimit {
				t.Errorf("MeasureSMS(%q).Limit = %d, want %d", tc.text, got.Limit, wantLimit)
			}
		})
	}
}

func TestTransliterateReplacesTheTurkishLettersOutsideGSM7(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"ş ğ ı İ Ş Ğ", "s g i I S G"},
		{"Şükrü Çağlayan, İzmir", "Sükrü Çaglayan, Izmir"},
		{"çöüÇÖÜ", "çöüÇÖÜ"},
		{"Kampanya {{first_name}}", "Kampanya {{first_name}}"},
	} {
		if got := content.Transliterate(tc.in); got != tc.want {
			t.Errorf("Transliterate(%q) = %q, want %q", tc.in, got, tc.want)
		}
		if m := content.MeasureSMS(content.Transliterate(tc.in)); m.Encoding != content.GSM7 {
			t.Errorf("Transliterate(%q) is %s, want GSM-7", tc.in, m.Encoding)
		}
	}
}

func TestSMSIsValidOnlyWhenItFitsOneSegmentOfItsEncoding(t *testing.T) {
	for _, tc := range []struct {
		name  string
		body  string
		valid bool
	}{
		{"160 septets of GSM-7", strings.Repeat("a", 160), true},
		{"161 septets of GSM-7", strings.Repeat("a", 161), false},
		{"extension characters push GSM-7 over 160", strings.Repeat("a", 150) + strings.Repeat("€", 6), false},
		{"70 characters of UCS-2", "ş" + strings.Repeat("a", 69), true},
		{"71 characters of UCS-2", "ş" + strings.Repeat("a", 70), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := content.ValidateSMS(&content.SMS{Body: tc.body}, content.Sample{})
			if v.Valid != tc.valid {
				t.Errorf("valid = %v, want %v (issues %+v)", v.Valid, tc.valid, v.Issues)
			}
		})
	}
}

func TestSMSLengthUsesTheLongestPlaceholderValues(t *testing.T) {
	longest := content.Sample{FirstName: "Abdurrahman", City: "Kahramanmaraş"}
	// 11 + 13 = 24 septets of values; "Merhaba , " is 10 and the filler is 126.
	body := "Merhaba {{first_name}}, {{city}}" + strings.Repeat("a", 126)

	v := content.ValidateSMS(&content.SMS{Body: body}, longest)
	if u := v.Usage["body"]; u.Encoding != content.GSM7 || u.Used != 160 || u.Limit != 160 || !v.Valid {
		t.Errorf("usage = %+v valid = %v, want GSM-7 160/160 valid", u, v.Valid)
	}
	v = content.ValidateSMS(&content.SMS{Body: body + "a"}, longest)
	if v.Valid {
		t.Errorf("161 septets after rendering is valid, want invalid")
	}
}

func TestGSM7SMSRenderedForATurkishNameStaysGSM7(t *testing.T) {
	body := "Merhaba {{first_name}}, {{city}} magazamiz seni bekliyor."
	r := content.Sample{FirstName: "Şükrü", City: "Muğla"}

	got := content.RenderSMS(body, r)

	if want := "Merhaba Sükrü, Mugla magazamiz seni bekliyor."; got != want {
		t.Errorf("RenderSMS = %q, want %q", got, want)
	}
	if m := content.MeasureSMS(got); m.Encoding != content.GSM7 {
		t.Errorf("rendered SMS is %s, want GSM-7", m.Encoding)
	}
}

func TestUCS2SMSKeepsTurkishLettersInRenderedValues(t *testing.T) {
	got := content.RenderSMS("Merhaba {{first_name}}, iyi alışverişler", content.Sample{FirstName: "Şükrü"})
	if want := "Merhaba Şükrü, iyi alışverişler"; got != want {
		t.Errorf("RenderSMS = %q, want %q", got, want)
	}
}

func TestGSM7SMSNeverRendersANonGSM7Value(t *testing.T) {
	got := content.RenderSMS("Merhaba {{first_name}}", content.Sample{FirstName: "Zoë 😀"})
	if m := content.MeasureSMS(got); m.Encoding != content.GSM7 {
		t.Errorf("RenderSMS = %q is %s, want GSM-7", got, m.Encoding)
	}
}

func TestEmailLengthUsesTheLongestPlaceholderValues(t *testing.T) {
	longest := content.Sample{FirstName: "Abdurrahman", City: "Kahramanmaraş"}
	subject := "{{first_name}}" + strings.Repeat("a", 69) // 11 + 69 = 80

	v := content.ValidateEmail(&content.Email{Subject: subject, Preheader: "p", Body: "b"}, longest)
	if u := v.Usage["subject"]; u.Used != 80 || u.Limit != 80 || !v.Valid {
		t.Errorf("subject usage = %+v valid = %v, want 80/80 valid", u, v.Valid)
	}
	v = content.ValidateEmail(&content.Email{Subject: subject + "a", Preheader: "p", Body: "b"}, longest)
	if v.Valid {
		t.Errorf("81 characters after rendering is valid, want invalid")
	}
	if u := v.Usage["preheader"]; u.Used != 1 || u.Limit != 120 {
		t.Errorf("preheader usage = %+v, want 1/120", u)
	}
	if u := v.Usage["body"]; u.Used != 1 || u.Limit != 1500 {
		t.Errorf("body usage = %+v, want 1/1500", u)
	}
}
