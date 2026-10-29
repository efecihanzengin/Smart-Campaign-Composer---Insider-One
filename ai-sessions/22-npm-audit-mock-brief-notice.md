# AI session 22 — npm audit fixes and mock Brief notice

<!-- session: 7ccc96e5-b242-446f-9a82-97185bf4c606 -->

Claude Code session export (desktop app). Thinking blocks and internal UI tool calls are omitted; tool outputs are truncated; secrets and personal data are redacted.

---

## 🧑 User

npm ci reports 6 vulnerabilities (2 critical). Run npm audit, check whether they're dev-only or reach the production bundle, and fix them with non-breaking upgrades if possible. If something can't be fixed, note it in the README with the reason.

---

## 🤖 Claude

**Bash** — Show package.json and audit report

```bash
cat package.json && npm audit 2>&1 | head -100
```

<details><summary>Error</summary>

```
Exit code 1
cat: package.json: No such file or directory
```
</details>

**Bash** — List repo root and find package.json files

```bash
ls -a && find . -name package.json -not -path '*/node_modules/*'
```

<details><summary>Result</summary>

```
.
..
.claude
.env
.env.example
.git
.github
.gitignore
.scrub-terms
CLAUDE.md
DECISIONS.md
GLOSSARY.md
Makefile
README.md
ai-sessions
backend
docker-compose.yml
docs
scripts
web
./web/package.json
```
</details>

**Bash** — Show web package.json and audit report

```bash
cd ~/Documents/Projects/smart-campaign-composer/web && cat package.json && npm audit 2>&1
```

<details><summary>Error</summary>

```
Exit code 1
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
# npm audit report

@vitest/mocker  <=4.1.11
Severity: moderate
Vitest: Path Traversal / Arbitrary File Read via @vitest/mocker Redirect Mock - https://github.com/advisories/GHSA-82fw-gwwq-j7x9
Vitest: Code injection via unauthenticated manual mock registration - https://github.com/advisories/GHSA-qxg6-89mg-rqv8
fix available via `npm audit fix`
node_modules/@vitest/mocker
  vitest  <=4.1.10
  Depends on vulnerable versions of @vitest/mocker
  Depends on vulnerable versions of tinypool
  node_modules/vitest
… (40 more lines)
```
</details>

**Bash** — Audit prod-only deps and show installed versions

```bash
cd ~/Documents/Projects/smart-campaign-composer/web && npm audit --omit=dev 2>&1 | tail -5; npm ls vue @vue/server-renderer vite vitest tinypool @vitest/mocker 2>&1; grep -rn "server-renderer\|renderToString" src 2>/dev/null | head
```

<details><summary>Result</summary>

```
2 high severity vulnerabilities

To address all issues, run:
  npm audit fix
smart-campaign-composer-web@ ~/Documents/Projects/smart-campaign-composer/web
+-- @tailwindcss/vite@4.1.11
| `-- vite@6.3.5 deduped
+-- @vitejs/plugin-vue@5.2.4
| +-- vite@6.3.5 deduped
| `-- vue@3.5.17 deduped
+-- pinia@3.0.3
| `-- vue@3.5.17 deduped
+-- vite@6.3.5
+-- vitest@3.2.4
| +-- @vitest/mocker@3.2.4
| | `-- vite@6.3.5 deduped
| +-- tinypool@1.1.1
| +-- vite-node@3.2.4
| | `-- vite@6.3.5 deduped
| `-- vite@6.3.5 deduped
`-- vue@3.5.17
  `-- @vue/server-renderer@3.5.17
    `-- vue@3.5.17 deduped
```
</details>

**Bash** — Apply non-breaking audit fixes and re-audit

```bash
npm audit fix 2>&1 | tail -8; npm audit 2>&1 | tail -3; npm ls vue @vue/server-renderer vite vitest tinypool @vitest/mocker 2>&1; git diff --stat
```

<details><summary>Result</summary>

```
To address all issues (including breaking changes), run:
  npm audit fix --force
npm warn allow-scripts 3 packages have install scripts not yet covered by allowScripts:
npm warn allow-scripts   @tailwindcss/oxide@4.1.11 (install: (install scripts present))
npm warn allow-scripts   esbuild@0.25.12 (install: (install scripts present))
npm warn allow-scripts   fsevents@2.3.3 (install: (install scripts present))
npm warn allow-scripts
npm warn allow-scripts Run `npm approve-scripts --allow-scripts-pending` to review, or `npm approve-scripts <pkg>` to allow.

To address all issues (including breaking changes), run:
  npm audit fix --force
smart-campaign-composer-web@ ~/Documents/Projects/smart-campaign-composer/web
+-- @tailwindcss/vite@4.1.11
| `-- vite@6.4.5 deduped
+-- @vitejs/plugin-vue@5.2.4
| +-- vite@6.4.5 deduped
| `-- vue@3.5.44 deduped
+-- pinia@3.0.3
| `-- vue@3.5.44 deduped
+-- vite@6.4.5
+-- vitest@3.2.7
| +-- @vitest/mocker@3.2.7
| | `-- vite@6.4.5 deduped
| +-- tinypool@1.1.1
| +-- vite-node@3.2.4
| | `-- vite@6.4.5 deduped
| `-- vite@6.4.5 deduped
`-- vue@3.5.44
  `-- @vue/server-renderer@3.5.44

 web/package-lock.json | 234 ++++++++++++++++++++++----------------------------
 1 file changed, 103 insertions(+), 131 deletions(-)
```
</details>

Applied the in-range fixes (vue 3.5.44, vite 6.4.5, vitest 3.2.7); checking what's left.

**Bash** — Show remaining audit findings and vitest/tinypool versions

```bash
npm audit 2>&1; npm view vitest@3 version | tail -3; npm view tinypool versions --json | tail -8; npm view vitest@3.2.7 dependencies.tinypool
```

<details><summary>Result</summary>

```
# npm audit report

@vitest/mocker  <=4.1.11
Severity: moderate
Vitest: Path Traversal / Arbitrary File Read via @vitest/mocker Redirect Mock - https://github.com/advisories/GHSA-82fw-gwwq-j7x9
Vitest: Code injection via unauthenticated manual mock registration - https://github.com/advisories/GHSA-qxg6-89mg-rqv8
fix available via `npm audit fix --force`
Will install vitest@5.0.5, which is a breaking change
node_modules/@vitest/mocker
  vitest  0.0.95 - 4.1.10
  Depends on vulnerable versions of @vitest/mocker
  Depends on vulnerable versions of tinypool
  node_modules/vitest

tinypool  <=2.1.1
Severity: critical
Tinypool: Prototype Pollution gadget in worker options leads to Remote Code Execution - https://github.com/advisories/GHSA-5gmw-xhrv-c9v3
Tinypool: Prototype Pollution Gadget to RCE in run() options - https://github.com/advisories/GHSA-85c8-ppgw-ccpr
fix available via `npm audit fix --force`
Will install vitest@5.0.5, which is a breaking change
node_modules/tinypool


3 vulnerabilities (1 moderate, 2 critical)

To address all issues (including breaking changes), run:
  npm audit fix --force
