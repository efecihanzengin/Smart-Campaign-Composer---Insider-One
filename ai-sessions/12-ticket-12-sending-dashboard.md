# AI session 12 — Ticket #12 sending and dashboard (subagent)

<!-- session: agent-ab40c40da8c9ccbf9 -->

Claude Code session export (desktop app). Thinking blocks and internal UI tool calls are omitted; tool outputs are truncated; secrets and personal data are redacted.

---

## 🧑 Orchestrator

You are an implementer for GitHub issue #12 ("Sending happy path and live dashboard"). Parent spec: issue #2. Read them with `gh issue view`, plus GLOSSARY.md, docs/adr/ (esp. ADR-0001, ADR-0006), and the notes in <scratchpad> (ticket-11.md has direct pointers for you). Another implementer is doing #10 (per-part regeneration in the generation paths) in parallel; stay out of generation code.

Scope only #12's happy path; failures/retries (#13), rate limiting (#14), leases/crash safety (#15) and cancel (#16) are later tickets — design the worker so they slot in, but don't build them now.

Steps:
1. Confirm your worktree is based on `origin/spec-2-core-flow` (fetch; `git merge-base --is-ancestor origin/spec-2-core-flow HEAD`); if not, `git reset --hard origin/spec-2-core-flow`. It must contain commit b5d306f (#11).
2. Call the Skill tool with `mattpocock-skills:tdd` and build the ticket test-first through the HTTP API seam with worker in-process and the real mock Provider, plus Vitest for the dashboard.
3. Run tests with `COMPOSE_PROJECT_NAME=smart-campaign-composer make test`.
4. One commit titled `Sending happy path and live dashboard (#12)` with a summary body ending with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Don't push or close the issue.
5. Before reporting, fetch and merge `origin/spec-2-core-flow` into your branch; resolve conflicts and re-run tests if anything came in.

Report in under 150 words: branch, SHA, worktree path, and pointers for #13, #14, #16 (where retries, the token bucket and cancellation should hook in). Longer notes go to the notes dir as ticket-12.md.

_(skill instructions loaded: skill)_

---

## 🤖 Claude

**Bash** — Check worktree base against spec branch

```bash
git fetch -q && git merge-base --is-ancestor origin/spec-2-core-flow HEAD; echo $status; git log --oneline -3 origin/spec-2-core-flow
```

<details><summary>Error</summary>

```
This agent is isolated in the worktree ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-ab40c40da8c9ccbf9, but this command names git in a form too complex to verify that it stays inside the worktree. Refusing to run it — a worktree-isolated agent's git operations must target its own worktree. Split it into plain, separate commands and run them from ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-ab40c40da8c9ccbf9.
```
</details>

**Bash** — Fetch origin

```bash
git fetch -q origin
```

<details><summary>Result</summary>

```
(Bash completed with no output)
```
</details>

**Bash** — Check whether spec branch is ancestor

```bash
git merge-base --is-ancestor origin/spec-2-core-flow HEAD; echo $status
```

<details><summary>Error</summary>

```
This agent is isolated in the worktree ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-ab40c40da8c9ccbf9, but this command names git in a form too complex to verify that it stays inside the worktree. Refusing to run it — a worktree-isolated agent's git operations must target its own worktree. Split it into plain, separate commands and run them from ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-ab40c40da8c9ccbf9.
```
</details>

**Bash** — Check whether spec branch is ancestor

```bash
git merge-base --is-ancestor origin/spec-2-core-flow HEAD
```

<details><summary>Error</summary>

```
Exit code 1
```
</details>

**Bash** — Reset worktree to spec branch

```bash
git reset --hard origin/spec-2-core-flow
```

<details><summary>Result</summary>

```
HEAD is now at 5b6a46f Merge remote-tracking branch 'origin/spec-2-core-flow' into worktree-agent-ac458a5b29496bd6b
```
</details>

**Bash** — Show recent commits

```bash
git log --oneline -12
```

**Bash** — Read issues 12 and 2

```bash
gh issue view 12; gh issue view 2
```

**Bash** — Read implementer notes

```bash
ls <scratchpad> cat <scratchpad>
```

<details><summary>Result</summary>

```
5b6a46f Merge remote-tracking branch 'origin/spec-2-core-flow' into worktree-agent-ac458a5b29496bd6b
b5d306f Approval freezes the Audience into Deliveries (#11)
e76c3b5 Validate-and-repair loop for both LLM calls (#9)
ad3c4a2 Merge remote-tracking branch 'origin/spec-2-core-flow' into worktree-agent-ada0a8a5b7b5138f8
3a4a3f3 SMS encoding rules, live counters and inline Content editing (#8)
a8a4c9c Real Claude client behind the LLM interface (#7)
d58973d Streamed Content generation (#6)
be3d0c9 Redact name parts in any case and add the export-session skill
2d58564 Read scrub terms from .scrub-terms and tag exports with their session
f77c01f Add AI session export for ticket #5
1bb8faf Full Segment Rule language and security tests (#5)
55ec654 Add AI session export for ticket #4
```
</details>

<details><summary>Result</summary>

```
title:	Sending happy path and live dashboard
state:	OPEN
author:	[redacted] ([redacted name])
labels:	ready-for-agent
comments:	0
assignees:	
projects:	
milestone:	
issue-type:	
parent:	[redacted]/Smart-Campaign-Composer---Insider-One#2
sub-issues:	
sub-issues-completed:	
blocked-by:	[redacted]/Smart-Campaign-Composer---Insider-One#11
blocking:	[redacted]/Smart-Campaign-Composer---Insider-One#16, [redacted]/Smart-Campaign-Composer---Insider-One#14, [redacted]/Smart-Campaign-Composer---Insider-One#13
number:	12
--
## Parent

#2

## What to build

Approved Campaigns actually go out, and the Marketer watches it happen. The worker claims `pending` Deliveries per Channel with `SELECT … FOR UPDATE SKIP LOCKED` and sends each to the mock Provider (`POST /sms`, `POST /email`) with `Idempotency-Key: <delivery id>` (ADR-0006). The Provider deduplicates on that key, returning the original result for a repeated key. A 2xx moves the Delivery to `sent`. The Campaign moves `approved → sending → completed` once no Delivery is `pending` or `sending`. A live dashboard (SSE) shows per-Channel `pending` / `sent` / `failed` counts and a progress bar per Campaign. In this slice, the Provider always succeeds.

## Acceptance criteria

- [ ] After approval, every Delivery reaches `sent` and the Campaign reaches `completed`
- [ ] Several concurrent workers never claim the same Delivery
- [ ] Every Provider request carries the Delivery id as `Idempotency-Key`, and the Provider returns the same result for a repeated key without recording a second message
- [ ] The dashboard updates live, without refresh, with per-Channel counts and a progress bar
- [ ] HTTP-level test: example Brief → approve → worker drains → the Provider received exactly one request per Delivery
- [ ] Vitest covers the dashboard rendering from SSE events

title:	Spec: Smart Campaign Composer — core flow
state:	OPEN
author:	[redacted] ([redacted name])
labels:	ready-for-agent
comments:	0
assignees:	
projects:	
… (243 more lines)
```
</details>

<details><summary>Result</summary>

```
ticket-11.md
ticket-6.md
ticket-7.md
ticket-8.md
ticket-9.md
# Ticket #11 notes

Branch worktree-agent-ac458a5b29496bd6b. Commit b5d306f (#11), then merge 5b6a46f of origin/spec-2-core-flow
(brought in #9, e76c3b5; conflicts in httpapi/campaigns.go and web/src/api.ts resolved by keeping both sides).
`COMPOSE_PROJECT_NAME=smart-campaign-composer make test` is green after the merge.

## Schema (migration 0005_deliveries.sql)
- `deliveries`: id, campaign_id, recipient_id, channel, payload jsonb, state (pending|sending|sent|failed, default
  pending), attempt_count int default 0, next_attempt_at default now(), lease_expires_at null, created_at, updated_at.
  UNIQUE (campaign_id, recipient_id, channel).
- Indexes: `deliveries_due (channel, next_attempt_at) WHERE state = 'pending'` (for the worker's claim) and
  `deliveries_campaign_state (campaign_id, channel, state)` (for dashboard counts).
- `campaigns.generation_started_at` (set while a Content call runs) and `campaigns.approved_at`.

## Payload shapes (what the worker sends)
- SMS: `{"to": phone, "body": rendered}`. Already transliterated for GSM-7 by content.RenderSMS.
- Email: `{"to": email, "subject", "preheader", "html"}`. html is the full rendered template (RenderEmailHTML).
- The Recipient's address is frozen in the payload, so the worker never needs to join recipients.

## API
- `POST /api/campaigns/{id}/approve`:
  - 200 `{id, status: "approved", deliveries: {sms: n, email: n}}`. Only enabled Channels appear in deliveries.
  - 422 `{error, reasons: [{code, channel?, message}]}`. Codes are `generating` (this is then the only reason),
    `no_channel`, `invalid_content` (missing or invalid, re-validated in the tx), and `empty_audience`.
  - 409 when the Campaign is not a draft, which covers approving twice or concurrently. 404 for an unknown id.
- Create Campaign now also returns `enabled_channels` and `disabled_channels` ({channel: explanation}).
  Channels with an empty Audience start off, stay off after content.done, and `PUT channels/{c} {enabled:true}`
  answers 409 with the explanation.
- `httpapi.beginGeneration(ctx, id) (end func(), err)` in generation.go marks a generation call. Content generation
  uses it. Segment regeneration (#10) should use it too. A mark older than 5 min counts as stale.

## For #12 (worker + dashboard)
- Approval leaves the Campaign in `approved`. The worker should move it to `sending` on the first claim, or claim
  Deliveries of `approved`/`sending` Campaigns and skip `cancelled` ones.
- Nothing yet exposes Delivery counts over HTTP. There is no GET campaign endpoint either.
… (8 more lines)
```
</details>

**Bash** — Read glossary, ADRs, Makefile, compose

```bash
ls; cat GLOSSARY.md | head -80; ls docs/adr; cat docs/adr/0001* docs/adr/0006*; cat Makefile; cat docker-compose*.yml
```

**Bash** — List project files

```bash
find . -path ./web/node_modules -prune -o -path ./.git -prune -o -type f -print | grep -v ai-sessions | sort
```

<details><summary>Result</summary>

```
./.claude/skills/export-session/SKILL.md
./.env.example
./.github/workflows/ci.yml
./.gitignore
./CLAUDE.md
./DECISIONS.md
./GLOSSARY.md
./Makefile
./backend/.dockerignore
./backend/Dockerfile
./backend/cmd/campaign/main.go
./backend/cmd/provider/main.go
./backend/go.mod
./backend/go.sum
./backend/internal/app/app.go
./backend/internal/app/app_test.go
./backend/internal/config/config.go
./backend/internal/config/config_test.go
./backend/internal/content/content.go
./backend/internal/content/email.go
./backend/internal/content/email.html
./backend/internal/content/partial_test.go
./backend/internal/content/sms.go
./backend/internal/content/sms_test.go
./backend/internal/db/db.go
./backend/internal/db/migrations/0001_recipients_events.sql
./backend/internal/db/migrations/0002_campaigns.sql
./backend/internal/db/migrations/0003_tr_fold.sql
./backend/internal/db/migrations/0004_content.sql
./backend/internal/db/migrations/0005_deliveries.sql
./backend/internal/httpapi/api_test.go
./backend/internal/httpapi/approve.go
./backend/internal/httpapi/approve_guards_test.go
./backend/internal/httpapi/approve_test.go
./backend/internal/httpapi/campaigns.go
./backend/internal/httpapi/campaigns_test.go
./backend/internal/httpapi/content.go
./backend/internal/httpapi/content_test.go
./backend/internal/httpapi/edit.go
./backend/internal/httpapi/edit_test.go
… (54 more lines)
```
</details>

<details><summary>Result</summary>

```
CLAUDE.md
DECISIONS.md
GLOSSARY.md
Makefile
ai-sessions
backend
docker-compose.yml
docs
scripts
web
# Smart Campaign Composer

A tool where a Marketer describes a campaign in natural language, Claude proposes the audience and channel content, the Marketer approves, and the system delivers to (fake) Recipients while showing progress live.

## People

**Marketer**:
The human operating the tool: writes the Brief, reviews Claude's proposal, edits and approves the Campaign.
_Avoid_: User, operator, customer

**Recipient**:
A (fake, seeded) end person who may receive messages; has profile data, per-channel consent, and a history of Events.
_Avoid_: User, customer, contact, subscriber

**Event**:
A recorded behaviour of a Recipient at a point in time, such as adding to cart or purchasing.
_Avoid_: Activity, action, log

## Campaign

**Brief**:
The Marketer's free-text description of what the campaign should do and to whom.
_Avoid_: Prompt, request, input

**Campaign**:
The unit of work that ties together one Brief, its Segment Rule, its Content per channel, and its lifecycle from draft to completion.
_Avoid_: Job, blast, send

**Segment Rule**:
The structured, validated definition of who a Campaign targets, expressed over Recipient fields and Events.
… (172 more lines)
```
</details>

**Skill** `mattpocock-skills:tdd`

_(skill instructions loaded: skills)_

The spec and caller already fix the seams (HTTP API with worker in-process and real mock Provider; Vitest for the dashboard). Reading the code.

**Bash** — Read entrypoints, config, testenv, migrations

```bash
cd backend; cat cmd/campaign/main.go cmd/provider/main.go internal/app/app.go internal/config/config.go internal/testenv/testenv.go internal/db/migrations/0005_deliveries.sql internal/db/migrations/0002_campaigns.sql
```

<details><summary>Result</summary>

```
// Command campaign runs the API (`campaign api`) or the Delivery worker
// (`campaign worker`).
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"campaigncomposer/internal/app"
	"campaigncomposer/internal/config"
	"campaigncomposer/internal/db"
	"campaigncomposer/internal/httpapi"
	"campaigncomposer/internal/llm"
)

func main() {
	if len(os.Args) != 2 || (os.Args[1] != "api" && os.Args[1] != "worker") {
		fmt.Fprintln(os.Stderr, "usage: campaign api|worker")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg := config.FromEnv()
	var err error
	if os.Args[1] == "api" {
		err = runAPI(ctx, cfg)
	} else {
		err = runWorker(ctx, cfg)
	}
	if err != nil {
		log.Fatal(err)
	}
… (274 more lines)
```
</details>

**Bash** — Read server, approve, test helpers

```bash
cd backend/internal/httpapi; cat server.go approve.go approve_test.go; sed -n 1,200p api_test.go
```

<details><summary>Result</summary>

```
// Package httpapi is the HTTP surface of the api process.
package httpapi

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"campaigncomposer/internal/llm"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Options configures the handler.
type Options struct {
	// MockLLM is true when no Anthropic API key is configured and the scripted
	// mock LLM stands in for Claude.
	MockLLM bool
	Model   string
	// LLM answers the Segment call; nil means the scripted mock.
	LLM llm.LLM
}

type server struct {
	pool *pgxpool.Pool
	opts Options
}

// NewHandler returns the API's HTTP handler.
func NewHandler(pool *pgxpool.Pool, opts Options) http.Handler {
	if opts.LLM == nil {
		opts.LLM = llm.NewMock()
	}
	s := &server{pool: pool, opts: opts}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("GET /api/overview", s.overview)
	mux.HandleFunc("POST /api/campaigns", s.createCampaign)
… (648 more lines)
```
</details>

**Bash** — List functions and SSE usage

```bash
cd backend/internal/httpapi; grep -n "^func \|text/event-stream\|Flush" *.go | head -80; cat ../../go.mod
```

<details><summary>Result</summary>

```
approve.go:30:func (s *server) approveCampaign(w http.ResponseWriter, r *http.Request) {
approve.go:65:func (*notFoundError) Error() string { return "campaign not found" }
approve.go:69:func (e *conflictError) Error() string { return e.message }
approve.go:80:func (e *blockedError) Error() string { return fmt.Sprintf("approval blocked: %+v", e.reasons) }
approve.go:82:func approve(ctx context.Context, tx pgx.Tx, id int64) (approvalResult, error) {
approve.go:164:func invalidMessage(channel string, v content.Validation) string {
approve.go:173:func storedContent(ctx context.Context, tx pgx.Tx, id int64, enabled []string) (content.Proposal, error) {
approve.go:210:func insertDeliveries(ctx context.Context, tx pgx.Tx, id int64, channel string, p content.Proposal, members []segment.Member) (int, error) {
approve_test.go:32:func approve(t *testing.T, baseURL string, id int64) (int, approval) {
approve_test.go:41:func reviewedCampaign(t *testing.T) (baseURL string, pool *pgxpool.Pool, c campaign) {
approve_test.go:49:func deliveryCount(t *testing.T, pool *pgxpool.Pool, id int64, channel string) int {
approve_test.go:56:func exec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
approve_test.go:63:func TestApprovalFreezesTheAudienceIntoOnePendingDeliveryPerRecipientAndChannel(t *testing.T) {
approve_test.go:98:func TestDeliveriesCarryTheContentRenderedForTheirRecipient(t *testing.T) {
approve_test.go:137:func audienceMember(t *testing.T, pool *pgxpool.Pool, consent string) int64 {
approve_test.go:147:func TestConsentWithdrawnAfterReviewExcludesTheRecipient(t *testing.T) {
approve_test.go:165:func TestRecipientChangesAfterApprovalDoNotChangeTheDeliveries(t *testing.T) {
approve_test.go:192:func TestApprovingTwiceCreatesNoExtraDeliveries(t *testing.T) {
approve_test.go:208:func TestConcurrentApprovalsCreateEachDeliveryOnce(t *testing.T) {
api_test.go:20:func startAPI(t *testing.T, opts httpapi.Options) *httptest.Server {
api_test.go:28:func startAPIWithPool(t *testing.T, opts httpapi.Options) (*httptest.Server, *pgxpool.Pool) {
api_test.go:41:func queryRow(t *testing.T, pool *pgxpool.Pool, sql string, args []any, dest ...any) {
api_test.go:48:func getJSON(t *testing.T, url string, into any) int {
api_test.go:61:func TestHealthzIsHealthyWhenPostgresIsReachable(t *testing.T) {
api_test.go:81:func TestOverviewReportsSeededRecipientsAndMockLLM(t *testing.T) {
api_test.go:99:func TestOverviewReportsRealLLMWhenNotMocked(t *testing.T) {
api_test.go:109:func TestHealthzIsUnhealthyWhenPostgresIsUnreachable(t *testing.T) {
approve_guards_test.go:19:func assertBlocked(t *testing.T, pool *pgxpool.Pool, id int64, code int, a approval, want ...string) {
approve_guards_test.go:47:func TestApprovalIsRefusedWhileAnEnabledChannelHasInvalidContent(t *testing.T) {
approve_guards_test.go:57:func TestApprovalIsRefusedWhenAnEnabledChannelHasNoContent(t *testing.T) {
approve_guards_test.go:69:func TestApprovalIsRefusedWhenAnEnabledChannelsAudienceHasBecomeEmpty(t *testing.T) {
approve_guards_test.go:78:func TestApprovalIsRefusedWithNoChannelEnabled(t *testing.T) {
approve_guards_test.go:90:func TestApprovalListsEveryReasonItIsRefused(t *testing.T) {
approve_guards_test.go:101:func TestAChannelWithNoConsentingRecipientsIsDisabledWithAnExplanation(t *testing.T) {
approve_guards_test.go:135:func TestApprovalIsRefusedWhileContentIsBeingGenerated(t *testing.T) {
content_test.go:24:// readSSE reads a whole text/event-stream response into its events.
content_test.go:25:func readSSE(t *testing.T, r io.Reader) []sseEvent {
content_test.go:85:func createCampaign(t *testing.T, baseURL, brief string) campaign {
content_test.go:95:func streamContent(t *testing.T, baseURL string, id int64) []sseEvent {
content_test.go:105:	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
… (73 more lines)
```
</details>

**Bash** — Read SSE test helpers and writer

```bash
cd backend/internal/httpapi; sed -n 1,140p content_test.go; sed -n 220,260p content.go; grep -n "postJSON" -A20 campaigns_test.go | head -40
```

<details><summary>Result</summary>

```
package httpapi_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"

	"campaigncomposer/internal/httpapi"
	"campaigncomposer/internal/llm"
)

type sseEvent struct {
	Name string
	Data json.RawMessage
}

// readSSE reads a whole text/event-stream response into its events.
func readSSE(t *testing.T, r io.Reader) []sseEvent {
	t.Helper()
	var events []sseEvent
	var cur sseEvent
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			if cur.Name != "" || cur.Data != nil {
				events = append(events, cur)
			}
			cur = sseEvent{}
		case strings.HasPrefix(line, "event: "):
			cur.Name = strings.TrimPrefix(line, "event: ")
… (176 more lines)
```
</details>

**Bash** — Read frontend api, store, approval panel

```bash
cd web; cat package.json vite.config.ts src/api.ts src/stores/campaign.ts src/components/ApprovalPanel.vue src/components/ApprovalPanel.test.ts src/App.vue nginx.conf
```

<details><summary>Result</summary>

```
<persisted-output>
Output too large (29.7KB). Full output saved to: ~/.claude/projects/-Users-[user]-Documents-Projects-smart-campaign-composer/e746afbf-7d53-430f-9572-d2e7f36dedd2/tool-results/bx22bbjgl.txt

Preview (first 2KB):
{
  "name": "smart-campaign-composer-web",
  "private": true,
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "vue-tsc --noEmit && vite build",
    "typecheck": "vue-tsc --noEmit",
    "test": "vitest run"
  },
  "dependencies": {
    "pinia": "^3.0.3",
    "vue": "^3.5.17"
  },
  "devDependencies": {
    "@tailwindcss/vite": "^4.1.11",
    "@types/node": "^22.15.34",
    "@vitejs/plugin-vue": "^5.2.4",
    "@vue/test-utils": "^2.4.6",
    "jsdom": "^26.1.0",
    "tailwindcss": "^4.1.11",
    "typescript": "^5.8.3",
    "vite": "^6.3.5",
    "vitest": "^3.2.4",
    "vue-tsc": "^2.2.12"
  }
}
/// <reference types="vitest/config" />
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [vue(), tailwindcss()],
  server: {
    // In development, proxy the API like nginx does in the container.
… (43 more lines)
```
</details>

**Bash** — Read rest of api.ts, ApprovalPanel, App

```bash
cd web; sed -n 40,400p src/api.ts; cat src/components/ApprovalPanel.vue src/App.vue nginx.conf
```

<details><summary>Result</summary>

```
) {
    super(message)
  }
}

export async function createCampaign(brief: string): Promise<Campaign> {
  const res = await fetch('/api/campaigns', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ brief }),
  })
  const body = await res.json().catch(() => null)
  if (!res.ok) throw new ApiError(body?.error ?? `POST /api/campaigns: ${res.status}`, body?.retryable === true)
  return body
}

export type Channel = 'sms' | 'email'
export const channels: Channel[] = ['sms', 'email']

export interface SmsContent {
  body: string
}

export interface EmailContent {
  subject: string
  preheader: string
  body: string
  cta_text?: string
}

export interface Content {
  sms?: SmsContent | null
  email?: EmailContent | null
}

// Length is how much of a field's limit Content uses once rendered with the
// longest placeholder values in the Audience; encoding is set for SMS only.
export interface Length {
  encoding?: 'gsm7' | 'ucs2'
  used: number
… (252 more lines)
```
</details>

**Bash** — Read ApprovalPanel test and composer wiring

```bash
cd web; cat src/components/ApprovalPanel.test.ts; grep -n "CampaignComposer\|ApprovalPanel" -r src --include=*.vue; sed -n 1,60p src/components/CampaignComposer.vue | head -60
```

<details><summary>Error</summary>

```
Exit code 1
import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia } from 'pinia'
import CampaignComposer from './CampaignComposer.vue'

const sms = { body: 'Merhaba {{first_name}}, sepetin seni bekliyor!' }
const email = { subject: 'İndirim', preheader: 'Sepetin hâlâ orada', body: 'Merhaba {{first_name}}', cta_text: '' }
const valid = { valid: true, issues: [] }
const invalid = { valid: false, issues: [{ field: 'body', message: 'SMS tek parçaya sığmalı.' }] }

const frame = (event: string, data: unknown) => `event: ${event}\ndata: ${JSON.stringify(data)}\n\n`

interface Options {
  audience?: { sms: number; email: number }
  // approve answers the approval request.
  approve?: () => Response
}

const json = (b: unknown, status = 200) =>
  new Response(JSON.stringify(b), { status, headers: { 'Content-Type': 'application/json' } })

function mockApi(opts: Options = {}) {
  const audience = opts.audience ?? { sms: 412, email: 1534 }
  const enabled = (['sms', 'email'] as const).filter((c) => audience[c] > 0)
  const campaign = {
    id: 7,
    status: 'draft',
    brief: 'Sepet hatırlatması gönder',
    segment: { rule: { all: [] }, labels: ['Şehir: İstanbul'], unsupported_criteria: [] },
    audience,
    enabled_channels: enabled,
    disabled_channels: Object.fromEntries(
      (['sms', 'email'] as const)
        .filter((c) => audience[c] === 0)
        .map((c) => [c, `Bu hedef kitlede ${c === 'sms' ? 'SMS' : 'E-posta'} için izin vermiş alıcı yok, bu yüzden kanal kapatıldı.`]),
    ),
  }
  let stream!: ReadableStreamDefaultController<Uint8Array>
  const body = new ReadableStream<Uint8Array>({ start: (c) => void (stream = c) })
… (180 more lines)
```
</details>

**Bash** — Read later tickets' scope

```bash
for i in 13 14 15 16; do gh issue view $i --json title,body -q '.title + "\n" + .body' | sed -n '/What to build/,/Acceptance/p'; echo ----; done
```

<details><summary>Error</summary>

```
This agent is isolated in the worktree ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-ab40c40da8c9ccbf9, but this command runs gh with the text .title + "\n" + .body inside a construct too complex to verify, so what it runs cannot be shown not to be git. Refusing to run it — a worktree-isolated agent's git operations must target its own worktree. Split it into plain, separate commands and run them from ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-ab40c40da8c9ccbf9.
```
</details>

**Bash** — Read tickets 13 and 15

```bash
gh issue view 13 | sed -n '/What to build/,$p'; gh issue view 15 | sed -n '/What to build/,$p'
```

**Bash** — Read tickets 14 and 16

```bash
gh issue view 14 | sed -n '/What to build/,$p'; gh issue view 16 | sed -n '/What to build/,$p'
```

<details><summary>Result</summary>

```
## What to build

Sending survives a misbehaving Provider, and the outcome is explainable and observable. The mock Provider gets configurable behaviour from env: 429 rate with `Retry-After`, 5xx rate, slow-response rate beyond the client timeout, latency range, and random seed. Defaults are 10% 429, 5% 5xx, 3% slow, 20–300 ms. A deterministic scripted mode for tests pins per-request outcomes. The worker retries 429 (honouring `Retry-After`), 5xx and timeouts with exponential backoff and jitter, up to 5 Attempts, then marks the Delivery `failed`. Any other 4xx is `failed` immediately. Every Attempt is recorded with status code, latency and error. Retries are scheduled through the Delivery's next-attempt time, and only Deliveries whose next-attempt time has passed are claimed. A completed Campaign shows its sent/failed breakdown. `/metrics` exposes per-Channel sent, failed and retry counters in Prometheus text format. Backoff base and client timeout are configuration values.

## Acceptance criteria

- [ ] Scripted 429 (with `Retry-After`) → 5xx → 200 results in `sent` after 3 Attempts, honouring `Retry-After`
- [ ] 5 retryable failures in a row result in `failed`
- [ ] A 400-class error other than 429 results in `failed` after 1 Attempt
- [ ] Each Attempt is recorded with status code, latency and error
- [ ] A Campaign with some failed Deliveries still reaches `completed`, and the dashboard shows the breakdown
- [ ] `/metrics` returns Prometheus text with per-Channel sent, failed and retry counters that match the Deliveries
- [ ] Provider failure behaviour is configurable via env and documented in `.env.example`
- [ ] Tests run in milliseconds by configuring small backoff and timeout values (no clock seam)

## What to build

Prove "the same message never goes to the same person twice" under the failures that usually break it (ADR-0006). Claiming a Delivery sets a lease expiry. A Delivery stuck in `sending` past its lease (e.g. its worker crashed) is reclaimed by another worker. Because every request carries the idempotency key, a second Attempt for a message the Provider already accepted is harmless. Lease duration is a configuration value.

## Acceptance criteria

- [ ] Scripted "Provider accepts, then the response is slower than the client timeout" → the retry gets the original result, the Delivery is `sent`, and the Provider recorded exactly one message
- [ ] A worker killed mid-send leaves Deliveries in `sending`; after the lease expires, another worker completes them
- [ ] Restarting the worker container during a Campaign under the default Provider failure rates ends with every Delivery `sent` or `failed`, and zero duplicate messages recorded by the Provider
- [ ] Lease duration is configurable, so these tests run fast
```
</details>

<details><summary>Result</summary>

```
## What to build

Each Channel sends at no more than 100 messages per second. One token bucket per Channel is shared by that Channel's sending in the worker process. On a burst of 429s, the bucket's rate halves and then recovers gradually towards 100/s. Horizontal scaling (a distributed limiter) is out of scope.

## Acceptance criteria

- [ ] Sending a few thousand Deliveries never exceeds 100 Provider requests per second per Channel (measured on the Provider side)
- [ ] SMS and email limits are independent: a busy SMS Channel doesn't slow email
- [ ] A burst of 429s lowers the effective rate, and it recovers once 429s stop
- [ ] The limit is configurable, so tests can use a small limit and run quickly

## What to build

The Marketer can stop a Campaign. Cancelling is allowed from `draft`, `approved` and `sending`, and moves the Campaign to `cancelled`. Once cancelled, the worker claims no more of its `pending` Deliveries; Deliveries already in flight finish normally. The dashboard reflects the cancellation and the final counts.

## Acceptance criteria

- [ ] Cancel is available in the UI for `draft`, `approved` and `sending` Campaigns, and not for `completed` or `cancelled` ones
- [ ] Cancelling a `draft` or `approved` Campaign means nothing is sent
- [ ] Cancelling mid-send stops new claims; in-flight Deliveries finish, and the remaining ones stay `pending`
- [ ] The dashboard shows the Campaign as cancelled, with its final per-Channel counts
- [ ] HTTP-level test: cancel during sending → the Provider receives no requests for Deliveries still `pending` at cancel time
```
</details>

Seams are fixed by the spec. First slice: the mock Provider deduplicates on `Idempotency-Key`.

**Write** `~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-ab40c40da8c9ccbf9/backend/internal/provider/provider_test.go` (102 lines)

**Bash** — Run provider tests (expect red)

```bash
cd backend && go test ./internal/provider/ 2>&1 | tail -5
```

<details><summary>Result</summary>

```
campaigncomposer/internal/provider: no non-test Go files in ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-ab40c40da8c9ccbf9/backend/internal/provider
FAIL	campaigncomposer/internal/provider [build failed]
FAIL
```
</details>

**Write** `~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-ab40c40da8c9ccbf9/backend/internal/provider/provider.go` (108 lines)

**Write** `~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-ab40c40da8c9ccbf9/backend/cmd/provider/main.go` (37 lines)

**Bash** — Run provider tests

```bash
cd backend && go test ./internal/provider/ ./cmd/... 2>&1 | tail -5
```

<details><summary>Result</summary>

```
ok  	campaigncomposer/internal/provider	0.255s
?   	campaigncomposer/cmd/campaign	[no test files]
?   	campaigncomposer/cmd/provider	[no test files]
```
</details>

Provider green. Next slice: the end-to-end drain through the HTTP API with an in-process worker.

**Write** `~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-ab40c40da8c9ccbf9/backend/internal/httpapi/sending_test.go` (242 lines)

**Bash** — Tidy test file

```bash
cd backend/internal/httpapi && python3 - <<'EOF'
p='sending_test.go'
s=open(p).read()
s=s.replace("	payloads := map[string]json.RawMessage{}\n","").replace("	_ = payloads\n","")
s=s.replace("startAPI(t, httpOptions())","startAPI(t, httpapi.Options{})")
s=s.replace('"campaigncomposer/internal/provider"','"campaigncomposer/internal/httpapi"\n\t"campaigncomposer/internal/provider"')
s=s.replace('import (\n\t"context"\n\t"encoding/json"\n','import (\n\t"context"\n')
open(p,'w').write(s)
EOF
```

<details><summary>Result</summary>

```
(Bash completed with no output)
```
</details>

Now the worker.

**Write** `~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-ab40c40da8c9ccbf9/backend/internal/worker/worker.go` (248 lines)

Now `GET /api/campaigns/{id}` with Delivery counts.

**Write** `~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-ab40c40da8c9ccbf9/backend/internal/httpapi/dashboard.go` (102 lines)

**Bash** — Register route and run sending tests

```bash
cd backend/internal/httpapi && python3 - <<'EOF'
p='dashboard.go'
s=open(p).read()
s=s.replace('sprintf(progressQuery','fmt.Sprintf(progressQuery')
s=s.replace('"context"\n\t"errors"\n','"context"\n\t"fmt"\n')
s=s.replace('var errNoCampaign = errors.New("campaign not found")\n\n','')
open(p,'w').write(s)
p='server.go'
s=open(p).read()
s=s.replace('	mux.HandleFunc("POST /api/campaigns/{id}/content", s.generateContent)\n','	mux.HandleFunc("GET /api/campaigns/{id}", s.getCampaign)\n	mux.HandleFunc("POST /api/campaigns/{id}/content", s.generateContent)\n')
open(p,'w').write(s)
EOF
cd ../.. && TEST_DATABASE_URL='postgres://campaign:campaign@localhost:55432/campaign?sslmode=disable' COMPOSE_PROJECT_NAME=smart-campaign-composer docker compose up -d --wait postgres 2>&1 | tail -1; TEST_DATABASE_URL='postgres://campaign:campaign@localhost:55432/campaign?sslmode=disable' go test -count=1 -run 'Sent|Sending|Concurrent|Unknown|Frozen' ./internal/httpapi/ 2>&1 | tail -20
```

<details><summary>Result</summary>

```
Container smart-campaign-composer-postgres-1  Healthy
ok  	campaigncomposer/internal/httpapi	11.667s
```
</details>

Green. Next slice: the dashboard SSE stream.

**Write** `~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-ab40c40da8c9ccbf9/backend/internal/httpapi/dashboard_test.go` (135 lines)

**Bash** — Implement dashboard SSE and run its tests

```bash
cd backend/internal/httpapi && python3 - <<'EOF'
p='server.go'
s=open(p).read()
s=s.replace('''	LLM llm.LLM
}''','''	LLM llm.LLM
	// DashboardInterval is how often the dashboard stream re-reads the
	// Delivery counts; zero means 500ms.
	DashboardInterval time.Duration
}''')
s=s.replace('''		opts.LLM = llm.NewMock()
	}''','''		opts.LLM = llm.NewMock()
	}
	if opts.DashboardInterval <= 0 {
		opts.DashboardInterval = 500 * time.Millisecond
	}''')
s=s.replace('	mux.HandleFunc("GET /api/campaigns/{id}", s.getCampaign)\n','	mux.HandleFunc("GET /api/campaigns/{id}", s.getCampaign)\n	mux.HandleFunc("GET /api/dashboard", s.dashboard)\n')
open(p,'w').write(s)
p='dashboard.go'
s=open(p).read()
s=s.replace('"context"\n\t"fmt"\n','"bytes"\n\t"context"\n\t"encoding/json"\n\t"fmt"\n\t"log"\n')
s=s.replace('"strconv"\n','"strconv"\n\t"time"\n')
s+='''
// dashboardLimit is how many of the newest non-draft Campaigns the dashboard shows.
const dashboardLimit = 20

// dashboard streams the per-Channel Delivery counts and progress of the
// newest non-draft Campaigns as `dashboard` events: one when the stream opens
// and another whenever a count or status changes. It runs until the client
// goes away.
func (s *server) dashboard(w http.ResponseWriter, r *http.Request) {
	sse, ok := startSSE(w)
	if !ok {
		writeError(w, fmt.Errorf("dashboard: streaming unsupported"))
		return
	}
	ctx := r.Context()
	tick := time.NewTicker(s.opts.DashboardInterval)
	defer tick.Stop()
	var last []byte
	for {
		cs, err := campaignsProgress(ctx, s.pool, fmt.Sprintf(
			"c.id IN (SELECT id FROM campaigns WHERE status <> 'draft' ORDER BY id DESC LIMIT %d)", dashboardLimit))
		if err != nil && ctx.Err() == nil {
			log.Printf("dashboard: %v", err)
		}
		if err == nil {
			if cs == nil {
				cs = []campaignProgress{}
			}
			b, _ := json.Marshal(map[string]any{"campaigns": cs})
			if !bytes.Equal(b, last) {
				sse.send("dashboard", json.RawMessage(b))
				last = b
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
'''
open(p,'w').write(s)
EOF
grep -n '"time"' server.go; cd ../.. && TEST_DATABASE_URL='postgres://campaign:campaign@localhost:55432/campaign?sslmode=disable' go test -count=1 -run 'Dashboard' ./internal/httpapi/ 2>&1 | tail -20
```

<details><summary>Result</summary>

```
9:	"time"
ok  	campaigncomposer/internal/httpapi	2.941s
```
</details>

Now wire the worker into `campaign worker`.

**Bash** — Wire worker into main and vet

```bash
cd backend && python3 - <<'EOF'
p='cmd/campaign/main.go'
s=open(p).read()
old=s[s.index('// runWorker connects'):]
s=s.replace(old,'''// runWorker claims and sends Deliveries until it is stopped. The api process
// owns migrations, so the worker starts only once the api is healthy.
func runWorker(ctx context.Context, cfg config.Config) error {
	pool, err := db.Connect(ctx, cfg.DatabaseURL, 60*time.Second)
	if err != nil {
		return err
	}
	defer pool.Close()
	log.Printf("worker sending to the Provider at %s", cfg.ProviderURL)
	return worker.New(pool, worker.Options{ProviderURL: cfg.ProviderURL}).Run(ctx)
}
''')
s=s.replace('"campaigncomposer/internal/llm"\n','"campaigncomposer/internal/llm"\n\t"campaigncomposer/internal/worker"\n')
open(p,'w').write(s)
EOF
go build ./... && go vet ./...
```

<details><summary>Result</summary>

```
(Bash completed with no output)
```
</details>

Backend done. Now the dashboard component, test first.

**Write** `~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-ab40c40da8c9ccbf9/web/src/components/CampaignDashboard.test.ts` (129 lines)

**Bash** — Run dashboard test (expect red)

```bash
cd web && npx vitest run src/components/CampaignDashboard.test.ts 2>&1 | tail -8
```

<details><summary>Result</summary>

```
at #resolve (node:internal/modules/esm/loader:686:17)
    at ModuleLoader.getOrCreateModuleJob (node:internal/modules/esm/loader:606:35)
    at ModuleJob.syncLink (node:internal/modules/esm/module_job:163:33) {
  code: 'ERR_MODULE_NOT_FOUND'
}
```
</details>

**Bash** — Inspect test error

```bash
cd web && ls node_modules | head -3; npx vitest run src/components/CampaignDashboard.test.ts 2>&1 | head -15
```

<details><summary>Result</summary>

```
ls: node_modules: No such file or directory
vite.config.ts (2:29) [33m[UNRESOLVED_IMPORT] [0mCould not resolve 'vite' in vite.config.ts
   [38;5;246m╭[0m[38;5;246m─[0m[38;5;246m[[0m vite.config.ts:2:30 [38;5;246m][0m
   [38;5;246m│[0m
 [38;5;246m2 │[0m [38;5;249mi[0m[38;5;249mm[0m[38;5;249mp[0m[38;5;249mo[0m[38;5;249mr[0m[38;5;249mt[0m[38;5;249m [0m[38;5;249m{[0m[38;5;249m [0m[38;5;249md[0m[38;5;249me[0m[38;5;249mf[0m[38;5;249mi[0m[38;5;249mn[0m[38;5;249me[0m[38;5;249mC[0m[38;5;249mo[0m[38;5;249mn[0m[38;5;249mf[0m[38;5;249mi[0m[38;5;249mg[0m[38;5;249m [0m[38;5;249m}[0m[38;5;249m [0m[38;5;249mf[0m[38;5;249mr[0m[38;5;249mo[0m[38;5;249mm[0m[38;5;249m [0m'vite'
 [38;5;240m  │[0m                              ───┬──  
 [38;5;240m  │[0m                                 ╰──── Module not found, treating it as an external dependency
[38;5;246m───╯[0m

vite.config.ts (3:16) [33m[UNRESOLVED_IMPORT] [0mCould not resolve '@vitejs/plugin-vue' in vite.config.ts
   [38;5;246m╭[0m[38;5;246m─[0m[38;5;246m[[0m vite.config.ts:3:17 [38;5;246m][0m
   [38;5;246m│[0m
 [38;5;246m3 │[0m [38;5;249mi[0m[38;5;249mm[0m[38;5;249mp[0m[38;5;249mo[0m[38;5;249mr[0m[38;5;249mt[0m[38;5;249m [0m[38;5;249mv[0m[38;5;249mu[0m[38;5;249me[0m[38;5;249m [0m[38;5;249mf[0m[38;5;249mr[0m[38;5;249mo[0m[38;5;249mm[0m[38;5;249m [0m'@vitejs/plugin-vue'
 [38;5;240m  │[0m                 ──────────┬─────────  
 [38;5;240m  │[0m                           ╰─────────── Module not found, treating it as an external dependency
[38;5;246m───╯[0m
```
</details>

**Bash** — Install deps and run dashboard test

```bash
cd web && npm ci --silent 2>&1 | tail -3; npx vitest run src/components/CampaignDashboard.test.ts 2>&1 | tail -8
```

<details><summary>Result</summary>

```
⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯[1/1]⎯


 Test Files  1 failed (1)
      Tests  no tests
   Start at  15:42:20
   Duration  530ms (transform 12ms, setup 0ms, collect 0ms, tests 0ms, environment 145ms, prepare 46ms)
```
</details>

Red (component missing). Adding the API types and the component.

**Bash** — Add dashboard component and run tests

```bash
cd web && cat >> src/api.ts <<'EOF'

// ChannelCounts is a Channel's Deliveries by state; pending includes those in flight.
export interface ChannelCounts {
  pending: number
  sent: number
  failed: number
  total: number
}

// CampaignProgress is one Campaign on the live dashboard.
export interface CampaignProgress {
  id: number
  brief: string
  status: string
  deliveries: Partial<Record<Channel, ChannelCounts>>
  // progress is the share of Deliveries with an outcome, from 0 to 1.
  progress: number
}

export interface DashboardEvent {
  campaigns: CampaignProgress[]
}

// subscribeDashboard opens the dashboard SSE stream and hands every event to
// onEvent. EventSource reconnects on its own; the returned function closes it.
export function subscribeDashboard(onEvent: (e: DashboardEvent) => void): () => void {
  const source = new EventSource('/api/dashboard')
  source.addEventListener('dashboard', (e) => onEvent(JSON.parse((e as MessageEvent).data)))
  return () => source.close()
}
EOF
cat > src/components/CampaignDashboard.vue <<'EOF'
<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { channels, subscribeDashboard, type CampaignProgress, type Channel } from '../api'

const campaigns = ref<CampaignProgress[]>([])
let close: (() => void) | null = null

onMounted(() => {
  close = subscribeDashboard((e) => (campaigns.value = e.campaigns))
})
onUnmounted(() => close?.())

const statusNames: Record<string, string> = {
  approved: 'Onaylandı',
  sending: 'Gönderiliyor',
  completed: 'Tamamlandı',
  cancelled: 'İptal edildi',
}
const channelNames: Record<Channel, string> = { sms: 'SMS', email: 'E-posta' }

const percent = (c: CampaignProgress) => Math.round(c.progress * 100)
const formatCount = (n: number) => n.toLocaleString('tr-TR')
const sentOn = (c: CampaignProgress) => channels.filter((ch) => c.deliveries[ch])
</script>

<template>
  <section v-if="campaigns.length" data-test="dashboard" class="space-y-4 rounded-lg border border-slate-200 bg-white p-6">
    <h2 class="text-sm font-medium text-slate-500">Gönderim durumu</h2>
    <article
      v-for="c in campaigns"
      :key="c.id"
      :data-test="`dashboard-campaign-${c.id}`"
      class="space-y-3 border-t border-slate-100 pt-4 first-of-type:border-t-0 first-of-type:pt-0"
    >
      <div class="flex items-baseline justify-between gap-4">
        <p class="truncate text-sm text-slate-700" :title="c.brief">#{{ c.id }} · {{ c.brief }}</p>
        <span class="shrink-0 rounded-full bg-slate-100 px-2 py-0.5 text-xs font-medium text-slate-700">
          {{ statusNames[c.status] ?? c.status }}
        </span>
      </div>
      <div
        data-test="progress"
        role="progressbar"
        aria-valuemin="0"
        aria-valuemax="100"
        :aria-valuenow="percent(c)"
        class="h-2 overflow-hidden rounded-full bg-slate-100"
      >
        <div class="h-full bg-emerald-600 transition-all" :style="{ width: `${percent(c)}%` }" />
      </div>
      <dl class="grid gap-2 text-sm sm:grid-cols-2">
        <div v-for="ch in sentOn(c)" :key="ch" :data-test="`counts-${ch}`" class="rounded-md bg-slate-50 p-3">
          <dt class="font-medium text-slate-700">{{ channelNames[ch] }} · %{{ percent(c) }}</dt>
          <dd class="mt-1 flex flex-wrap gap-x-4 text-slate-600">
            <span>Bekleyen {{ formatCount(c.deliveries[ch]!.pending) }}</span>
            <span class="text-emerald-700">Gönderildi {{ formatCount(c.deliveries[ch]!.sent) }}</span>
            <span class="text-red-700">Başarısız {{ formatCount(c.deliveries[ch]!.failed) }}</span>
          </dd>
        </div>
      </dl>
    </article>
  </section>
</template>
EOF
npx vitest run src/components/CampaignDashboard.test.ts 2>&1 | tail -15
```

<details><summary>Error</summary>

```
This agent is isolated in the worktree ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-ab40c40da8c9ccbf9, but this command is too complex to verify that it stays inside the worktree. Refusing to run it — a worktree-isolated agent's git operations must target its own worktree. Split it into plain, separate commands and run them from ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-ab40c40da8c9ccbf9.
```
</details>

**Read** `~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-ab40c40da8c9ccbf9/web/src/api.ts`

**Read** `~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-ab40c40da8c9ccbf9/web/src/api.ts`

<details><summary>Result</summary>

```
210	  ) {
211	    super(message)
212	  }
213	}
214	
215	// approveCampaign freezes the Campaign's Audience into Deliveries.
216	export async function approveCampaign(id: number): Promise<Approval> {
217	  const res = await fetch(`/api/campaigns/${id}/approve`, { method: 'POST' })
218	  const body = await res.json().catch(() => null)
219	  if (res.status === 422) throw new ApprovalRefused(body?.error ?? 'Kampanya onaylanamaz.', body?.reasons ?? [])
220	  if (!res.ok) throw new ApiError(body?.error ?? `POST approve: ${res.status}`)
221	  return body
222	}
223
```
</details>

**Edit** `~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-ab40c40da8c9ccbf9/web/src/api.ts`

**Write** `~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-ab40c40da8c9ccbf9/web/src/components/CampaignDashboard.vue` (70 lines)

**Bash** — Run dashboard tests

```bash
cd web && npx vitest run src/components/CampaignDashboard.test.ts 2>&1 | tail -15
```

<details><summary>Result</summary>

```
(node:76538) [DEP0205] DeprecationWarning: `module.register()` is deprecated. Use `module.registerHooks()` instead.
(Use `node --trace-deprecation ...` to show where the warning was created)

 RUN  v3.2.4 ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-ab40c40da8c9ccbf9/web

 ✓ src/components/CampaignDashboard.test.ts (7 tests) 54ms

 Test Files  1 passed (1)
      Tests  7 passed (7)
   Start at  15:43:00
   Duration  722ms (transform 71ms, setup 0ms, collect 100ms, tests 54ms, environment 260ms, prepare 60ms)
```
</details>

Mount it in the app.

**Bash** — Mount dashboard and run all web tests and typecheck

```bash
cd web && python3 - <<'EOF'
p='src/App.vue'
s=open(p).read()
s=s.replace("import CampaignComposer from './components/CampaignComposer.vue'\n","import CampaignComposer from './components/CampaignComposer.vue'\nimport CampaignDashboard from './components/CampaignDashboard.vue'\n")
s=s.replace('      <CampaignComposer v-if="!store.failed" />\n','      <CampaignComposer v-if="!store.failed" />\n      <CampaignDashboard v-if="!store.failed" />\n')
open(p,'w').write(s)
EOF
grep -n Dashboard src/App.vue; npx vitest run 2>&1 | tail -15; npx vue-tsc --noEmit 2>&1 | tail
```

<details><summary>Result</summary>

```
4:import CampaignDashboard from './components/CampaignDashboard.vue'
42:      <CampaignDashboard v-if="!store.failed" />
 ❯ callWithAsyncErrorHandling node_modules/@vue/runtime-core/dist/runtime-core.cjs.js:207:17
 ❯ hook.__weh.hook.__weh node_modules/@vue/runtime-core/dist/runtime-core.cjs.js:2798:19
 ❯ flushPostFlushCbs node_modules/@vue/runtime-core/dist/runtime-core.cjs.js:382:28
 ❯ render node_modules/@vue/runtime-core/dist/runtime-core.cjs.js:5988:7
 ❯ mount node_modules/@vue/runtime-core/dist/runtime-core.cjs.js:3940:13
 ❯ Object.app.mount node_modules/@vue/runtime-dom/dist/runtime-dom.cjs.js:1711:19

⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯[1/4]⎯


 Test Files  1 failed | 4 passed (5)
      Tests  4 failed | 41 passed (45)
   Start at  15:43:08
   Duration  935ms (transform 260ms, setup 0ms, collect 844ms, tests 556ms, environment 1.18s, prepare 251ms)

src/components/CampaignDashboard.test.ts(112,28): error TS2339: Property 'exists' does not exist on type 'Omit<DOMWrapper<Element>, "exists">'.
```
</details>

**Bash** — Read App test setup

```bash
cd web && sed -n 1,40p src/App.test.ts
```

<details><summary>Result</summary>

```
import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import App from './App.vue'

function mockOverview(body: unknown, status = 200) {
  const fetchMock = vi.fn().mockResolvedValue(
    new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } }),
  )
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

async function mountApp() {
  const wrapper = mount(App, { global: { plugins: [createPinia()] } })
  await flushPromises()
  return wrapper
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('App', () => {
  it('shows the Recipient count from the API', async () => {
    const fetchMock = mockOverview({ recipient_count: 5000, llm: { mock: true, model: 'claude-sonnet-5-5' } })

    const wrapper = await mountApp()

    expect(fetchMock).toHaveBeenCalledWith('/api/overview')
    expect(wrapper.get('[data-test="recipient-count"]').text()).toBe('5.000')
  })

  it('shows the Mock LLM badge when the API runs on the mock LLM', async () => {
    mockOverview({ recipient_count: 5000, llm: { mock: true, model: 'claude-sonnet-5-5' } })

    const wrapper = await mountApp()

    expect(wrapper.get('[data-test="mock-llm-badge"]').text()).toBe('Mock LLM')
  })
```
</details>

**Bash** — Stub EventSource in App test; rerun

```bash
cd web && python3 - <<'EOF'
p='src/App.test.ts'
s=open(p).read()
s=s.replace('''async function mountApp() {''','''// jsdom has no EventSource; the dashboard's stream stays silent here.
class SilentEventSource {
  addEventListener() {}
  close() {}
}

async function mountApp() {
  vi.stubGlobal('EventSource', SilentEventSource)''')
open(p,'w').write(s)
p='src/components/CampaignDashboard.test.ts'
s=open(p).read()
s=s.replace("expect(row(wrapper, 7).exists()).toBe(true)","expect(wrapper.find('[data-test=\"dashboard-campaign-7\"]').exists()).toBe(true)")
open(p,'w').write(s)
EOF
npx vitest run 2>&1 | tail -5; npx vue-tsc --noEmit 2>&1 | tail
```

<details><summary>Error</summary>

```
This agent is isolated in the worktree ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-ab40c40da8c9ccbf9, but this command is too complex to verify that it stays inside the worktree. Refusing to run it — a worktree-isolated agent's git operations must target its own worktree. Split it into plain, separate commands and run them from ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-ab40c40da8c9ccbf9.
```
</details>

**Edit** `~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-ab40c40da8c9ccbf9/web/src/App.test.ts`

**Edit** `~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-ab40c40da8c9ccbf9/web/src/components/CampaignDashboard.test.ts`

**Bash** — Typecheck and run web tests

```bash
cd web && npx vue-tsc --noEmit && npx vitest run 2>&1 | tail -5
```

<details><summary>Result</summary>

```
Test Files  5 passed (5)
      Tests  45 passed (45)
   Start at  15:43:30
   Duration  950ms (transform 286ms, setup 0ms, collect 811ms, tests 499ms, environment 1.60s, prepare 291ms)
```
</details>

Full suite now.

**Bash** — Run full test suite

```bash
COMPOSE_PROJECT_NAME=smart-campaign-composer make test 2>&1 | grep -v "^\s*$" | grep -E "ok|FAIL|---|Tests|Test Files|panic" | head -30
```

<details><summary>Result</summary>

```
ok  	campaigncomposer/internal/app	2.635s
ok  	campaigncomposer/internal/config	0.209s
ok  	campaigncomposer/internal/content	0.212s
ok  	campaigncomposer/internal/httpapi	53.634s
ok  	campaigncomposer/internal/llm	0.271s
ok  	campaigncomposer/internal/provider	0.132s
ok  	campaigncomposer/internal/segment	1.886s
(node:77207) [DEP0205] DeprecationWarning: `module.register()` is deprecated. Use `module.registerHooks()` instead.
 Test Files  5 passed (5)
      Tests  45 passed (45)
```
</details>

**Bash** — Run sending tests with race detector

```bash
cd backend && TEST_DATABASE_URL='postgres://campaign:campaign@localhost:55432/campaign?sslmode=disable' go test -race -count=1 -run 'Sent|Sending|Concurrent|Dashboard|Frozen' ./internal/httpapi/ ./internal/provider/ 2>&1 | tail -5
```

<details><summary>Result</summary>

```
ok  	campaigncomposer/internal/httpapi	16.193s
ok  	campaigncomposer/internal/provider	1.293s [no tests to run]
```
</details>

**Bash** — Show working tree status

```bash
git status --short
```

<details><summary>Result</summary>

```
M backend/cmd/campaign/main.go
 M backend/cmd/provider/main.go
 M backend/internal/httpapi/server.go
 M web/src/App.test.ts
 M web/src/App.vue
 M web/src/api.ts
?? backend/internal/httpapi/dashboard.go
?? backend/internal/httpapi/dashboard_test.go
?? backend/internal/httpapi/sending_test.go
?? backend/internal/provider/
?? backend/internal/worker/
?? web/src/components/CampaignDashboard.test.ts
?? web/src/components/CampaignDashboard.vue
```
</details>

**Bash** — Stage changes

```bash
git add -A
```

<details><summary>Result</summary>

```
(Bash completed with no output)
```
</details>

**Write** `<scratchpad>` (27 lines)

**Bash** — Commit ticket 12

```bash
git commit -q -F <scratchpad>
```

**Bash** — Fetch origin

```bash
git fetch -q origin
```

<details><summary>Result</summary>

```
(Bash completed with no output)
```
</details>

<details><summary>Result</summary>

```
(Bash completed with no output)
```
</details>

**Bash** — Merge spec branch into worktree branch

```bash
git merge origin/spec-2-core-flow
```

<details><summary>Result</summary>

```
Already up to date.
```
</details>

**Bash** — Show commit SHA

```bash
git log --oneline -2
```

**Bash** — Show branch name

```bash
git branch --show-current
```

<details><summary>Result</summary>

```
worktree-agent-ab40c40da8c9ccbf9
```
</details>

<details><summary>Result</summary>

```
fc18733 Sending happy path and live dashboard (#12)
5b6a46f Merge remote-tracking branch 'origin/spec-2-core-flow' into worktree-agent-ac458a5b29496bd6b
```
</details>

**Write** `<scratchpad>` (48 lines)

**SubagentHandback**

<details><summary>Result</summary>

```
{"success":true,"message":"Report delivered to your caller."}
```
</details>
