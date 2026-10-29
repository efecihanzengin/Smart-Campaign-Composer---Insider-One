# AI session 19 — Code review, Standards axis (subagent)

<!-- session: agent-a4d342b947a654449 -->

Claude Code session export (desktop app). Thinking blocks and internal UI tool calls are omitted; tool outputs are truncated; secrets and personal data are redacted.

---

## 🧑 Orchestrator

You are the Standards reviewer for branch `spec-2-core-flow` in ~/Documents/Projects/smart-campaign-composer. Read-only: do not edit files.

Diff: `git diff main...spec-2-core-flow` (70 files, ~10k lines). Commits: `git log main..spec-2-core-flow --oneline` (tickets #6–#17).

Standards sources (no CODING_STANDARDS/CONTRIBUTING/lint config exists): CLAUDE.md, docs/agents/domain.md, GLOSSARY.md (domain vocabulary must be used consistently in code and UI), docs/adr/*.md and DECISIONS.md (architecture decisions the code must respect), and the spec's Testing Decisions in `gh issue view 2` (tests exercise behaviour through public seams; assert observable outcomes, never internal calls/private structure; tests pass without an API key). Also: code should read like the surrounding pre-existing code (tickets #3–#5 on main) in naming, idiom and comment density.

Smell baseline (Fowler, Refactoring ch.3) — always judgement calls; a documented repo standard overrides:
- Mysterious Name: name doesn't reveal what it does/holds → rename.
- Duplicated Code: same logic shape in more than one hunk/file → extract.
- Feature Envy: method reaches into another object's data more than its own → move it.
- Data Clumps: same few fields/params travel together → bundle into a type.
- Primitive Obsession: primitive/string standing in for a domain concept → small type.
- Repeated Switches: same switch/if-cascade on the same type recurs → polymorphism or shared map.
- Shotgun Surgery: one logical change forces scattered edits across many files → gather.
- Divergent Change: one module edited for several unrelated reasons → split.
- Speculative Generality: abstraction/params/hooks for needs the spec doesn't have → delete.
- Message Chains: long a.b().c().d() navigation → hide behind one method.
- Middle Man: mostly delegates onward → cut it.
- Refused Bequest: implementer ignores most of what it inherits → composition.

Brief: Report, per file/hunk where relevant, (a) every place the diff violates a documented standard: cite the standard (file + the rule); and (b) any baseline smell you spot: name it and quote the hunk. Distinguish hard violations from judgement calls: documented-standard breaches can be hard, but baseline smells are always judgement calls, and a documented repo standard overrides the baseline. Skip anything tooling enforces. Under 400 words.

_(skill instructions loaded: skill)_

---

## 🤖 Claude

**Bash** — Show diff stats and glossary

```bash
git diff main...spec-2-core-flow --stat | tail -75; cat GLOSSARY.md; ls docs/adr; wc -l DECISIONS.md docs/adr/*
```

<details><summary>Result</summary>

```
.env.example                                       |  45 +-
 README.md                                          |  84 ++++
 backend/cmd/campaign/main.go                       |  34 +-
 backend/cmd/provider/main.go                       |  32 +-
 backend/go.mod                                     |  27 +-
 backend/go.sum                                     | 139 ++++++-
 backend/internal/config/config.go                  |  42 +-
 backend/internal/config/config_test.go             |  78 ++++
 backend/internal/content/content.go                | 286 +++++++++++++
 backend/internal/content/email.go                  |  49 +++
 backend/internal/content/email.html                |  23 ++
 backend/internal/content/partial_test.go           |  34 ++
 backend/internal/content/sms.go                    |  93 +++++
 backend/internal/content/sms_test.go               | 146 +++++++
 backend/internal/db/migrations/0004_content.sql    |  18 +
 backend/internal/db/migrations/0005_deliveries.sql |  29 ++
 backend/internal/db/migrations/0006_attempts.sql   |  17 +
 .../internal/db/migrations/0007_lease_reclaim.sql  |   3 +
 backend/internal/httpapi/approve.go                | 238 +++++++++++
 backend/internal/httpapi/approve_guards_test.go    | 162 ++++++++
 backend/internal/httpapi/approve_test.go           | 240 +++++++++++
 backend/internal/httpapi/campaigns.go              |  83 +++-
 backend/internal/httpapi/cancel.go                 |  40 ++
 backend/internal/httpapi/cancel_test.go            | 199 +++++++++
 backend/internal/httpapi/content.go                | 255 ++++++++++++
 backend/internal/httpapi/content_test.go           | 403 ++++++++++++++++++
 backend/internal/httpapi/crash_test.go             | 454 ++++++++++++++++++++
 backend/internal/httpapi/dashboard.go              | 144 +++++++
 backend/internal/httpapi/dashboard_test.go         | 134 ++++++
 backend/internal/httpapi/deliveries.go             |  87 ++++
 backend/internal/httpapi/edit.go                   | 164 ++++++++
 backend/internal/httpapi/edit_test.go              | 224 ++++++++++
 backend/internal/httpapi/generation.go             |  42 ++
 backend/internal/httpapi/metrics.go                |  75 ++++
 backend/internal/httpapi/ratelimit_test.go         | 165 ++++++++
 backend/internal/httpapi/regenerate.go             | 302 ++++++++++++++
 backend/internal/httpapi/regenerate_test.go        | 361 ++++++++++++++++
 backend/internal/httpapi/repair.go                 | 132 ++++++
 backend/internal/httpapi/repair_test.go            | 271 ++++++++++++
 backend/internal/httpapi/retries_test.go           | 286 +++++++++++++
… (108 more lines)
```
</details>

**Bash** — Read ADRs and spec testing decisions

```bash
cat DECISIONS.md docs/adr/*.md; gh issue view 2 | sed -n '/Testing Decisions/,/^## /p' | head -40
```

<details><summary>Result</summary>

```
# Decisions

Each line is one decision; the linked ADR holds the reasoning and rejected alternatives. Domain terms are defined in [GLOSSARY.md](GLOSSARY.md).

1. [Postgres is both the database and the delivery queue](docs/adr/0001-postgres-as-queue.md): no broker; a unique Delivery per (Campaign, Recipient, Channel) is enforced transactionally.
2. [SMS Content is written in GSM-7 by default](docs/adr/0002-sms-gsm7-by-default.md): encoding-aware one-segment limit; Turkish letters transliterated, personalization values included.
3. [Claude emits a whitelisted JSON Segment Rule, never SQL](docs/adr/0003-segment-rule-is-whitelisted-json-not-sql.md): the backend validates it and compiles it to parameterized SQL.
4. [Consent is enforced per Channel outside the Segment Rule](docs/adr/0004-consent-outside-segment-rule.md): compliance cannot depend on the prompt or the UI.
5. [Two separate LLM calls, each behind a validate-and-repair loop](docs/adr/0005-two-llm-calls-with-repair-loop.md): a structured Segment Rule call and a streamed Content call, each repaired up to 2 times.
6. [Idempotency key and claim lease prevent duplicate sends](docs/adr/0006-idempotency-key-and-lease.md): retries after timeouts and crashed workers are safe.
7. [Claude reports what it could not target instead of guessing](docs/adr/0007-unsupported-criteria-warning.md): `unsupported_criteria` is shown to the Marketer as a warning.
8. [One Content per Channel for now](docs/adr/0008-one-content-per-channel.md): per-language and A/B Content are deferred until after the core flow.
9. [The seed is deterministic relative to the day it runs](docs/adr/0009-seed-anchored-to-seed-day.md): identical data from a fixed random seed, timestamps measured back from the seed day.
# Postgres is both the database and the delivery queue

Deliveries are rows in Postgres, and workers claim them with `SELECT … FOR UPDATE SKIP LOCKED`; there is no separate broker (Redis, RabbitMQ, NATS). A unique constraint on (campaign, recipient, channel) makes "never send the same message to the same person twice" a transactional guarantee at enqueue time, which a broker alongside Postgres would split across two stores. The required throughput (100 msg/s per channel) is far below what Postgres handles, so the extra infrastructure would buy nothing but moving parts.

## Considered Options

- **Postgres + Redis streams**: faster queue, but deduplication and queue state live in different stores and can disagree after a crash.
- **Postgres + RabbitMQ/NATS**: a conventional broker, but more infrastructure for `docker compose up`, and deduplication would still have to live in Postgres.
# SMS Content is written in GSM-7 by default, with Turkish letters transliterated

A single ş, ğ, ı, İ, Ş or Ğ switches an SMS from GSM-7 (160 septets) to UCS-2 (70 characters), so "SMS ≤ 160 characters" is wrong for Turkish text. The validator is encoding-aware (extension-table characters cost 2 septets) and requires an SMS to fit in one segment of whatever encoding it ends up in. Claude is instructed to write SMS in GSM-7, transliterating ş→s, ğ→g, ı→i, İ→I, Ş→S, Ğ→G (ç, ö, ü are already in GSM-7). That is common practice in Turkish SMS marketing, and it keeps the full 160. Personalization values rendered into a GSM-7 SMS are transliterated the same way, so a Recipient named "Şükrü" can never flip their message to UCS-2 and overflow it.

## Considered Options

- **Count raw characters ≤ 160**: the case's literal wording, but it would let a 140-character Turkish SMS be split into 3 parts.
- **Let Claude pick the encoding freely**: correct, but Turkish copy gets squeezed into 70 characters most of the time.
- **Turkish National Language Shift Table (3GPP 23.038)**: keeps Turkish letters at about 155 septets, but provider support varies and it is hard to explain and test.
# Claude emits a whitelisted JSON Segment Rule, never SQL

Claude produces the Segment Rule as a JSON AST through a tool schema: logical nodes (`all` / `any` / `not`, max depth 3), profile conditions over whitelisted fields (`city`, `language`, `created_at`) with typed operators, and Event conditions (`{event, within_days, has, min_count?, min_amount?}`). The backend validates every node against these whitelists and compiles the tree to parameterized SQL itself; no text from the LLM ever reaches the database as SQL. This removes injection and "creative" queries (unbounded scans, writes, access to other tables) by construction, and makes the rule renderable as readable labels for the Marketer.

## Considered Options

- **Let Claude write SQL and run it read-only or sandboxed**: more expressive, but validating arbitrary SQL is effectively impossible, and the case explicitly forbids running LLM output as SQL.
- **Free-form JSON filters without a whitelist**: easy to emit, but field and operator names would drift and could not be compiled safely.

## Consequences
… (86 more lines)
```
</details>

**Bash** — Grep added lines for glossary avoid terms

```bash
git diff main...spec-2-core-flow -U0 -- backend web/src ':!*.sum' | grep '^+' | grep -inE '\b(user|users|operator|customer|contact|subscriber|retry|retries|gateway|vendor|job|blast|template|copy|prompt)\b' | grep -v '^+++' | head -80
```

<details><summary>Result</summary>

```
67:+	// Worker retry tuning. Zero (unset or invalid) leaves the worker's
183:+// Marketer's edits, ADR-0002) and renders the fixed email HTML template.
472:+	"html/template"
493:+var emailTemplate = template.Must(template.New("email").Parse(emailHTML))
497:+// RenderEmailHTML renders email Content into the fixed HTML template, with
655:+// smsEncoding is the encoding an SMS template is sent in, decided by its own
661:+// RenderSMS fills an SMS template for one Recipient. In a GSM-7 SMS the values
866:+-- One row per Provider request made for a Delivery, so an operator can see
869:+-- 'sent', 'retry' (back to pending with a later next_attempt_at) or
878:+    outcome     text        NOT NULL CHECK (outcome IN ('sent', 'retry', 'failed')),
2049:+// previewEmail renders email Content into the fixed HTML template with sample
2282:+	// A Turkish Brief yields Turkish copy.
2340:+		t.Errorf("repair prompt errors = %q, want the notice's %q", script.requests[1].Repair.Errors, notices[0].Errors)
2390:+		t.Errorf("email Content = %+v, want English copy", e)
2817:+			if len(d.Attempts) != 2 || d.Attempts[0].StatusCode != nil || d.Attempts[0].Outcome != "retry" ||
2819:+				t.Errorf("Delivery %d Attempts = %+v, want a cut-off retry then sent", d.ID, d.Attempts)
3264:+// operator can see why one was retried or failed. `state` and `channel`
3769:+// count Deliveries in those states, retries count Attempts that sent their
3772:+	type counts struct{ sent, failed, retries int64 }
3786:+		WHERE a.outcome = 'retry' GROUP BY d.channel`)
3794:+		var sent, failed, retries int64
3795:+		if err := rows.Scan(&ch, &sent, &failed, &retries); err != nil {
3806:+		c.retries += retries
3820:+		{"campaign_delivery_retries_total", "Attempts that failed and were scheduled for a retry.", func(c *counts) int64 { return c.retries }},
5092:+// fastRetries keeps backoff and the client timeout small so retry tests run
5167:+		wantOutcomes := []string{"retry", "retry", "sent"}
5174:+			t.Errorf("Delivery %d retried %v after the 429, want at least its Retry-After of 200ms", d.ID, gap)
5196:+			wantOutcome := "retry"
5268:+		if first.StatusCode != nil || first.Error == "" || first.Outcome != "retry" || first.LatencyMS < opts.Timeout.Milliseconds() {
5694:+	var user strings.Builder
5695:+	user.WriteString("Brief:\n" + req.Brief)
5697:+		user.WriteString("\n\nAudience (the Segment Rule, as shown to the Marketer):")
5699:+			user.WriteString("\n- " + l)
5703:+		user.WriteString(regenerationText(r, r.Channel+" Content",
5706:+	user.WriteString(repairText(req.Repair, contentToolName))
5713:+		Messages:   []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(user.String()))},
5745:+// regenerationText is appended to the user message when the Marketer asks
5766:+// repairText is appended to the user message of a repair call (ADR-0005): the
5812:+// as claude-sonnet-5-5, so the system prompt asks for the call instead, and a
5813:+// response without it is an error the repair loop can retry.
… (40 more lines)
```
</details>

**Bash** — Grep for message/segment identifiers

```bash
git diff main...spec-2-core-flow -U0 -- backend web/src ':!*.sum' | grep '^+' | grep -nE '\b([Mm]essages?|[Ss]egment[A-Z_a-z]*|[Tt]emplate[A-Z]\w*|[Ff]ilter)\b' | grep -v 'Segment Rule' | grep -viE 'sms segment|one segment|segments' | head -50
```

**Bash** — View pre-existing code style

```bash
git show main:backend/internal/httpapi/campaigns.go | head -80; ls backend/internal/*/ ; git show main --stat | head -5
```

<details><summary>Result</summary>

```
337:+	Message string `json:"message"`
361:+// message overflows (ADR-0002).
378:+		return result([]Issue{{Field: "body", Message: "SMS içeriği eksik."}}, nil)
387:+		issues = append(issues, Issue{Field: "body", Message: fmt.Sprintf(
390:+		issues = append(issues, Issue{Field: "body", Message: fmt.Sprintf(
401:+		return result([]Issue{{Field: "subject", Message: "E-posta içeriği eksik."}}, nil)
417:+			issues = append(issues, Issue{Field: f.field, Message: fmt.Sprintf("%s en fazla %d karakter olabilir (şu an %d).", f.name, f.max, n)})
441:+		return []Issue{{Field: field, Message: name + " boş olamaz."}}
457:+			issues = append(issues, Issue{Field: field, Message: fmt.Sprintf("Bilinmeyen yer tutucu %s; yalnızca {{first_name}} ve {{city}} kullanılabilir.", tok)})
462:+		issues = append(issues, Issue{Field: field, Message: "Kapanmamış yer tutucu; yalnızca {{first_name}} ve {{city}} kullanılabilir."})
589:+// Single-segment limits: septets in GSM-7, UTF-16 code units in UCS-2.
662:+// are made GSM-7 (ADR-0002), so the message keeps its encoding and its limit.
694:+		{"ş makes the message UCS-2", "Merhaba Ayşe", content.UCS2, 12},
901:+	"campaigncomposer/internal/segment"
937:+		writeJSON(w, http.StatusConflict, map[string]string{"error": conflict.message})
954:+type conflictError struct{ message string }
956:+func (e *conflictError) Error() string { return e.message }
962:+	Message string `json:"message"`
976:+	err := tx.QueryRow(ctx, `SELECT status, segment_rule, enabled_channels,
1003:+		return approvalResult{}, &blockedError{[]blockReason{{Code: "generating", Message: "İçerik hâlâ üretiliyor; bitmesini bekleyin."}}}
1007:+		reasons = append(reasons, blockReason{Code: "no_channel", Message: "En az bir kanal açık olmalı."})
1009:+	sms, email, err := segment.LongestValues(ctx, tx, rule)
1017:+	audience := map[string][]segment.Member{}
1023:+			reasons = append(reasons, blockReason{Code: "invalid_content", Channel: ch, Message: invalidMessage(ch, v)})
1025:+		if audience[ch], err = segment.Members(ctx, tx, rule, ch); err != nil {
1029:+			reasons = append(reasons, blockReason{Code: "empty_audience", Channel: ch, Message: channelNames[ch] +
1054:+		issues[i] = is.Message
1097:+func insertDeliveries(ctx context.Context, tx pgx.Tx, id int64, channel string, p content.Proposal, members []segment.Member) (int, error) {
1157:+		if strings.TrimSpace(r.Message) == "" {
1158:+			t.Errorf("reason %s has no message", key)
1229:+		SegmentFunc: scriptedSegment(`{"rule": ` + konyaRule + `, "unsupported_criteria": []}`).SegmentFunc,
1317:+		Message string `json:"message"`
1534:+	// Repairs is how many repairs the Segment call needed (ADR-0005).
1541:+func campaignAudience(ctx context.Context, q segment.Querier, id int64) (segment.Audience, error) {
1543:+	if err := q.QueryRow(ctx, `SELECT segment_rule FROM campaigns WHERE id = $1`, id).Scan(&ruleJSON); err != nil {
1544:+		return segment.Audience{}, err
1548:+		return segment.Audience{}, err
1550:+	return segment.CountAudience(ctx, q, rule)
1553:+func audienceSize(a segment.Audience, channel string) int {
1561:+func withAudience(channels []string, a segment.Audience) []string {
… (10 more lines)
```
</details>

<details><summary>Result</summary>

```
package httpapi

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"campaigncomposer/internal/llm"
	"campaigncomposer/internal/segment"
)

type segmentView struct {
	Rule                segment.Node `json:"rule"`
	Labels              []string     `json:"labels"`
	UnsupportedCriteria []string     `json:"unsupported_criteria"`
}

type campaignView struct {
	ID       int64            `json:"id"`
	Status   string           `json:"status"`
	Brief    string           `json:"brief"`
	Segment  segmentView      `json:"segment"`
	Audience segment.Audience `json:"audience"`
}

// createCampaign creates a draft Campaign from a Brief, asks the LLM for its
// Segment Rule and answers with the rule, its labels and the per-Channel
// Audience counts.
func (s *server) createCampaign(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Brief string `json:"brief"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Geçersiz istek."})
		return
	}
	brief := strings.TrimSpace(req.Brief)
	if brief == "" {
… (123 more lines)
```
</details>

**Bash** — Find repeated channel switches

```bash
cd ~/Documents/Projects/smart-campaign-composer/backend/internal && grep -rnE 'case "sms"|case "email"|== "sms"|== "email"|"sms", "email"|channelNames' --include=*.go . | grep -v _test | head -40; grep -rn 'case .sms.\|=== .sms.\|=== .email.' ../../web/src --include=*.ts --include=*.vue | grep -v test | head
```

<details><summary>Result</summary>

```
(eval):1: no matches found: --include=*.go
(eval):1: no matches found: --include=*.ts
```
</details>

**Bash** — Find repeated channel switches

```bash
grep -rnE 'case "sms"|case "email"|== "sms"|== "email"|"sms", "email"|channelNames' backend/internal | grep -v _test.go | head -40; echo ---; grep -rnE "=== 'sms'|=== 'email'|case 'sms'|'sms', 'email'" web/src | grep -v test | head
```

<details><summary>Result</summary>

```
backend/internal/llm/mock.go:77:	channels := []string{"sms", "email"}
backend/internal/llm/mock.go:159:		if ch == "email" {
backend/internal/llm/llm.go:69:- First decide which Channels the Brief asks for and list them in "channels" ("sms", "email"). If the Brief does not restrict the Channels, use both. Write Content only for those Channels.
backend/internal/llm/llm.go:79:    "channels": {"type": "array", "items": {"enum": ["sms", "email"]}},
backend/internal/content/content.go:18:var Channels = []string{"sms", "email"}
backend/internal/content/content.go:251:		case "sms":
backend/internal/content/content.go:253:		case "email":
backend/internal/httpapi/approve.go:142:			reasons = append(reasons, blockReason{Code: "empty_audience", Channel: ch, Message: channelNames[ch] +
backend/internal/httpapi/approve.go:162:var channelNames = map[string]string{"sms": "SMS", "email": "E-posta"}
backend/internal/httpapi/approve.go:169:	return channelNames[channel] + " içeriği kurallara uymuyor: " + strings.Join(issues, " ")
backend/internal/httpapi/approve.go:186:		case ch == "sms" && slices.Contains(enabled, ch):
backend/internal/httpapi/approve.go:188:		case ch == "email" && slices.Contains(enabled, ch):
backend/internal/httpapi/approve.go:217:		case "sms":
backend/internal/httpapi/approve.go:219:		case "email":
backend/internal/httpapi/repair.go:123:	case "sms":
backend/internal/httpapi/repair.go:125:	case "email":
backend/internal/httpapi/campaigns.go:50:	if channel == "sms" {
backend/internal/httpapi/campaigns.go:73:			out[ch] = "Bu hedef kitlede " + channelNames[ch] + " için izin vermiş alıcı yok, bu yüzden kanal kapatıldı."
backend/internal/httpapi/edit.go:68:	if channel == "email" {
backend/internal/httpapi/regenerate.go:263:		case "sms":
backend/internal/httpapi/regenerate.go:265:		case "email":
backend/internal/httpapi/regenerate.go:276:	case channel == "sms" && v.SMS != nil:
backend/internal/httpapi/regenerate.go:278:	case channel == "email" && v.Email != nil:
backend/internal/worker/worker.go:27:var Channels = []string{"sms", "email"}
---
web/src/api.ts:57:export const channels: Channel[] = ['sms', 'email']
web/src/stores/campaign.ts:234:      if (channel === 'email' && this.content.email) {
web/src/stores/campaign.ts:300:      if (channel === 'email' && this.content.email && !this.contentError) {
web/src/components/ContentReview.vue:46:    ? (['sms', 'email'] as Channel[]).filter((c) => store.content[c] && store.validation[c]?.valid === false)
web/src/components/ContentReview.vue:87:      <label v-for="c in (['sms', 'email'] as Channel[])" :key="c" class="flex items-center gap-2 text-sm">
web/src/components/ContentReview.vue:141:          :busy="store.regenerating === 'sms'"
web/src/components/ContentReview.vue:181:          :busy="store.regenerating === 'email'"
```
</details>

Let me look at those switch sites.

**Bash** — Inspect channel switch sites

```bash
cd ~/Documents/Projects/smart-campaign-composer/backend/internal && sed -n 240,262p content/content.go; echo ----A; sed -n 175,238p httpapi/approve.go; echo ----R; sed -n 110,132p httpapi/repair.go; echo ----G; sed -n 250,290p httpapi/regenerate.go; sed -n 20,30p worker/worker.go
```

<details><summary>Result</summary>

```
issues = append(issues, placeholders(f.field, f.value)...)
	}
	issues = append(issues, placeholders("cta_text", e.CTAText)...)
	return result(issues, usage)
}

// Validate checks the Content of every Channel in p.Channels.
func Validate(p Proposal, longest Longest) map[string]Validation {
	out := map[string]Validation{}
	for _, ch := range p.Channels {
		switch ch {
		case "sms":
			out[ch] = ValidateSMS(p.SMS, longest.SMS)
		case "email":
			out[ch] = ValidateEmail(p.Email, longest.Email)
		}
	}
	return out
}

func required(field, name, value string) []Issue {
	if strings.TrimSpace(value) == "" {
		return []Issue{{Field: field, Message: name + " boş olamaz."}}
----A
	rows, err := tx.Query(ctx, `SELECT channel, subject, preheader, body, cta_text FROM contents WHERE campaign_id = $1`, id)
	if err != nil {
		return p, err
	}
	defer rows.Close()
	for rows.Next() {
		var ch, subject, preheader, body, cta string
		if err := rows.Scan(&ch, &subject, &preheader, &body, &cta); err != nil {
			return p, err
		}
		switch {
		case ch == "sms" && slices.Contains(enabled, ch):
			p.SMS = &content.SMS{Body: body}
		case ch == "email" && slices.Contains(enabled, ch):
			p.Email = &content.Email{Subject: subject, Preheader: preheader, Body: body, CTAText: cta}
		}
… (125 more lines)
```
</details>

**Bash** — Inspect unit tests for seam conformance

```bash
cd ~/Documents/Projects/smart-campaign-composer/backend/internal && head -30 worker/bucket_test.go; echo ----; head -34 content/partial_test.go; echo; for f in llm/*_test.go config/config_test.go provider/provider_test.go content/sms_test.go; do echo "$f: $(grep -m1 ^package $f)"; done; grep -n 'func Test' llm/claude_test.go llm/mock_test.go llm/new_test.go
```

<details><summary>Result</summary>

```
package worker_test

import (
	"testing"
	"time"

	"campaigncomposer/internal/worker"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newBucket(rate float64) (*worker.Bucket, *clock) {
	c := &clock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	return worker.NewBucket(rate, c.now), c
}

func TestBucketPacesTakesAtTheRate(t *testing.T) {
	b, _ := newBucket(10)
	// Without time passing, the n-th take waits (n-1) tenths of a second: no
	// burst beyond one token.
	for i, want := range []time.Duration{0, 100 * time.Millisecond, 200 * time.Millisecond, 300 * time.Millisecond} {
		if got := b.Reserve(); got != want {
			t.Errorf("take %d waits %v, want %v", i+1, got, want)
		}
	}
}

----
package content_test

import (
	"reflect"
	"testing"

	"campaigncomposer/internal/content"
)

… (44 more lines)
```
</details>

**Bash** — Check Bucket usage and clock seam

```bash
cd ~/Documents/Projects/smart-campaign-composer/backend && grep -rn 'NewBucket\|Bucket' --include='*.go' . | grep -v bucket_test | head; sed -n 1,40p internal/worker/bucket.go; grep -rn 'Now\b\|now func' internal/worker/worker.go | head
```

<details><summary>Result</summary>

```
internal/worker/bucket.go:22:// Bucket is a token bucket that paces one Channel's Provider requests. It
internal/worker/bucket.go:27:type Bucket struct {
internal/worker/bucket.go:39:// NewBucket returns a Bucket allowing limit requests per second. now is the
internal/worker/bucket.go:41:func NewBucket(limit float64, now func() time.Time) *Bucket {
internal/worker/bucket.go:45:	return &Bucket{limit: limit, now: now, tokens: 1, last: now(), base: limit}
internal/worker/bucket.go:49:func (b *Bucket) Wait(ctx context.Context) error {
internal/worker/bucket.go:66:func (b *Bucket) Reserve() time.Duration {
internal/worker/bucket.go:81:func (b *Bucket) Observe429() {
internal/worker/bucket.go:93:func (b *Bucket) Rate() float64 {
internal/worker/bucket.go:99:func (b *Bucket) rateAt(now time.Time) float64 {
package worker

import (
	"context"
	"math"
	"sync"
	"time"
)

const (
	// cooldown is how long one cut of the rate lasts: 429s within a second
	// of a cut count as the same burst, and recovery starts a second after
	// the last 429.
	cooldown = time.Second
	// minRate is the floor the rate is never halved below.
	minRate = 1.0
	// recoverySteps is how many seconds a full climb from zero would take:
	// the rate recovers by limit/recoverySteps per second.
	recoverySteps = 5
)

// Bucket is a token bucket that paces one Channel's Provider requests. It
// holds at most one token, so requests are spread evenly at the rate and
// never burst above it. A burst of 429s halves the rate (at most once per
// second); once the 429s stop for a second, the rate climbs back towards the
// limit by a fifth of the limit per second.
type Bucket struct {
	limit float64
	now   func() time.Time

… (14 more lines)
```
</details>

**Bash** — Check how worker uses bucket

```bash
cd ~/Documents/Projects/smart-campaign-composer/backend/internal && grep -n -i 'bucket\|Observe429\|Reserve\|Rate()' worker/worker.go | head; grep -rn 'Rate()' --include=*.go . | head
```

<details><summary>Result</summary>

```
54:	// lowers it for a while; see Bucket.
113:	// buckets holds one token bucket per Channel; the map is not written
115:	buckets map[string]*Bucket
124:	buckets := make(map[string]*Bucket, len(Channels))
126:		buckets[ch] = NewBucket(opts.RateLimit*paceHeadroom, nil)
132:		buckets: buckets,
200:			err := w.buckets[channel].Wait(wait)
216:// Channel's bucket lets through in half the start window at its current
219:	fit := int(w.buckets[channel].Rate() * w.opts.startWindow().Seconds() / 2)
423:		w.buckets[channel].Observe429()
(eval):1: no matches found: --include=*.go
```
</details>

**Bash** — Read generation and repair handlers

```bash
cd ~/Documents/Projects/smart-campaign-composer/backend/internal/httpapi && cat generation.go; sed -n 1,110p repair.go
```

**Bash** — Read regenerate handler and list functions

```bash
cd ~/Documents/Projects/smart-campaign-composer/backend/internal/httpapi && sed -n 1,120p regenerate.go; grep -n '^func' content.go edit.go dashboard.go deliveries.go
```

<details><summary>Result</summary>

```
package httpapi

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
)

// generationTimeout is how long a started generation call blocks approval. A
// mark older than this is stale: the process running the call died.
const generationTimeout = 5 * time.Minute

var errNotDraft = errors.New("campaign is not a draft")

// beginGeneration marks a generation call for a draft Campaign as running, so
// approval is refused until it ends, and returns the function that ends it.
// The mark is set only while the Campaign is a draft, under the row lock
// approval takes, so a call can't start on an approved Campaign either.
func (s *server) beginGeneration(ctx context.Context, id int64) (end func(), err error) {
	var started time.Time
	err = s.pool.QueryRow(ctx, `UPDATE campaigns SET generation_started_at = clock_timestamp()
		WHERE id = $1 AND status = 'draft' RETURNING generation_started_at`, id).Scan(&started)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errNotDraft
	}
	if err != nil {
		return nil, err
	}
	return func() {
		// Only this call's own mark is cleared: a call started meanwhile keeps
		// blocking approval until it ends too.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := s.pool.Exec(ctx, `UPDATE campaigns SET generation_started_at = NULL
			WHERE id = $1 AND generation_started_at = $2`, id, started); err != nil {
			log.Printf("end generation of campaign %d: %v", id, err)
		}
… (112 more lines)
```
</details>

<details><summary>Result</summary>

```
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"slices"
	"strconv"

	"campaigncomposer/internal/content"
	"campaigncomposer/internal/llm"
	"campaigncomposer/internal/segment"
)

// regenerationTarget reads the Campaign and the Marketer's optional
// instruction of a regeneration request, writing the error response itself
// when it can't. Only a draft Campaign can be regenerated.
func (s *server) regenerationTarget(w http.ResponseWriter, r *http.Request) (storedCampaign, string, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Kampanya bulunamadı."})
		return storedCampaign{}, "", false
	}
	var req struct {
		Instruction string `json:"instruction"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Geçersiz istek."})
		return storedCampaign{}, "", false
	}
	c, err := s.loadCampaign(r.Context(), id)
	if errors.Is(err, errCampaignNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Kampanya bulunamadı."})
		return c, "", false
	}
	if err != nil {
… (98 more lines)
```
</details>

**Bash** — Count duplicated handler boilerplate

```bash
cd ~/Documents/Projects/smart-campaign-composer/backend/internal && grep -rn 'Kampanya bulunamadı' httpapi/*.go | grep -v _test | wc -l; grep -rn 'strconv.ParseInt(r.PathValue' httpapi/*.go | grep -v _test | wc -l; grep -rn 'pgxQuerier\|type Querier' . | grep -v _test | head -5; sed -n 125,160p httpapi/content.go; grep -n 'Longest' content/content.go | head -5
```

<details><summary>Result</summary>

```
13
       8
segment/compile.go:104:type Querier interface {
httpapi/dashboard.go:48:func campaignsProgress(ctx context.Context, q pgxQuerier, where string, args ...any) ([]campaignProgress, error) {
httpapi/dashboard.go:84:type pgxQuerier interface {
		return nil, fmt.Errorf("stored Segment Rule: %w", err)
	}
	return p.Rule, nil
}

// saveContent replaces the Campaign's Content with the proposal's and enables
// the Channels Claude inferred, except those whose Audience is empty. It
// returns the enabled Channels.
func saveContent(ctx context.Context, s *server, id int64, p content.Proposal) (enabled []string, err error) {
	return enabled, pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		audience, err := campaignAudience(ctx, tx, id)
		if err != nil {
			return err
		}
		enabled = withAudience(p.Channels, audience)
		if _, err := tx.Exec(ctx, `DELETE FROM contents WHERE campaign_id = $1`, id); err != nil {
			return err
		}
		if p.SMS != nil && slices.Contains(p.Channels, "sms") {
			if _, err := tx.Exec(ctx, `INSERT INTO contents (campaign_id, channel, body) VALUES ($1, 'sms', $2)`,
				id, p.SMS.Body); err != nil {
				return err
			}
		}
		if e := p.Email; e != nil && slices.Contains(p.Channels, "email") {
			if _, err := tx.Exec(ctx, `INSERT INTO contents (campaign_id, channel, subject, preheader, body, cta_text)
				VALUES ($1, 'email', $2, $3, $4, $5)`, id, e.Subject, e.Preheader, e.Body, e.CTAText); err != nil {
				return err
			}
		}
		_, err = tx.Exec(ctx, `UPDATE campaigns SET enabled_channels = $2 WHERE id = $1`, id, enabled)
		return err
	})
}

… (4 more lines)
```
</details>

**Bash** — Compare querier interfaces and upsertContent

```bash
cd ~/Documents/Projects/smart-campaign-composer/backend/internal && sed -n 80,90p httpapi/dashboard.go; sed -n 100,110p segment/compile.go; sed -n 145,164p httpapi/edit.go
```

<details><summary>Result</summary>

```
}
	return out, rows.Err()
}

type pgxQuerier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func (s *server) getCampaign(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
	Email int `json:"email"`
}

// Querier is satisfied by a pool, a connection or a transaction.
type Querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Longest is the longest first name and city among one Channel's Audience,
// empty when the Audience is.
type Longest struct {
// Channel's alone. The draft check is repeated in the write, under a share
// lock that waits for an approval holding the row, so a write racing approval
// can't change Content that is already frozen: it affects no row.
func (s *server) upsertContent(ctx context.Context, id int64, p content.Proposal) (any, pgconn.CommandTag, error) {
	if sms := p.SMS; sms != nil {
		tag, err := s.pool.Exec(ctx, `INSERT INTO contents (campaign_id, channel, body)
			SELECT id, 'sms', $2 FROM campaigns WHERE id = $1 AND status = 'draft' FOR SHARE
			ON CONFLICT (campaign_id, channel) DO UPDATE SET body = EXCLUDED.body, updated_at = now()`, id, sms.Body)
		return sms, tag, err
	}
	if e := p.Email; e != nil {
		tag, err := s.pool.Exec(ctx, `INSERT INTO contents (campaign_id, channel, subject, preheader, body, cta_text)
			SELECT id, 'email', $2, $3, $4, $5 FROM campaigns WHERE id = $1 AND status = 'draft' FOR SHARE
			ON CONFLICT (campaign_id, channel) DO UPDATE SET subject = EXCLUDED.subject, preheader = EXCLUDED.preheader,
				body = EXCLUDED.body, cta_text = EXCLUDED.cta_text, updated_at = now()`,
			id, e.Subject, e.Preheader, e.Body, e.CTAText)
		return e, tag, err
	}
… (2 more lines)
```
</details>

**Bash** — Check frontend tests for internal-call assertions

```bash
cd ~/Documents/Projects/smart-campaign-composer/web/src && grep -n 'toHaveBeenCalled\|\.vm\.\|setData\|store\.\w* =' components/*.test.ts | head -20; grep -c 'toHaveBeenCalled' components/*.test.ts
```

<details><summary>Result</summary>

```
components/CampaignComposer.test.ts:63:    expect(fetchMock).toHaveBeenCalledWith('/api/campaigns', expect.objectContaining({ method: 'POST' }))
components/CampaignDashboard.test.ts:146:    expect(fetchMock).toHaveBeenCalledWith('/api/campaigns/7/cancel', { method: 'POST' })
components/CampaignComposer.test.ts:1
components/ApprovalPanel.test.ts:0
components/CampaignDashboard.test.ts:1
components/ContentReview.test.ts:0
components/Regeneration.test.ts:0
```
</details>

**Bash** — Read domain docs guidance

```bash
cat docs/agents/domain.md
```

<details><summary>Result</summary>

```
# Domain Docs

How the engineering skills should consume this repo's domain documentation when exploring the codebase.

## Before exploring, read these

- **`GLOSSARY.md`** at the repo root, or
- **`GLOSSARY-MAP.md`** at the repo root if it exists: it points at one `GLOSSARY.md` per context. Read each one relevant to the topic.
- **`docs/adr/`**: read ADRs that touch the area you're about to work in. In multi-context repos, also check `src/<context>/docs/adr/` for context-scoped decisions.

If any of these files don't exist, **proceed silently**. Don't flag their absence; don't suggest creating them upfront. The `/domain-modeling` skill (reached via `/grill-with-docs` and `/improve-codebase-architecture`) creates them lazily when terms or decisions actually get resolved.

## File structure

Single-context repo (most repos):

```
/
├── GLOSSARY.md
├── docs/adr/
│   ├── 0001-event-sourced-orders.md
│   └── 0002-postgres-for-write-model.md
└── src/
```

Multi-context repo (presence of `GLOSSARY-MAP.md` at the root):

```
/
├── GLOSSARY-MAP.md
├── docs/adr/                          ← system-wide decisions
└── src/
    ├── ordering/
    │   ├── GLOSSARY.md
    │   └── docs/adr/                  ← context-specific decisions
    └── billing/
        ├── GLOSSARY.md
        └── docs/adr/
```

… (11 more lines)
```
</details>

**SubagentHandback**

<details><summary>Result</summary>

```
{"success":true,"message":"Report delivered to your caller."}
```
</details>
