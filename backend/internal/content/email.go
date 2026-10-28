package content

import (
	"bytes"
	_ "embed"
	"html/template"
	"regexp"
	"strings"
)

// Sample holds the placeholder values an email preview is rendered with.
type Sample struct {
	FirstName, City string
}

// PreviewSample is the Recipient the Marketer's email preview is addressed to.
var PreviewSample = Sample{FirstName: "Ayşe", City: "İstanbul"}

// Render replaces the supported placeholders with the sample's values.
func (s Sample) Render(text string) string {
	return strings.NewReplacer("{{first_name}}", s.FirstName, "{{city}}", s.City).Replace(text)
}

//go:embed email.html
var emailHTML string

var emailTemplate = template.Must(template.New("email").Parse(emailHTML))

var paragraphBreak = regexp.MustCompile(`\n[ \t]*\n\s*`)

// RenderEmailHTML renders email Content into the fixed HTML template, with
// placeholders filled from sample. Every value is HTML-escaped: Claude and the
// Marketer write plain text only.
func RenderEmailHTML(e Email, sample Sample) (string, error) {
	var paragraphs [][]string
	for _, p := range paragraphBreak.Split(strings.TrimSpace(sample.Render(e.Body)), -1) {
		if p = strings.TrimSpace(p); p != "" {
			paragraphs = append(paragraphs, strings.Split(p, "\n"))
		}
	}
	var buf bytes.Buffer
	err := emailTemplate.Execute(&buf, map[string]any{
		"Subject":    sample.Render(e.Subject),
		"Preheader":  sample.Render(e.Preheader),
		"Paragraphs": paragraphs,
		"CTAText":    strings.TrimSpace(sample.Render(e.CTAText)),
	})
	return buf.String(), err
}