vitest@3.2.5 '3.2.5'
vitest@3.2.6 '3.2.6'
vitest@3.2.7 '3.2.7'
  "1.1.0",
  "1.1.1",
  "2.0.0",
  "2.1.0",
  "2.1.1",
  "2.1.2",
  "2.2.0"
]
^1.1.1
```
</details>

Remaining 3 are all inside vitest (dev-only test runner), with fixes only in vitest 5 — a major bump. Verifying build and tests next.

**Bash** — Build, run tests, and check bundle for SSR code

```bash
npm run build 2>&1 | tail -8 && npm test 2>&1 | tail -8; grep -c "server-renderer\|ssrRender" dist/assets/*.js
```

<details><summary>Result</summary>

```
transforming...
✓ 31 modules transformed.
rendering chunks...
computing gzip size...
dist/index.html                   0.41 kB │ gzip:  0.29 kB
dist/assets/index-DsV8kOh6.css   13.05 kB │ gzip:  3.48 kB
dist/assets/index-B8REWNKs.js   103.42 kB │ gzip: 37.37 kB
✓ built in 552ms
 ✓ src/components/CampaignComposer.test.ts (10 tests) 316ms
 ✓ src/components/ApprovalPanel.test.ts (13 tests) 356ms

 Test Files  6 passed (6)
      Tests  60 passed (60)
   Start at  10:03:53
   Duration  1.47s (transform 552ms, setup 0ms, collect 1.55s, tests 1.48s, environment 2.58s, prepare 587ms)

1
```
</details>

**Bash** — Inspect SSR match in bundle and README headings

```bash
grep -o ".\{60\}\(server-renderer\|ssrRender\).\{60\}" dist/assets/*.js | head -3; git status --short; grep -n "^## \|^### " ../README.md
```

<details><summary>Result</summary>

```
se Ir(e)}function oi(e,t,s){L(t)?e.type.__ssrInlineRender?e.ssrRender=t:e.render=t:X(t)&&(e.setupState=qi(t)),Ir(e)}function Ir(e
 M package-lock.json
7:## Setup
28:### Mock LLM vs real Claude
33:## Testing
47:## Architecture
68:## Not done and known limitations
79:## Repository layout
```
</details>

That match is just a property name in Vue's runtime core; `@vue/server-renderer` isn't in the bundle. Adding the README note.

**Read** `~/Documents/Projects/smart-campaign-composer/README.md`

<details><summary>Result</summary>

```
30	- **`ANTHROPIC_API_KEY` empty (default):** a scripted mock LLM answers both calls and the UI shows a "Mock LLM" badge. It parses the city, the "son N gün" period and the cart/purchase intent from the Brief. It streams fixed, valid Content, so its Content does not follow the Brief closely. It also understands a few regeneration instructions: "daha kısa", "sadece Ankara", and "kadın"/"erkek", which come back as an unsupported criterion so you can see the warning.
31	- **`ANTHROPIC_API_KEY` set:** the Segment and Content calls go to Claude. The model is `claude-sonnet-5-5` unless `ANTHROPIC_MODEL` overrides it. Restart the stack after changing either variable (`docker compose up -d`).
32	
33	## Testing
34	
35	```sh
36	make test
37	```
38	
39	This runs the Go tests (unit tests, plus integration tests against the compose Postgres, which it starts) and the Vitest suite for the frontend. No API key is needed, because the Claude client is tested against a local fake of the Messages API. If Go is not installed on the host, the Go tests run in a container. The full run takes a few minutes: the sending, rate-limit and crash tests run real workers against a mock Provider at 100 requests per second. CI (`.github/workflows/ci.yml`) runs `make test` on every push.
40	
41	Notable suites:
42	
43	- `backend/internal/segment`: the Segment Rule whitelist, the compiler, and the injection and consent-override tests
44	- `backend/internal/content`: GSM-7/UCS-2 measurement, transliteration and the limits checked against the longest name and city in the Audience
45	- `backend/internal/httpapi/*_test.go`: the HTTP flow end to end, including the repair loop, approval guards, retries, rate limiting, cancel, `crash_test.go`, which SIGKILLs and SIGSTOPs worker processes mid-send and then checks for zero duplicates, and `generation_test.go`, which SIGKILLs an api process mid-generation
46	
47	## Architecture
48	
49	```
50	web (Vue 3, nginx) ──/api──▶ api (Go) ──▶ Claude / mock LLM
51	                               │
52	                               ▼
53	                           postgres  ◀── worker (Go) ──▶ provider (mock, Go)
54	```
55	
56	- **Five containers.** `postgres`. `api` serves HTTP and SSE, makes the LLM calls, and runs the migrations and seed on startup. `worker` is the same Go binary in worker mode and sends Deliveries. `provider` is the mock SMS/email Provider. `web` is the built Vue app on nginx, which proxies `/api` to the API.
57	- **Postgres is the only store and also the queue** ([ADR-0001](docs/adr/0001-postgres-as-queue.md)). Approval freezes the Audience into one `deliveries` row per (Campaign, Recipient, Channel), with a unique constraint on that triple. The row holds the rendered payload and re-checks consent inside the approval transaction. Workers claim due rows with `FOR UPDATE SKIP LOCKED`.
58	- **Two LLM calls, each with a validate-and-repair loop** ([ADR-0005](docs/adr/0005-two-llm-calls-with-repair-loop.md)). The first is a structured call for the Segment Rule. That rule is whitelisted JSON, never SQL. The backend validates it and compiles it to parameterized SQL ([ADR-0003](docs/adr/0003-segment-rule-is-whitelisted-json-not-sql.md)), and consent is applied outside the rule ([ADR-0004](docs/adr/0004-consent-outside-segment-rule.md)). The second is a streamed call for the Content, which is forwarded to the browser over SSE as it arrives. Both calls stream over SSE, so the UI shows a repair while it runs. If the output is malformed or breaks a rule, the errors are sent back to Claude, up to 2 times, before the Marketer sees an error. Approval is refused while a generation call runs: each call registers itself and renews the registration every few seconds, so a long call keeps blocking and one left by a crashed api expires within 15 seconds. SMS is held to one GSM-7 segment, with the longest real first name and city of the Audience substituted ([ADR-0002](docs/adr/0002-sms-gsm7-by-default.md)).
59	- **How duplicates are prevented** ([ADR-0006](docs/adr/0006-idempotency-key-and-lease.md)):
60	  - The unique constraint means each Recipient has exactly one Delivery per Campaign and Channel.
61	  - Every request carries `Idempotency-Key: <delivery id>`, so a retry after a timeout replays the Provider's original acceptance instead of sending again.
62	  - A claim holds a lease. A crashed worker's Deliveries are reclaimed once the lease expires.
63	  - Each result is written only if the `attempt_count` still matches the claim, so a stalled worker cannot overwrite a newer outcome.
64	- **Sending.** The limit is 100 requests per second per Channel, enforced by a token bucket that halves its rate when the Provider answers with bursts of 429s. 429s (honouring Retry-After), 5xx and timeouts are retried with exponential backoff and jitter, up to 5 Attempts. Other 4xx fail the Delivery at once. Every Attempt is logged with its status code, latency and error in the `attempts` table.
65	- **Cancelling.** Cancel stops new claims and moves every Delivery not yet handed to the Provider to `cancelled` ([ADR-0006](docs/adr/0006-idempotency-key-and-lease.md)). Deliveries in flight finish. One the Provider rejected is cancelled. One whose outcome is unknown (its request timed out, or its worker died) is retried with the same Idempotency-Key until the Provider says whether it was accepted, so the final counts are true.
66	- **Monitoring.** The dashboard is an SSE stream of pending, sent, failed and cancelled counts per Channel. `/metrics` serves Prometheus counters for sent, failed and retries per Channel, read from the database, so they cover every worker. `/healthz` pings the database.
67	
68	## Not done and known limitations
69	
… (10 more lines)
```
</details>

**Edit** `~/Documents/Projects/smart-campaign-composer/README.md`

I fixed 3 of the 6 vulnerabilities with non-breaking upgrades. The other 3 are in the test runner only and need a major upgrade, so I wrote them up in the README. The build and all 60 frontend tests pass after the change.

**Fixed (only `web/package-lock.json` changed, `package.json` didn't):**
- **`vue` 3.5.17 → 3.5.44**: fixes the 2 high `@vue/server-renderer` XSS advisories. These were the only ones that came from production dependencies. The app doesn't use server-side rendering, and I checked that `@vue/server-renderer` isn't in the built bundle, so they weren't actually reachable.
- **`vite` 6.3.5 → 6.4.5**: fixes the high dev-server advisories (path traversal, `server.fs` bypasses, `launch-editor`). Vite is dev-only.
- **`vitest` 3.2.4 → 3.2.7**.

**Not fixed (all dev-only, nothing reaches the production bundle):**
- **`tinypool`** (the 2 critical prototype-pollution → remote code execution advisories): Vitest 3.x requires `tinypool@^1`, and every 1.x version is affected.
- **`@vitest/mocker`** (1 moderate): the fix isn't in any 3.x release.
- `npm audit fix --force` would install Vitest 5.0.5, which is a breaking change. I didn't do that. The risk only applies when the test suite runs, locally or in CI.

I added a bullet with that explanation under "Not done and known limitations" in [README.md](README.md). Upgrading to Vitest 5 would clear these last 3.

---

## 🧑 User

When the mock LLM doesn't recognise a Brief, show a clear notice in the UI: "Mock LLM yalnızca örnek brief'i tanır; gerçek sonuç için ANTHROPIC_API_KEY girin." Also make the example Brief match robust to whitespace and quote differences. Mention this in the README quick start. Then run make test, commit and push.

after that /export-session

---

## 🤖 Claude

**Bash** — Find mock LLM code and frontend layout

```bash
cd ~/Documents/Projects/smart-campaign-composer && grep -rln -i "mock" backend --include=*.go | head -30; ls web/src web/src/*; sed -n 1,30p README.md
```

<details><summary>Result</summary>

```
(eval):1: no matches found: --include=*.go
web/src/App.test.ts
web/src/App.vue
web/src/api.ts
web/src/env.d.ts
web/src/main.ts
web/src/style.css

web/src:
App.test.ts
App.vue
api.ts
components
env.d.ts
main.ts
stores
style.css
testing

web/src/components:
ApprovalPanel.test.ts
ApprovalPanel.vue
CampaignComposer.test.ts
CampaignComposer.vue
CampaignDashboard.test.ts
CampaignDashboard.vue
ContentReview.test.ts
ContentReview.vue
RegenerateControl.vue
Regeneration.test.ts

web/src/stores:
campaign.ts
overview.ts

web/src/testing:
sse.ts
# Smart Campaign Composer

A Marketer writes a Brief in plain Turkish. Claude turns it into a Segment Rule and into SMS and email Content. The Marketer reviews, edits and approves the Campaign. A worker then sends it to a mock Provider through a rate-limited, retrying queue that does not send duplicates, and a live dashboard shows the progress.
… (27 more lines)
```
</details>

**Bash** — Locate mock LLM files and UI badge references

```bash
grep -rli "mock" backend | grep '\.go$'; grep -rn -i "mock" web/src --include=*.vue --include=*.ts | grep -v test | head -20
```

<details><summary>Result</summary>

```
backend/cmd/campaign/main.go
backend/internal/llm/new_test.go
backend/cmd/provider/main.go
backend/internal/llm/claude.go
backend/internal/llm/llm.go
backend/internal/config/config.go
backend/internal/llm/mock_test.go
backend/internal/llm/mock.go
backend/internal/config/config_test.go
backend/internal/httpapi/ratelimit_test.go
backend/internal/provider/provider.go
backend/internal/httpapi/edit_test.go
backend/internal/httpapi/server.go
backend/internal/httpapi/approve_guards_test.go
backend/internal/httpapi/content_test.go
backend/internal/httpapi/generation_test.go
backend/internal/httpapi/retries_test.go
backend/internal/httpapi/sending_test.go
backend/internal/httpapi/cancel_test.go
backend/internal/httpapi/campaigns_test.go
backend/internal/httpapi/api_test.go
backend/internal/httpapi/approve_test.go
backend/internal/httpapi/repair_test.go
backend/internal/httpapi/regenerate_test.go
backend/internal/httpapi/dashboard_test.go
backend/internal/worker/worker.go
(eval):1: no matches found: --include=*.vue
```
</details>

**Bash** — Read mock LLM and interface files

```bash
cat backend/internal/llm/mock.go backend/internal/llm/llm.go; grep -rn -i "mock" web/src | grep -v "test" | head -20
```

<details><summary>Result</summary>

```
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
… (348 more lines)
```
</details>

**Bash** — Survey the HTTP API and frontend API types

```bash
wc -l backend/internal/httpapi/*.go | grep -v _test; grep -n "llm\.\|Mock\|mock" backend/internal/httpapi/server.go | head -40; cat web/src/api.ts | head -80
```

<details><summary>Result</summary>

```
212 backend/internal/httpapi/approve.go
     193 backend/internal/httpapi/campaigns.go
      47 backend/internal/httpapi/cancel.go
     297 backend/internal/httpapi/content.go
     142 backend/internal/httpapi/dashboard.go
     123 backend/internal/httpapi/edit.go
      69 backend/internal/httpapi/generation.go
      75 backend/internal/httpapi/metrics.go
     255 backend/internal/httpapi/regenerate.go
     134 backend/internal/httpapi/repair.go
     103 backend/internal/httpapi/server.go
    5439 total
18:	// MockLLM is true when no Anthropic API key is configured and the scripted
19:	// mock LLM stands in for Claude.
20:	MockLLM bool
22:	// LLM answers the Segment call; nil means the scripted mock.
23:	LLM llm.LLM
41:		opts.LLM = llm.NewMock()
88:		"llm":             map[string]any{"mock": s.opts.MockLLM, "model": s.opts.Model},
export interface Overview {
  recipient_count: number
  llm: { mock: boolean; model: string }
}

export async function fetchOverview(): Promise<Overview> {
  const res = await fetch('/api/overview')
  if (!res.ok) throw new Error(`GET /api/overview: ${res.status}`)
  return res.json()
}

export interface Audience {
  sms: number
  email: number
}

export interface Campaign {
  id: number
  status: string
  brief: string
  segment: {
… (59 more lines)
```
</details>

**Bash** — Read server and campaigns handlers

```bash
cat backend/internal/httpapi/server.go backend/internal/httpapi/campaigns.go
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
	// DashboardInterval is how often the dashboard stream re-reads the
	// Delivery counts; zero means 500ms.
	DashboardInterval time.Duration
	// GenerationHeartbeat is how often a running generation call renews its
	// claim to block approval; a claim not renewed for three heartbeats has
	// expired (the api process died). Zero means 5s.
	GenerationHeartbeat time.Duration
}

type server struct {
	pool *pgxpool.Pool
	opts Options
}

// NewHandler returns the API's HTTP handler.
func NewHandler(pool *pgxpool.Pool, opts Options) http.Handler {
	if opts.LLM == nil {
… (256 more lines)
```
</details>

Still reading the code: I'm figuring out where the API builds the Campaign response, since that's where the "unrecognised Brief" flag will go.

**Bash** — Find campaign view builders and frontend notice spots

```bash
grep -rn "campaignView{\|func (s \*server) getCampaign" backend/internal/httpapi/*.go | grep -v _test; cat backend/internal/llm/mock_test.go | head -60; grep -n "unsupported\|Mock\|mock" web/src/components/CampaignComposer.vue web/src/App.vue web/src/stores/campaign.ts | head -30
```

<details><summary>Result</summary>

```
backend/internal/httpapi/campaigns.go:173:	c := campaignView{Brief: brief, Repairs: repairs, Segment: newSegmentView(proposal)}
backend/internal/httpapi/dashboard.go:87:func (s *server) getCampaign(w http.ResponseWriter, r *http.Request) {
backend/internal/httpapi/regenerate.go:191:	out := regeneratedSegment{campaignView: campaignView{
package llm_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"campaigncomposer/internal/llm"
)

const brief = "Son 30 günde sepetine ürün ekleyip alışveriş yapmayan İstanbul'daki kullanıcılara %30 indirim hatırlatması gönder. Samimi ama kısa olsun."

func mockContent(t *testing.T, req llm.ContentRequest) map[string]map[string]string {
	t.Helper()
	var raw strings.Builder
	if err := llm.NewMock().Content(context.Background(), req, func(s string) { raw.WriteString(s) }); err != nil {
		t.Fatal(err)
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw.String()), &out); err != nil {
		t.Fatalf("mock Content %q: %v", raw.String(), err)
	}
	got := map[string]map[string]string{}
	for k, v := range out {
		if k == "channels" {
			continue
		}
		m := map[string]string{}
		if err := json.Unmarshal(v, &m); err != nil {
			t.Fatal(err)
		}
		got[k] = m
	}
	return got
}

… (29 more lines)
```
</details>

**Bash** — Read getCampaign, regenerated view, and composer component

```bash
sed -n 80,142p backend/internal/httpapi/dashboard.go; sed -n 170,215p backend/internal/httpapi/regenerate.go; cat web/src/components/CampaignComposer.vue; grep -n "placeholder\|exampleBrief" -r web/src | head
```

<details><summary>Result</summary>

```
if total > 0 {
			out[i].Progress = float64(done) / float64(total)
		}
	}
	return out, rows.Err()
}

func (s *server) getCampaign(w http.ResponseWriter, r *http.Request) {
	id, ok := campaignID(w, r)
	if !ok {
		return
	}
	cs, err := campaignsProgress(r.Context(), s.pool, "c.id = $1", id)
	switch {
	case err != nil:
		writeError(w, err)
	case len(cs) == 0:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": campaignNotFound})
	default:
		writeJSON(w, http.StatusOK, cs[0])
	}
}

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
… (222 more lines)
```
</details>

**Bash** — Add example Brief matcher with tests and run them

```bash
cd ~/Documents/Projects/smart-campaign-composer/backend && cat > internal/llm/example.go <<'EOF'
package llm

import "strings"

// ExampleBrief is the case's example Brief, the one the mock LLM is scripted
// for and the UI shows as the placeholder.
const ExampleBrief = "Son 30 günde sepetine ürün ekleyip alışveriş yapmayan İstanbul'daki kullanıcılara %30 indirim hatırlatması gönder. Samimi ama kısa olsun."

// quoteFolder turns typographic quotes into ASCII ones, so a Brief pasted from
// a document or chat still matches.
var quoteFolder = strings.NewReplacer(
	"‘", "'", "’", "'", "‚", "'", "‛", "'", "ʼ", "'", "´", "'", "`", "'",
	"“", `"`, "”", `"`, "„", `"`, "‟", `"`, "«", `"`, "»", `"`,
)

// IsExampleBrief reports whether brief is the example Brief, ignoring
// differences in whitespace and quote characters, and quotes around the whole
// Brief.
func IsExampleBrief(brief string) bool {
	return normalizeBrief(brief) == normalizeBrief(ExampleBrief)
}

func normalizeBrief(s string) string {
	s = strings.Join(strings.Fields(quoteFolder.Replace(s)), " ")
	for len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		s = strings.TrimSpace(s[1 : len(s)-1])
	}
	return s
}
EOF
cat > internal/llm/example_test.go <<'EOF'
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
EOF
go test ./internal/llm/ 2>&1 | tail -5
```

<details><summary>Result</summary>

```
ok  	campaigncomposer/internal/llm	2.859s
```
</details>

Matcher works. Now adding the flag to the Campaign response.

**Bash** — Find HTTP test helpers and MockLLM usage

```bash
grep -n "func \|MockLLM" internal/httpapi/api_test.go | head -40; grep -rn "MockLLM" internal/httpapi/*_test.go | head
```

<details><summary>Result</summary>

```
20:func startAPI(t *testing.T, opts httpapi.Options) *httptest.Server {
28:func startAPIWithPool(t *testing.T, opts httpapi.Options) (*httptest.Server, *pgxpool.Pool) {
41:func queryRow(t *testing.T, pool *pgxpool.Pool, sql string, args []any, dest ...any) {
48:func getJSON(t *testing.T, url string, into any) int {
61:func TestHealthzIsHealthyWhenPostgresIsReachable(t *testing.T) {
81:func TestOverviewReportsSeededRecipientsAndMockLLM(t *testing.T) {
82:	srv := startAPI(t, httpapi.Options{MockLLM: true, Model: "claude-sonnet-5-5"})
99:func TestOverviewReportsRealLLMWhenNotMocked(t *testing.T) {
100:	srv := startAPI(t, httpapi.Options{MockLLM: false, Model: "claude-opus-5-5"})
109:func TestHealthzIsUnhealthyWhenPostgresIsUnreachable(t *testing.T) {
internal/httpapi/api_test.go:81:func TestOverviewReportsSeededRecipientsAndMockLLM(t *testing.T) {
internal/httpapi/api_test.go:82:	srv := startAPI(t, httpapi.Options{MockLLM: true, Model: "claude-sonnet-5-5"})
internal/httpapi/api_test.go:100:	srv := startAPI(t, httpapi.Options{MockLLM: false, Model: "claude-opus-5-5"})
internal/httpapi/campaigns_test.go:107:	srv, pool := startAPIWithPool(t, httpapi.Options{MockLLM: true, LLM: llm.NewMock()})
internal/httpapi/content_test.go:141:	srv := startAPI(t, httpapi.Options{MockLLM: true, LLM: llm.NewMock()})
```
</details>

**Bash** — Read campaign creation tests

```bash
sed -n 1,140p internal/httpapi/campaigns_test.go
```

<details><summary>Result</summary>

```
package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"campaigncomposer/internal/httpapi"
	"campaigncomposer/internal/llm"
)

// exampleBrief is the Brief from the case, used as the placeholder in the UI.
const exampleBrief = "Son 30 günde sepetine ürün ekleyip alışveriş yapmayan İstanbul'daki kullanıcılara %30 indirim hatırlatması gönder. Samimi ama kısa olsun."

type campaign struct {
	ID      int64  `json:"id"`
	Status  string `json:"status"`
	Brief   string `json:"brief"`
	Segment struct {
		Rule                json.RawMessage `json:"rule"`
		Labels              []string        `json:"labels"`
		UnsupportedCriteria []string        `json:"unsupported_criteria"`
	} `json:"segment"`
	Audience struct {
		SMS   int `json:"sms"`
		Email int `json:"email"`
	} `json:"audience"`
}

func postJSON(t *testing.T, url string, body, into any) int {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.Post(url, "application/json", bytes.NewReader(b))
… (100 more lines)
```
</details>

Now the backend edits: the field on `campaignView`, set on create and on Segment regeneration.

**Bash** — Add mock_brief_unrecognised flag to campaign responses

```bash
python3 - <<'EOF'
import re
p='internal/httpapi/campaigns.go'
s=open(p).read()
s=s.replace('''	DisabledChannels map[string]string `json:"disabled_channels"`
}''','''	DisabledChannels map[string]string `json:"disabled_channels"`
	// MockBriefUnrecognised is set when the mock LLM answered a Brief other
	// than the example Brief it is scripted for, so its proposal is a guess.
	MockBriefUnrecognised bool `json:"mock_brief_unrecognised"`
}

// mockBriefUnrecognised reports whether the mock LLM stands in for Claude and
// brief is not the example Brief.
func (s *server) mockBriefUnrecognised(brief string) bool {
	return s.opts.MockLLM && !llm.IsExampleBrief(brief)
}''',1)
s=s.replace('''c := campaignView{Brief: brief, Repairs: repairs, Segment: newSegmentView(proposal)}''','''c := campaignView{Brief: brief, Repairs: repairs, Segment: newSegmentView(proposal),
		MockBriefUnrecognised: s.mockBriefUnrecognised(brief)}''',1)
open(p,'w').write(s)
p='internal/httpapi/regenerate.go'
s=open(p).read()
s=s.replace('''		ID: c.id, Status: c.status, Brief: c.brief, Repairs: repairs, Segment: newSegmentView(proposal),
	}}''','''		ID: c.id, Status: c.status, Brief: c.brief, Repairs: repairs, Segment: newSegmentView(proposal),
		MockBriefUnrecognised: s.mockBriefUnrecognised(c.brief),
	}}''',1)
open(p,'w').write(s)
EOF
gofmt -l internal/ ; go vet ./internal/httpapi/ && git diff --stat
```

<details><summary>Result</summary>

```
README.md                              |   1 +
 backend/internal/httpapi/campaigns.go  |  12 +-
 backend/internal/httpapi/regenerate.go |   1 +
 web/package-lock.json                  | 234 +++++++++++++++------------------
 4 files changed, 116 insertions(+), 132 deletions(-)
```
</details>

Adding an HTTP test for the flag.

**Bash** — Show the createCampaign test helper

```bash
grep -n "func createCampaign" -A12 internal/httpapi/*_test.go
```

<details><summary>Result</summary>

```
internal/httpapi/content_test.go:85:func createCampaign(t *testing.T, baseURL, brief string) campaign {
internal/httpapi/content_test.go-86-	t.Helper()
internal/httpapi/content_test.go-87-	var c campaign
internal/httpapi/content_test.go-88-	segmentDone(t, streamSegment(t, baseURL+"/api/campaigns", map[string]string{"brief": brief}), &c)
internal/httpapi/content_test.go-89-	return c
internal/httpapi/content_test.go-90-}
internal/httpapi/content_test.go-91-
internal/httpapi/content_test.go-92-// streamContent asks for the Campaign's Content and returns every SSE event.
internal/httpapi/content_test.go-93-func streamContent(t *testing.T, baseURL string, id int64) []sseEvent {
internal/httpapi/content_test.go-94-	t.Helper()
internal/httpapi/content_test.go-95-	res, err := http.Post(baseURL+"/api/campaigns/"+strconv.FormatInt(id, 10)+"/content", "application/json", nil)
internal/httpapi/content_test.go-96-	if err != nil {
internal/httpapi/content_test.go-97-		t.Fatalf("POST content: %v", err)
```
</details>

**Bash** — Add HTTP test for the unrecognised-Brief flag

```bash
python3 - <<'EOF'
p='internal/httpapi/campaigns_test.go'
s=open(p).read()
s=s.replace('''		Email int `json:"email"`
	} `json:"audience"`
}''','''		Email int `json:"email"`
	} `json:"audience"`
	MockBriefUnrecognised bool `json:"mock_brief_unrecognised"`
}''',1)
s=s.replace('''func TestExampleBriefYieldsSegmentRuleAndPerChannelAudience(''','''func TestMockFlagsABriefOtherThanTheExample(t *testing.T) {
	srv := startAPI(t, httpapi.Options{MockLLM: true, LLM: llm.NewMock()})

	pasted := "  \\u201c" + strings.ReplaceAll(strings.ReplaceAll(exampleBrief, "'", "\\u2019"), " ", "  ") + "\\u201d\\n"
	if c := createCampaign(t, srv.URL, pasted); c.MockBriefUnrecognised {
		t.Errorf("example Brief with other whitespace and quotes: mock_brief_unrecognised = true, want false")
	}
	if c := createCampaign(t, srv.URL, "Ankara'daki kullanıcılara yeni yıl kutlaması gönder."); !c.MockBriefUnrecognised {
		t.Errorf("other Brief: mock_brief_unrecognised = false, want true")
	}

	real := startAPI(t, httpapi.Options{MockLLM: false, LLM: llm.NewMock()})
	if c := createCampaign(t, real.URL, "Ankara'daki kullanıcılara yeni yıl kutlaması gönder."); c.MockBriefUnrecognised {
		t.Errorf("real LLM: mock_brief_unrecognised = true, want false")
	}
}

func TestExampleBriefYieldsSegmentRuleAndPerChannelAudience(''',1)
open(p,'w').write(s)
EOF
grep -n "Regenerate.*Segment\|func Test" internal/httpapi/regenerate_test.go | head
```

<details><summary>Result</summary>

```
55:func TestRegeneratingTheSMSSendsTheInstructionAndKeepsTheEditedEmail(t *testing.T) {
104:func TestRegeneratingTheEmailIgnoresWhateverClaudeWritesForTheSMS(t *testing.T) {
142:func TestRegeneratedContentGoesThroughTheRepairLoop(t *testing.T) {
163:func TestRegenerationStillInvalidAfterTwoRepairsKeepsTheCurrentContent(t *testing.T) {
184:func TestRegeneratingAnUnknownOrNonDraftCampaignIsRejected(t *testing.T) {
222:func TestRegeneratingTheSegmentRuleRevalidatesTheEditedContentWithoutRegeneratingIt(t *testing.T) {
276:func TestSegmentRuleRegenerationGoesThroughTheRepairLoop(t *testing.T) {
295:func TestSegmentRuleRegenerationThatCannotBeRepairedKeepsTheCurrentRule(t *testing.T) {
313:func TestASegmentRuleRegenerationThatEmptiesTheAudienceDisablesTheChannels(t *testing.T) {
334:func TestTheDefaultMockRegeneratesValidContentAndSegmentRules(t *testing.T) {
```
</details>

Now the frontend: the type, the notice, and a component test.

**Bash** — Check store assignments and composer test setup

```bash
cd ~/Documents/Projects/smart-campaign-composer/web && grep -n "campaign.value = \|campaign.value\b.*=" src/stores/campaign.ts | head; sed -n 1,80p src/components/CampaignComposer.test.ts
```

<details><summary>Result</summary>

```
import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import CampaignComposer from './CampaignComposer.vue'
import { openStream, sse } from '../testing/sse'

const exampleBrief =
  "Son 30 günde sepetine ürün ekleyip alışveriş yapmayan İstanbul'daki kullanıcılara %30 indirim hatırlatması gönder. Samimi ama kısa olsun."

function campaign(overrides: { unsupported_criteria?: string[] } = {}) {
  return {
    id: 1,
    status: 'draft',
    brief: exampleBrief,
    segment: {
      rule: { all: [] },
      labels: ['Şehir: İstanbul', 'Son 30 günde sepete ekledi', 'Son 30 günde satın almadı'],
      unsupported_criteria: overrides.unsupported_criteria ?? [],
    },
    audience: { sms: 412, email: 1534 },
  }
}

// mockFetch answers the Segment call with response (a fresh one per call).
function mockFetch(response: () => Response) {
  const fetchMock = vi.fn(async () => response())
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

// mockCreate streams the created Campaign as the Segment call's result.
const mockCreate = (body: unknown) => mockFetch(() => sse(['segment.done', body]))

const failedCall = { error: 'Model geçerli bir hedef kitle kuralı üretemedi. Lütfen tekrar deneyin.', retryable: true }

async function submitBrief(brief = exampleBrief) {
  const wrapper = mount(CampaignComposer, { global: { plugins: [createPinia()] } })
  await wrapper.get('[data-test="brief"]').setValue(brief)
  await wrapper.get('[data-test="brief-form"]').trigger('submit')
  await flushPromises()
… (39 more lines)
```
</details>

**Bash** — See how the store updates the campaign

```bash
grep -n "campaign" src/stores/campaign.ts | grep -n "= \|\.\.\." | head -15; grep -n "unsupported" src/components/CampaignComposer.test.ts
```

<details><summary>Result</summary>

```
1:34:export const useCampaignStore = defineStore('campaign', {
4:81:    locked: (s) => s.streaming || s.segmentRegenerating || (s.campaign !== null && s.campaign.status !== 'draft'),
6:93:        if (s.campaign.audience[c] === 0) out.push(`${channelNames[c]} kanalında izin vermiş alıcı yok.`)
7:104:      this.campaign = null
8:107:        this.campaign = await createCampaign(brief, (n) => (this.segmentRepairing = n))
10:116:        this.enabledChannels = this.campaign.enabled_channels ?? [...channels]
15:215:        const v = await validateContent(this.campaign.id, channel, c)
17:230:        const v = await updateContent(this.campaign.id, channel, c)
21:322:        const r = await regenerateSegment(this.campaign.id, instruction, (n) => (this.segmentRepairing = n))
22:324:        this.campaign = { id, status, brief, segment, audience, repairs, enabled_channels, disabled_channels }
24:346:        this.enabledChannels = await setChannel(this.campaign.id, channel, enabled)
26:364:        this.approval = await approveCampaign(this.campaign.id)
27:365:        this.campaign = { ...this.campaign, status: this.approval.status }
28:377:      if (!this.campaign || this.campaign.status !== 'draft' || this.cancelling) return
29:381:        const status = await cancelCampaign(this.campaign.id)
10:function campaign(overrides: { unsupported_criteria?: string[] } = {}) {
18:      unsupported_criteria: overrides.unsupported_criteria ?? [],
87:  it('warns about unsupported criteria above the labels', async () => {
88:    mockCreate(campaign({ unsupported_criteria: ['ayakkabı satın alanlar'] }))
92:    const warning = wrapper.get('[data-test="unsupported-warning"]')
95:    expect(html.indexOf('data-test="unsupported-warning"')).toBeLessThan(html.indexOf('data-test="segment-label"'))
103:    expect(wrapper.find('[data-test="unsupported-warning"]').exists()).toBe(false)
```
</details>

**Bash** — Read regenerate assignment and unsupported test

```bash
sed -n 318,328p src/stores/campaign.ts; sed -n 84,106p src/components/CampaignComposer.test.ts
```

<details><summary>Result</summary>

```
this.segmentError = null
      this.segmentRetryable = false
      this.lastSegmentInstruction = instruction
      try {
        const r = await regenerateSegment(this.campaign.id, instruction, (n) => (this.segmentRepairing = n))
        const { id, status, brief, segment, audience, repairs, enabled_channels, disabled_channels } = r
        this.campaign = { id, status, brief, segment, audience, repairs, enabled_channels, disabled_channels }
        // Validation answers for the old Audience must not overwrite these.
        for (const c of channels) this.revisions[c]++
        this.validation = r.validation
        this.enabledChannels = r.enabled_channels
    expect(wrapper.get('[data-test="audience-email"]').text()).toBe('1.534')
  })

  it('warns about unsupported criteria above the labels', async () => {
    mockCreate(campaign({ unsupported_criteria: ['ayakkabı satın alanlar'] }))

    const wrapper = await submitBrief()

    const warning = wrapper.get('[data-test="unsupported-warning"]')
    expect(warning.text()).toContain('ayakkabı satın alanlar')
    const html = wrapper.html()
    expect(html.indexOf('data-test="unsupported-warning"')).toBeLessThan(html.indexOf('data-test="segment-label"'))
  })

  it('shows no warning when every criterion is supported', async () => {
    mockCreate(campaign())

    const wrapper = await submitBrief()

    expect(wrapper.find('[data-test="unsupported-warning"]').exists()).toBe(false)
  })

  it('shows the API error when generation fails', async () => {
```
</details>

**Bash** — Add notice to UI, type, store, and tests; run checks

```bash
python3 - <<'EOF'
def sub(p,a,b):
    s=open(p).read()
    assert s.count(a)==1,(p,a)
    open(p,'w').write(s.replace(a,b))
sub('src/api.ts','''  // repairs is how many repairs the Segment call needed (ADR-0005).
  repairs?: number
}''','''  // repairs is how many repairs the Segment call needed (ADR-0005).
  repairs?: number
  // mock_brief_unrecognised is set when the mock LLM answered a Brief other
  // than the example Brief, so its proposal is only a guess.
  mock_brief_unrecognised?: boolean
}''')
sub('src/stores/campaign.ts','''        const { id, status, brief, segment, audience, repairs, enabled_channels, disabled_channels } = r
        this.campaign = { id, status, brief, segment, audience, repairs, enabled_channels, disabled_channels }''','''        const { id, status, brief, segment, audience, repairs, enabled_channels, disabled_channels } = r
        const { mock_brief_unrecognised } = r
        this.campaign = {
          id,
          status,
          brief,
          segment,
          audience,
          repairs,
          enabled_channels,
          disabled_channels,
          mock_brief_unrecognised,
        }''')
sub('src/components/CampaignComposer.vue','''      <h2 class="text-sm font-medium text-slate-500">Hedef kitle</h2>
''','''      <h2 class="text-sm font-medium text-slate-500">Hedef kitle</h2>

      <p
        v-if="store.campaign.mock_brief_unrecognised"
        data-test="mock-brief-notice"
        role="status"
        class="rounded-md bg-sky-50 p-3 text-sm text-sky-800"
      >
        Mock LLM yalnızca örnek brief'i tanır; gerçek sonuç için ANTHROPIC_API_KEY girin.
      </p>
''')
sub('src/components/CampaignComposer.test.ts','''function campaign(overrides: { unsupported_criteria?: string[] } = {}) {
  return {
    id: 1,''','''function campaign(overrides: { unsupported_criteria?: string[]; mock_brief_unrecognised?: boolean } = {}) {
  return {
    mock_brief_unrecognised: overrides.mock_brief_unrecognised ?? false,
    id: 1,''')
sub('src/components/CampaignComposer.test.ts','''  it('shows the API error when generation fails', async () => {''','''  it('tells the Marketer when the mock LLM did not recognise the Brief', async () => {
    mockCreate(campaign({ mock_brief_unrecognised: true }))

    const wrapper = await submitBrief('Tüm kullanıcılara yeni yıl kutlaması gönder.')

    expect(wrapper.get('[data-test="mock-brief-notice"]').text()).toBe(
      "Mock LLM yalnızca örnek brief'i tanır; gerçek sonuç için ANTHROPIC_API_KEY girin.",
    )
  })

  it('shows no mock notice for the example Brief', async () => {
    mockCreate(campaign())

    const wrapper = await submitBrief()

    expect(wrapper.find('[data-test="mock-brief-notice"]').exists()).toBe(false)
  })

  it('shows the API error when generation fails', async () => {''')
EOF
npx prettier --check src 2>&1 | tail -3; npm run typecheck 2>&1 | tail -3 && npx vitest run 2>&1 | tail -4
```

<details><summary>Result</summary>

```
[warn] src/stores/overview.ts
[warn] src/testing/sse.ts
[warn] Code style issues found in 18 files. Run Prettier with --write to fix.
> typecheck
> vue-tsc --noEmit

      Tests  62 passed (62)
   Start at  10:14:59
   Duration  1.18s (transform 351ms, setup 0ms, collect 1.16s, tests 965ms, environment 2.38s, prepare 358ms)
```
</details>

Tests pass. Tidying the store destructure into one statement.

**Edit** `~/Documents/Projects/smart-campaign-composer/web/src/stores/campaign.ts`

Tests are in. Now the README quick start.

**Bash** — Add mock-Brief note to README quick start

```bash
cd ~/Documents/Projects/smart-campaign-composer && python3 - <<'EOF'
p='README.md'
s=open(p).read()
a='''With the seed it gives the rule "Şehir: İstanbul · Son 30 günde sepete ekledi · Son 30 günde satın almadı", with an Audience of 387 Recipients for SMS and 514 for email after consent.
'''
assert s.count(a)==1
s=s.replace(a,a+'''
Without an API key, use the example Brief: the mock LLM is scripted for it. Extra whitespace and typographic quotes (’ “ ”, or quotes around the whole Brief) do not matter. Any other Brief still gets a guessed proposal, but the UI then shows "Mock LLM yalnızca örnek brief'i tanır; gerçek sonuç için ANTHROPIC_API_KEY girin." Set `ANTHROPIC_API_KEY` in `.env` to have Claude read the Brief.
''')
open(p,'w').write(s)
EOF
git diff README.md | head -20
```

<details><summary>Result</summary>

```
diff --git a/README.md b/README.md
index 42e0ed3..4eff141 100644
--- a/README.md
+++ b/README.md
@@ -23,6 +23,8 @@ On first start the API runs the migrations and seeds about 5,000 Recipients and
 
 With the seed it gives the rule "Şehir: İstanbul · Son 30 günde sepete ekledi · Son 30 günde satın almadı", with an Audience of 387 Recipients for SMS and 514 for email after consent.
 
+Without an API key, use the example Brief: the mock LLM is scripted for it. Extra whitespace and typographic quotes (’ “ ”, or quotes around the whole Brief) do not matter. Any other Brief still gets a guessed proposal, but the UI then shows "Mock LLM yalnızca örnek brief'i tanır; gerçek sonuç için ANTHROPIC_API_KEY girin." Set `ANTHROPIC_API_KEY` in `.env` to have Claude read the Brief.
+
 If a port is taken, set `WEB_PORT`, `API_PORT` or `POSTGRES_PORT` in `.env`. All ports bind to 127.0.0.1. `.env.example` documents every variable, including the mock Provider's failure rates and the worker's retry, lease and rate-limit settings.
 
 ### Mock LLM vs real Claude
@@ -73,6 +75,7 @@ web (Vue 3, nginx) ──/api──▶ api (Go) ──▶ Claude / mock LLM
 - **The Provider can accept a Delivery that is marked `failed`.** If the last allowed Attempt times out after the Provider accepted it, the Delivery is marked failed. Nothing is duplicated, but the dashboard under-reports sent messages.
 - **A SIGKILLed worker's in-flight Attempt is never logged**, so `attempt_count` can exceed the number of Attempt rows. A graceful stop does log it.
 - **A cancel can still send a Delivery that was in flight.** If its request timed out without reaching the Provider, the retry that finds out its outcome sends it. The alternative was to cancel it while it might already have been sent, which makes the final counts wrong.
+- **`npm audit` reports 3 open advisories in the frontend test runner** (2 critical in `tinypool`, 1 moderate in `@vitest/mocker`). All three come in through `vitest`, a dev dependency, and none reaches the production bundle. Vitest 3.x pins `tinypool@^1`, and both fixes ship only in Vitest 5, a major upgrade that was out of scope. The risk is limited to running the test suite locally or in CI. The rest (Vue, Vite) were fixed with in-range upgrades on 29 October 2026.
 - Lease expiry uses the database clock and the start window uses the worker's clock. Clock skew between the two hosts eats into the safety margin.
 - **Not built:** authentication (there is a single implicit Marketer), per-language and A/B Content ([ADR-0008](docs/adr/0008-one-content-per-channel.md)), Brief criteria the Segment Rule cannot express (they are reported as warnings, [ADR-0007](docs/adr/0007-unsupported-criteria-warning.md)), real SMS or email Providers, and the case's bonus items (scheduled sending and so on).
```
</details>

README updated. Running `make test` now (it takes a few minutes).

**Bash** — Run the full test suite and log output

```bash
make test > <scratchpad> 2>&1; echo "exit $?"
```

<details><summary>Result</summary>

```
Command running in background with ID: bmgp3d2bi. Output is being written to: <scratchpad> You will be notified when it completes — do not poll that file or sleep while waiting. Read the file only if the user asks for progress, or if you need a line it prints before you can go on, such as a server's ready line or port.
```
</details>

While that runs, I'll go over the full diff before committing.

**Bash** — Review working tree diff

```bash
git status --short && git diff backend web/src | head -150
```

<details><summary>Result</summary>

```
M README.md
 M backend/internal/httpapi/campaigns.go
 M backend/internal/httpapi/campaigns_test.go
 M backend/internal/httpapi/regenerate.go
 M web/package-lock.json
 M web/src/api.ts
 M web/src/components/CampaignComposer.test.ts
 M web/src/components/CampaignComposer.vue
 M web/src/stores/campaign.ts
?? backend/internal/llm/example.go
?? backend/internal/llm/example_test.go
diff --git a/backend/internal/httpapi/campaigns.go b/backend/internal/httpapi/campaigns.go
index 693bc11..328c1ec 100644
--- a/backend/internal/httpapi/campaigns.go
+++ b/backend/internal/httpapi/campaigns.go
@@ -86,6 +86,15 @@ type campaignView struct {
 	// DisabledChannels explains each one that was turned off for that reason.
 	EnabledChannels  []string          `json:"enabled_channels"`
 	DisabledChannels map[string]string `json:"disabled_channels"`
+	// MockBriefUnrecognised is set when the mock LLM answered a Brief other
+	// than the example Brief it is scripted for, so its proposal is a guess.
+	MockBriefUnrecognised bool `json:"mock_brief_unrecognised"`
+}
+
+// mockBriefUnrecognised reports whether the mock LLM stands in for Claude and
+// brief is not the example Brief.
+func (s *server) mockBriefUnrecognised(brief string) bool {
+	return s.opts.MockLLM && !llm.IsExampleBrief(brief)
 }
 
 // campaignAudience counts a stored Campaign's Audience per Channel.
@@ -170,7 +179,8 @@ func (s *server) createCampaign(w http.ResponseWriter, r *http.Request) {
 		sse.fail("encode Segment Rule", err, saveFailed, true)
 		return
 	}
-	c := campaignView{Brief: brief, Repairs: repairs, Segment: newSegmentView(proposal)}
+	c := campaignView{Brief: brief, Repairs: repairs, Segment: newSegmentView(proposal),
+		MockBriefUnrecognised: s.mockBriefUnrecognised(brief)}
 	if c.Audience, err = segment.CountAudience(ctx, s.pool, proposal.Rule); err != nil {
 		sse.fail("count audience", err, saveFailed, true)
… (121 more lines)
```
</details>

**Bash** — Show make test results summary

```bash
L=<scratchpad> cat <scratchpad> grep -E "^(ok|FAIL|---)|Tests |Test Files" $L
```

<details><summary>Result</summary>

```
exit 2

[exited with code 0]
```
</details>

**Bash** — Read the end of the make test log

```bash
tail -40 <scratchpad>
```

<details><summary>Result</summary>

```
docker compose up -d --wait postgres
 Network smart-campaign-composer_default  Creating
 Network smart-campaign-composer_default  Created
 Container smart-campaign-composer-postgres-1  Creating
 Container smart-campaign-composer-postgres-1  Created
 Container smart-campaign-composer-postgres-1  Starting
Error response from daemon: failed to set up container networking: driver failed programming external connectivity on endpoint smart-campaign-composer-postgres-1 (5000e77f29e76f0d967218548814afdbca11ba0151b16dd201148b511d049c7a): Bind for 127.0.0.1:55432 failed: port is already allocated
make: *** [postgres] Error 1
```
</details>

`make test` failed before any test ran: Postgres couldn't start because port 55432 is already in use. Finding out what holds it.

**Bash** — Find what is listening on port 55432

```bash
docker ps --format '{{.Names}}\t{{.Ports}}\t{{.Status}}' | grep -i 55432; lsof -nP -iTCP:55432 -sTCP:LISTEN 2>/dev/null | head
```

<details><summary>Result</summary>

```
test-clone-postgres-1	127.0.0.1:55432->5432/tcp	Up 23 minutes (healthy)
COMMAND    PID  USER   FD   TYPE             DEVICE SIZE/OFF NODE NAME
com.docke 2218 [user]  218u  IPv4 0x78f8bc5513344aa0      0t0  TCP 127.0.0.1:55432 (LISTEN)
```
</details>

Another Compose project (`test-clone-postgres-1`, up 23 minutes) holds the port. It isn't mine, so I'll leave it running and check whether the Makefile accepts a different port.

**Bash** — Check how make test picks the Postgres port

```bash
cat Makefile; grep -n POSTGRES_PORT docker-compose.yml .env .env.example
```

<details><summary>Result</summary>

```
-include .env
POSTGRES_PORT ?= 55432
TEST_DATABASE_URL ?= postgres://campaign:campaign@localhost:$(POSTGRES_PORT)/campaign?sslmode=disable
HAS_GO := $(shell command -v go 2>/dev/null)

.PHONY: up down test test-go test-web postgres

up:
	docker compose up --build

down:
	docker compose down

test: test-go test-web

postgres:
	docker compose up -d --wait postgres

# Go tests run on the host when Go is installed, otherwise in a golang container
# on the compose network. Either way they hit the compose Postgres.
test-go: postgres
ifdef HAS_GO
	cd backend && TEST_DATABASE_URL='$(TEST_DATABASE_URL)' go test -count=1 ./...
else
	docker compose --profile test run --build --rm gotest
endif

web/node_modules: web/package-lock.json
	cd web && npm ci
	touch web/node_modules

test-web: web/node_modules
	cd web && npx vitest run
.env:52:POSTGRES_PORT=55432
docker-compose.yml:9:      - "127.0.0.1:${POSTGRES_PORT:-55432}:5432"
.env.example:52:POSTGRES_PORT=55432
```
</details>

I'll run it on port 55433 for this run only. The other project's container and `.env` stay as they are.

**Bash** — Run full test suite with Postgres on port 55433

```bash
POSTGRES_PORT=55433 make test POSTGRES_PORT=55433 > <scratchpad> 2>&1; echo "exit $?"
```

<details><summary>Result</summary>

```
Command running in background with ID: b10u013tu. Output is being written to: <scratchpad> You will be notified when it completes — do not poll that file or sleep while waiting. Read the file only if the user asks for progress, or if you need a line it prints before you can go on, such as a server's ready line or port.
```
</details>

`make test` is running again in the background. I'll commit and push once it finishes.

---

## 🧑 User

<task-notification>
<task-id>b10u013tu</task-id>
<tool-use-id>toolu_01KYUBEiTR5SxW9h8g86jfuW</tool-use-id>
<output-file><scratchpad>
<status>completed</status>
<summary>Background command "Run full test suite with Postgres on port 55433" completed (exit code 0)</summary>
</task-notification>

---

## 🤖 Claude

**Bash** — Summarize make test results

```bash
L=<scratchpad> cat <scratchpad> grep -E "^(ok|FAIL|---|panic)|Tests |Test Files|Error" $L | head -40
```

<details><summary>Result</summary>

```
exit 0

[exited with code 0]
ok  	campaigncomposer/internal/app	3.970s
ok  	campaigncomposer/internal/config	0.294s
ok  	campaigncomposer/internal/content	0.301s
ok  	campaigncomposer/internal/httpapi	157.924s
ok  	campaigncomposer/internal/llm	2.575s
ok  	campaigncomposer/internal/provider	0.240s
ok  	campaigncomposer/internal/segment	2.606s
ok  	campaigncomposer/internal/worker	0.217s
 Test Files  6 passed (6)
      Tests  62 passed (62)
```
</details>

All green: 8 Go packages and 62 Vitest tests. Before committing, I'm checking how this repo lands work on `main`.

**Bash** — Show first-parent history on main and remote

```bash
git log --first-parent --oneline main | head -12; git remote -v | head -2; git status -sb | head -1
```

<details><summary>Result</summary>

```
f80af49 Merge pull request #18 from [redacted]/spec-2-core-flow
be3d0c9 Redact name parts in any case and add the export-session skill
2d58564 Read scrub terms from .scrub-terms and tag exports with their session
f77c01f Add AI session export for ticket #5
1bb8faf Full Segment Rule language and security tests (#5)
55ec654 Add AI session export for ticket #4
938f897 Brief to Segment Rule to per-Channel Audience count (#4)
9dd8f54 Add AI session export for ticket #3
a36f67d Walking skeleton: compose, seed, health, CI (#3)
4cec866 Add AI session exports, transcript converter and case brief
44854e1 Add domain glossary and architecture decision records
11a584b Add agent skills setup
origin	https://github.com/[redacted]/Smart-Campaign-Composer---Insider-One.git (fetch)
origin	https://github.com/[redacted]/Smart-Campaign-Composer---Insider-One.git (push)
## main...origin/main
```
</details>

Commits go straight to `main` in this repo, so I'll push there. The audit fix and the feature go in as two separate commits.

**Bash** — Commit audit fix and feature separately, then push

```bash
git add web/package-lock.json && git add -p README.md <<'EOF' >/dev/null
n
y
EOF
git diff --cached --stat && git commit -q -F - <<'EOF'
Fix npm audit findings with in-range upgrades

Upgrade vue to 3.5.44, vite to 6.4.5 and vitest to 3.2.7 within their
existing ranges. The three advisories left (tinypool, @vitest/mocker) are
dev-only and need Vitest 5; the README notes why.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
git add -A backend web/src README.md && git diff --cached --stat && git commit -q -F - <<'EOF'
Tell the Marketer when the mock LLM does not recognise the Brief

The mock LLM is scripted for the example Brief. Campaigns now carry
mock_brief_unrecognised, and the composer shows a notice asking for an
ANTHROPIC_API_KEY when it is set. The example Brief match ignores
whitespace and typographic or surrounding quotes.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
git status --short; git log --oneline -3 && git push origin main 2>&1 | tail -2
```

<details><summary>Result</summary>

```
README.md             |   1 +
 web/package-lock.json | 234 ++++++++++++++++++++++----------------------------
 2 files changed, 104 insertions(+), 131 deletions(-)
 README.md                                   |  2 ++
 backend/internal/httpapi/campaigns.go       | 12 ++++++++++-
 backend/internal/httpapi/campaigns_test.go  | 18 +++++++++++++++++
 backend/internal/httpapi/regenerate.go      |  1 +
 backend/internal/llm/example.go             | 29 +++++++++++++++++++++++++++
 backend/internal/llm/example_test.go        | 31 +++++++++++++++++++++++++++++
 web/src/api.ts                              |  3 +++
 web/src/components/CampaignComposer.test.ts | 21 ++++++++++++++++++-
 web/src/components/CampaignComposer.vue     |  9 +++++++++
 web/src/stores/campaign.ts                  | 24 ++++++++++++++++++++--
 10 files changed, 146 insertions(+), 4 deletions(-)
897b221 Tell the Marketer when the mock LLM does not recognise the Brief
edbca0a Fix npm audit findings with in-range upgrades
f80af49 Merge pull request #18 from [redacted]/spec-2-core-flow
To https://github.com/[redacted]/Smart-Campaign-Composer---Insider-One.git
   f80af49..897b221  main -> main
```
</details>

Both commits are pushed to `main`. Now running `/export-session`.

**Skill** `export-session`

_(skill instructions loaded: NN-<slug>.md`)_

**Bash** — Locate transcript and existing exports

```bash
T=~/.claude/projects/-Users-[user]-Documents-Projects-smart-campaign-composer/7ccc96e5-b242-446f-9a82-97185bf4c606.jsonl; ls -la $T; ls ai-sessions; grep -l "session: 7ccc96e5-b242-446f-9a82-97185bf4c606" ai-sessions/* ; ls -a | grep scrub
```

<details><summary>Result</summary>

```
-rw-------  1 [user]  staff  1335320 Oct 29 10:19 ~/.claude/projects/-Users-[user]-Documents-Projects-smart-campaign-composer/7ccc96e5-b242-446f-9a82-97185bf4c606.jsonl
00-initial-spec.md
01-planning.md
02-ticket-3.md
03-ticket-4-segment-rule.md
04-ticket-5-segment-rule-language.md
05-spec-2-orchestrator.md
06-ticket-6-streamed-content.md
07-ticket-7-claude-client.md
08-ticket-8-sms-rules-editing.md
09-ticket-9-repair-loop.md
10-ticket-11-approval.md
11-ticket-10-regeneration.md
12-ticket-12-sending-dashboard.md
13-ticket-13-retries-metrics.md
14-ticket-14-rate-limiting.md
15-ticket-16-cancel.md
16-merge-ticket-13.md
17-ticket-15-crash-safety.md
18-ticket-17-readme-demo.md
19-review-standards.md
20-review-spec.md
21-review-fixes.md
.scrub-terms
```
</details>

**Bash** — Convert transcript and leak-check the export

```bash
python3 -I scripts/transcript_to_md.py ~/.claude/projects/-Users-[user]-Documents-Projects-smart-campaign-composer/7ccc96e5-b242-446f-9a82-97185bf4c606.jsonl ai-sessions/22-npm-audit-mock-brief-notice.md "AI session 22 — npm audit fixes and mock Brief notice" 2>&1 | tail -5; F=ai-sessions/22-npm-audit-mock-brief-notice.md; wc -l $F; for t in $(git config user.name) $(cat .scrub-terms) 'sk-ant-' '~' ; do echo "== $t: $(grep -ic -- "$t" $F)"; done; grep -nEi 'gh[pousr]_[A-Za-z0-9]' $F | head
```

<details><summary>Result</summary>

```
1652 ai-sessions/22-npm-audit-mock-brief-notice.md
== [redacted name]: 2
== [redacted name]: 0
== [redacted name]: 0
== #: 32
== Extra: 2
== terms: 4
== redacted: 6
== from: 15
== ai-sessions: 3
== exports: 3
== (gitignored).: 0
== One: 46
== per: 36
== line.: 16
== [redacted]: 0
== [redacted]: 0
== sk-ant-: 1
== ~: 0
```
</details>

The shell split `.scrub-terms` comment lines into separate words, so most of those counts are noise. The real terms (`[redacted]`, `[redacted]`) have 0 hits. Checking the `[redacted name]` and `sk-ant-` hits:

**Bash** — Inspect leak-check hits and turn headings

```bash
F=ai-sessions/22-npm-audit-mock-brief-notice.md; grep -v '^#' .scrub-terms; grep -ni "[redacted name]\b\|[redacted name] " $F | grep -i "[redacted name]" | head; grep -n "sk-ant-" $F; grep -ni "[user]" $F | head -3; head -5 $F; grep -c "^## User\|^### User" $F; grep -c "^## Claude\|^### Claude" $F; grep -n "^## \|^### " $F | head -5
```

<details><summary>Result</summary>

```
[redacted]
[redacted]
1651:python3 -I scripts/transcript_to_md.py ~/.claude/projects/-Users-[user]-Documents-Projects-smart-campaign-composer/7ccc96e5-b242-446f-9a82-97185bf4c606.jsonl ai-sessions/22-npm-audit-mock-brief-notice.md "AI session 22 — npm audit fixes and mock Brief notice" 2>&1 | tail -5; F=ai-sessions/22-npm-audit-mock-brief-notice.md; wc -l $F; for t in $(git config user.name) $(cat .scrub-terms) 'sk-ant-' '~' ; do echo "== $t: $(grep -ic -- "$t" $F)"; done; grep -nEi 'gh[pousr]_[A-Za-z0-9]' $F | head
# AI session 22 — npm audit fixes and mock Brief notice

<!-- session: 7ccc96e5-b242-446f-9a82-97185bf4c606 -->

Claude Code session export (desktop app). Thinking blocks and internal UI tool calls are omitted; tool outputs are truncated; secrets and personal data are redacted.
0
0
9:## 🧑 User
15:## 🤖 Claude
369:## 🧑 User
377:## 🤖 Claude
1482:## 🧑 User
```
</details>

**Bash** — Show context of name and key-prefix hits; count turns

```bash
F=ai-sessions/22-npm-audit-mock-brief-notice.md; grep -noi ".\{25\}[redacted name].\{25\}" $F; grep -noi ".\{20\}sk-ant-.\{20\}" $F; grep -c "^## 🧑 User" $F; grep -c "^## 🤖 Claude" $F
```

<details><summary>Result</summary>

```
1651:(cat .scrub-terms) 'sk-ant-' '~' ; do echo "== 
3
3
```
</details>

**Bash** — Show every line containing the first-name fragment

```bash
grep -ni "[redacted name]" ai-sessions/22-npm-audit-mock-brief-notice.md | cut -c1-200
```

<details><summary>Result</summary>

```
432:**Bash** — Locate mock LLM files and UI badge references
732:	defer tick.Stop()
```
</details>

The notice and the more forgiving example-Brief match are in. `make test` passed (all 8 Go packages, 62 Vitest tests), and both commits are pushed to `main`.

**What changed**
- **Matching** ([example.go](backend/internal/llm/example.go)): the example Brief now matches when the whitespace is different, including tabs, newlines and non-breaking spaces. It also matches with curly or backtick apostrophes, curly double quotes or `«»`, and with quotes around the whole Brief. A changed city or a missing sentence still counts as a different Brief. Unit tests cover these cases.
- **Notice**: every Campaign response now has a `mock_brief_unrecognised` flag, also set when the audience is regenerated. It's only true when the mock is running and the Brief isn't the example. The composer then shows "Mock LLM yalnızca örnek brief'i tanır; gerçek sonuç için ANTHROPIC_API_KEY girin." above the audience. The mock still makes its usual guessed proposal; the notice says it's only a guess. Backend and component tests cover this.
- **README**: the Setup section now has a paragraph explaining this.

**Commits**
- `edbca0a` is the npm audit fix from earlier, committed on its own.
- `897b221` is this change.

**Port conflict during `make test`:** port 55432 was held by another Compose project's Postgres (`test-clone-postgres-1`), so the first run stopped before any test ran. I didn't touch that container. I ran the suite against this project's Postgres on 55433 instead, which is still running.

**Session export:** it's in `ai-sessions/22-npm-audit-mock-brief-notice.md`, made from `~/.claude/projects/-Users-[user]-Documents-Projects-smart-campaign-composer/7ccc96e5-b242-446f-9a82-97185bf4c606.jsonl`. It has 3 user turns and 3 Claude turns, and the leak check found nothing.

---

## 🧑 User

Commit and push the session export to main.

---

## 🤖 Claude

**Bash** — Re-export session and rerun the leak check

```bash
F=ai-sessions/22-npm-audit-mock-brief-notice.md && python3 -I scripts/transcript_to_md.py ~/.claude/projects/-Users-[user]-Documents-Projects-smart-campaign-composer/7ccc96e5-b242-446f-9a82-97185bf4c606.jsonl $F "AI session 22 — npm audit fixes and mock Brief notice" && for t in [redacted name] $(grep -v '^#' .scrub-terms) sk-ant- ~; do echo "== $t: $(grep -ic -- "$t" $F)"; done; grep -nEi 'gh[pousr]_[A-Za-z0-9]{6}' $F | head -3; grep -ni "[redacted name]\|[redacted name]\|[redacted name]" $F | cut -c1-160; grep -c "^## 🧑 User" $F; grep -c "^## 🤖 Claude" $F
```
