package content_test

import (
	"reflect"
	"testing"

	"campaigncomposer/internal/content"
)

func TestPartialContentParsesAsFarAsItHasArrived(t *testing.T) {
	for _, tc := range []struct {
		partial string
		want    content.Proposal
		ok      bool
	}{
		{``, content.Proposal{}, false},
		{`{`, content.Proposal{}, true},
		{`{"chan`, content.Proposal{}, true},
		{`{"channels": ["sms", "em`, content.Proposal{Channels: []string{"sms", "em"}}, true},
		{`{"channels": ["sms"], "sms": {"body": "Merhaba {{fir`, content.Proposal{Channels: []string{"sms"}, SMS: &content.SMS{Body: "Merhaba {{fir"}}, true},
		{`{"sms": {"body": "a\`, content.Proposal{SMS: &content.SMS{Body: "a"}}, true},
		{`{"sms": {"body": "a\"b\u00`, content.Proposal{SMS: &content.SMS{Body: `a"b`}}, true},
		{`{"sms": {"body": "a\nb`, content.Proposal{SMS: &content.SMS{Body: "a\nb"}}, true},
		{`{"email": {"subject": "ş", "preheader"`, content.Proposal{Email: &content.Email{Subject: "ş"}}, true},
		{`{"email": {"subject": "ş"}, `, content.Proposal{Email: &content.Email{Subject: "ş"}}, true},
		// A multi-byte letter cut in half is held back until it is whole.
		{`{"sms": {"body": "` + "ş"[:1], content.Proposal{SMS: &content.SMS{}}, true},
	} {
		got, ok := content.ParsePartial(tc.partial)
		if ok != tc.ok || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("ParsePartial(%q) = %+v, %v; want %+v, %v", tc.partial, got, ok, tc.want, tc.ok)
		}
	}
}
