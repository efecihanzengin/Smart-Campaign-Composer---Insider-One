# AI session 14 — Ticket #14 rate limiting (subagent)

<!-- session: agent-a326591a31f810997 -->

Claude Code session export (desktop app). Thinking blocks and internal UI tool calls are omitted; tool outputs are truncated; secrets and personal data are redacted.

---

## 🧑 Orchestrator

You are an implementer for GitHub issue #14 ("Per-Channel rate limiting"). Parent spec: issue #2. Read them with `gh issue view`, plus GLOSSARY.md, docs/adr/ (esp. ADR-0006), and <scratchpad> (direct hooks in backend/internal/worker/worker.go: take a token in `drainBatch` just before `w.send`). Another implementer is doing #13 (retries, Attempt log, /metrics inside `send` and the mock Provider) in parallel; keep your changes to the token bucket and its wiring in `drainBatch`, and don't rewrite `send`. The "halve on a burst of 429s and recover gradually" behaviour needs a signal from send results — expose a small hook on the bucket (e.g. `Observe429()`) and call it at the narrowest point, so #13's changes merge easily.

Steps:
1. Confirm your worktree is based on `origin/spec-2-core-flow` (fetch; `git merge-base --is-ancestor origin/spec-2-core-flow HEAD`); if not, `git reset --hard origin/spec-2-core-flow`. It must contain commit fc18733 (#12).
2. Call the Skill tool with `mattpocock-skills:tdd` and build test-first: an HTTP/worker test asserting no Channel exceeds the configured rate (measured from what the Provider received), plus small table tests for the bucket if clearer. Keep rates configurable so tests are fast.
3. Run tests with `COMPOSE_PROJECT_NAME=smart-campaign-composer make test`.
4. One commit titled `Per-Channel rate limiting (#14)` with a summary body ending with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Don't push or close the issue.
5. Before reporting, fetch and merge `origin/spec-2-core-flow` into your branch; resolve conflicts and re-run tests if anything came in.

Report in under 120 words: branch, SHA, worktree path, anything #17 should know. Longer notes go to the notes dir as ticket-14.md.

_(skill instructions loaded: skill)_

---

## 🤖 Claude

**Bash** — 

```bash
git fetch -q && git merge-base --is-ancestor origin/spec-2-core-flow HEAD; echo $status; git log --oneline -3 origin/spec-2-core-flow
```

<details><summary>Error</summary>

```
This agent is isolated in the worktree ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a326591a31f810997, but this command names git in a form too complex to verify that it stays inside the worktree. Refusing to run it — a worktree-isolated agent's git operations must target its own worktree. Split it into plain, separate commands and run them from ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a326591a31f810997.
```
</details>

**Bash** — 

```bash
git fetch -q
```

**Bash** — 

```bash
git log --oneline -3 origin/spec-2-core-flow
```

<details><summary>Result</summary>

```
(Bash completed with no output)
```
</details>

<details><summary>Result</summary>

```
9f5a96f Merge branch 'worktree-agent-ab40c40da8c9ccbf9' into spec-2-core-flow
fc18733 Sending happy path and live dashboard (#12)
7adb1d6 Merge origin/spec-2-core-flow (#11 approval) into #10 regeneration
```
</details>

**Bash** — 

```bash
git merge-base --is-ancestor origin/spec-2-core-flow HEAD && echo yes || echo no
```

<details><summary>Result</summary>

```
no
```
</details>

**Bash** — 

```bash
git reset --hard origin/spec-2-core-flow
```

**Bash** — 

```bash
gh issue view 14; gh issue view 2 | head -150
```

**Bash** — 

```bash
cat <scratchpad> ls docs/adr; cat docs/adr/*0006*
```

<details><summary>Result</summary>

```
HEAD is now at 9f5a96f Merge branch 'worktree-agent-ab40c40da8c9ccbf9' into spec-2-core-flow
```
</details>

<details><summary>Result</summary>

```
title:	Per-Channel rate limiting
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
blocked-by:	[redacted]/Smart-Campaign-Composer---Insider-One#12
blocking:	[redacted]/Smart-Campaign-Composer---Insider-One#17
number:	14
--
## Parent

#2

## What to build

Each Channel sends at no more than 100 messages per second. One token bucket per Channel is shared by that Channel's sending in the worker process. On a burst of 429s, the bucket's rate halves and then recovers gradually towards 100/s. Horizontal scaling (a distributed limiter) is out of scope.

## Acceptance criteria

- [ ] Sending a few thousand Deliveries never exceeds 100 Provider requests per second per Channel (measured on the Provider side)
- [ ] SMS and email limits are independent: a busy SMS Channel doesn't slow email
- [ ] A burst of 429s lowers the effective rate, and it recovers once 429s stop
- [ ] The limit is configurable, so tests can use a small limit and run quickly

title:	Spec: Smart Campaign Composer — core flow
state:	OPEN
author:	[redacted] ([redacted name])
labels:	ready-for-agent
comments:	0
assignees:	
projects:	
milestone:	
issue-type:	
… (141 more lines)
```
</details>

<details><summary>Result</summary>

```
# Ticket #12 notes

Branch worktree-agent-ab40c40da8c9ccbf9, commit fc18733 on top of origin/spec-2-core-flow (5b6a46f, has #11).
Nothing new came in on the merge before reporting. `COMPOSE_PROJECT_NAME=smart-campaign-composer make test` is green,
and the sending tests also pass with -race.

## What exists
- `internal/provider` (`provider.New()` is an http.Handler; cmd/provider just serves it).
  - POST /sms, /email: Idempotency-Key required (400 otherwise). Dedup per (channel, key): the first request decides
    the response (`202 {"id":"sms-N","status":"accepted"}`), repeats replay it and bump `requests`.
  - GET /messages: `{messages:[{channel, idempotency_key, id, requests, payload}]}`. Tests count duplicates here.
- `internal/worker` (`worker.New(pool, Options).Run(ctx)`). Options: ProviderURL, PollInterval (200ms), BatchSize (100),
  Concurrency (16 in flight per Channel), Lease (30s), Timeout (5s, http.Client timeout), RetryDelay (1s).
  - One goroutine per Channel: `drainBatch` = `claim` -> `send` each (bounded concurrency) -> `completeCampaigns`.
  - `claim`: one tx, CTE `SELECT ... FOR UPDATE OF d SKIP LOCKED` on due pending Deliveries of Campaigns with status
    approved|sending, UPDATE to sending, attempt_count+1, lease_expires_at = now()+Lease; then campaigns approved->sending.
  - `send`: POST frozen payload with `Idempotency-Key: <delivery id>`. 2xx -> sent. Anything else -> back to pending
    with next_attempt_at = now()+RetryDelay (placeholder, unbounded; #13 replaces it). DB writes after a request use
    context.WithoutCancel; a request cut off by shutdown leaves the row in sending (for #15's reclaim).
  - `completeCampaigns`: approved|sending with Deliveries and none pending|sending -> completed.
- API: `GET /api/campaigns/{id}` -> `{id, brief, status, deliveries:{sms:{pending,sent,failed,total}}, progress}`
  (pending counts pending+sending; only Channels with Deliveries appear). `GET /api/dashboard` SSE: `dashboard` event
  `{campaigns:[...same shape]}` for the 20 newest non-draft Campaigns, sent on open and whenever the JSON changes,
  polled every `Options.DashboardInterval` (500ms default). Shared query: `campaignsProgress` in httpapi/dashboard.go.
- Web: `subscribeDashboard` in api.ts (EventSource), `CampaignDashboard.vue` mounted in App.vue. Tests stub
  EventSource (App.test.ts uses a silent stub).
- Test helpers (httpapi/sending_test.go): startProvider, startWorkers(t, pool, url, n), waitForStatus, getCampaign,
  providerMessages, assertOneRequestPerDelivery. dashboard_test.go: openDashboard(...).next().

## Pointers
- #13 retries: replace the non-2xx branch of `worker.send` with classification (429 + Retry-After, 5xx, timeout ->
  backoff with jitter via next_attempt_at; other 4xx and attempt_count >= 5 -> failed). Record the Attempt there
  (needs an attempts table migration 0006). Provider behaviour hooks into `provider.accept` where the first response is
  decided (scripted mode = an Options func returning code/headers per request). Note that a 429/5xx should probably
  not be memoised for the key, only accepted results. /metrics counters also belong in `send`.
  The dashboard already shows failed counts.
- #14 token bucket: take a token in `drainBatch` right before `w.send` (comment marks the spot). One bucket per
  Channel shared by the Worker; feed 429s from `send` into it. Concurrency 16 will exceed 100/s with a fast Provider
  until this exists.
- #15 leases: lease is already set on claim; add a reclaim (sending AND lease_expires_at < now() -> pending, or claim
… (28 more lines)
```
</details>

**Bash** — 

```bash
cat backend/internal/worker/worker.go; ls backend/internal/worker backend/internal/httpapi; grep -n "Rate\|rate" GLOSSARY.md
```

<details><summary>Result</summary>

```
// Package worker sends Deliveries to the Provider. Postgres is the queue
// (ADR-0001): each Channel's loop claims due `pending` Deliveries with
// `SELECT … FOR UPDATE SKIP LOCKED`, so any number of workers can run side by
// side without claiming the same Delivery, and every request carries the
// Delivery id as its Idempotency-Key (ADR-0006).
package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Channels are the Channels the worker sends on, each with its own loop.
var Channels = []string{"sms", "email"}

// Options configures a Worker. Zero values take the defaults below.
type Options struct {
	ProviderURL string
	// PollInterval is how long a Channel's loop sleeps when nothing is due.
	PollInterval time.Duration
	// BatchSize is how many Deliveries one claim takes.
	BatchSize int
	// Concurrency is how many requests a Channel has in flight at once.
	Concurrency int
	// Lease is how long a claimed Delivery is held before another worker may
	// reclaim it (reclaiming arrives with #15).
	Lease time.Duration
	// Timeout bounds one Provider request.
… (230 more lines)
```
</details>

**Bash** — 

```bash
cat backend/internal/httpapi/sending_test.go | head -150; ls backend/internal/provider; cat Makefile; grep -n "Worker\|worker\." backend/cmd/campaign/*.go; cat backend/go.mod
```

<details><summary>Result</summary>

```
package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"campaigncomposer/internal/httpapi"
	"campaigncomposer/internal/provider"
	"campaigncomposer/internal/worker"

	"github.com/jackc/pgx/v5/pgxpool"
)

type channelCounts struct {
	Pending int `json:"pending"`
	Sent    int `json:"sent"`
	Failed  int `json:"failed"`
	Total   int `json:"total"`
}

type campaignState struct {
	ID         int64                    `json:"id"`
	Status     string                   `json:"status"`
	Deliveries map[string]channelCounts `json:"deliveries"`
}

func getCampaign(t *testing.T, baseURL string, id int64) campaignState {
	t.Helper()
	var c campaignState
	if code := getJSON(t, baseURL+"/api/campaigns/"+strconv.FormatInt(id, 10), &c); code != http.StatusOK {
		t.Fatalf("GET campaign: status code %d", code)
	}
	return c
}

// startProvider serves the real mock Provider in-process.
… (182 more lines)
```
</details>

**Skill** `mattpocock-skills:tdd`

_(skill instructions loaded: skills)_

**Bash** — 

```bash
cd backend; grep -n "func reviewedCampaign" -A40 internal/httpapi/*_test.go | head -60; cat internal/config/*.go | head -80; sed -n 75,95p cmd/campaign/main.go; grep -rn "PROVIDER" ../.env.example ../docker-compose.yml
```

<details><summary>Result</summary>

```
internal/httpapi/approve_test.go:41:func reviewedCampaign(t *testing.T) (baseURL string, pool *pgxpool.Pool, c campaign) {
internal/httpapi/approve_test.go-42-	t.Helper()
internal/httpapi/approve_test.go-43-	srv, pool := startAPIWithPool(t, httpapi.Options{LLM: llm.NewMock()})
internal/httpapi/approve_test.go-44-	c = createCampaign(t, srv.URL, exampleBrief)
internal/httpapi/approve_test.go-45-	lastDone(t, streamContent(t, srv.URL, c.ID))
internal/httpapi/approve_test.go-46-	return srv.URL, pool, c
internal/httpapi/approve_test.go-47-}
internal/httpapi/approve_test.go-48-
internal/httpapi/approve_test.go-49-func deliveryCount(t *testing.T, pool *pgxpool.Pool, id int64, channel string) int {
internal/httpapi/approve_test.go-50-	t.Helper()
internal/httpapi/approve_test.go-51-	var n int
internal/httpapi/approve_test.go-52-	queryRow(t, pool, `SELECT count(*) FROM deliveries WHERE campaign_id = $1 AND channel = $2`, []any{id, channel}, &n)
internal/httpapi/approve_test.go-53-	return n
internal/httpapi/approve_test.go-54-}
internal/httpapi/approve_test.go-55-
internal/httpapi/approve_test.go-56-func exec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
internal/httpapi/approve_test.go-57-	t.Helper()
internal/httpapi/approve_test.go-58-	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
internal/httpapi/approve_test.go-59-		t.Fatalf("%s: %v", sql, err)
internal/httpapi/approve_test.go-60-	}
internal/httpapi/approve_test.go-61-}
internal/httpapi/approve_test.go-62-
internal/httpapi/approve_test.go-63-func TestApprovalFreezesTheAudienceIntoOnePendingDeliveryPerRecipientAndChannel(t *testing.T) {
internal/httpapi/approve_test.go-64-	baseURL, pool, c := reviewedCampaign(t)
internal/httpapi/approve_test.go-65-
internal/httpapi/approve_test.go-66-	code, a := approve(t, baseURL, c.ID)
internal/httpapi/approve_test.go-67-
internal/httpapi/approve_test.go-68-	if code != http.StatusOK || a.Status != "approved" {
internal/httpapi/approve_test.go-69-		t.Fatalf("approve: status code %d, body %+v; want 200, approved", code, a)
internal/httpapi/approve_test.go-70-	}
internal/httpapi/approve_test.go-71-	if a.Deliveries.SMS != c.Audience.SMS || a.Deliveries.Email != c.Audience.Email {
internal/httpapi/approve_test.go-72-		t.Errorf("deliveries = %+v, want the Audience %+v", a.Deliveries, c.Audience)
internal/httpapi/approve_test.go-73-	}
internal/httpapi/approve_test.go-74-	if n := deliveryCount(t, pool, c.ID, "sms"); n != c.Audience.SMS {
internal/httpapi/approve_test.go-75-		t.Errorf("sms Deliveries = %d, want %d", n, c.Audience.SMS)
internal/httpapi/approve_test.go-76-	}
internal/httpapi/approve_test.go-77-	if n := deliveryCount(t, pool, c.ID, "email"); n != c.Audience.Email {
internal/httpapi/approve_test.go-78-		t.Errorf("email Deliveries = %d, want %d", n, c.Audience.Email)
internal/httpapi/approve_test.go-79-	}
internal/httpapi/approve_test.go-80-	var notPending, both int
… (90 more lines)
```
</details>

**Bash** — 

```bash
cd backend; grep -n "func startAPIWithPool" -A40 internal/httpapi/*_test.go | head -60; ls internal/db/migrations 2>/dev/null || find . -name "*.sql"
```

<details><summary>Result</summary>

```
internal/httpapi/api_test.go:28:func startAPIWithPool(t *testing.T, opts httpapi.Options) (*httptest.Server, *pgxpool.Pool) {
internal/httpapi/api_test.go-29-	t.Helper()
internal/httpapi/api_test.go-30-	pool := testenv.NewDatabase(t)
internal/httpapi/api_test.go-31-	if err := app.Bootstrap(context.Background(), pool, time.Now()); err != nil {
internal/httpapi/api_test.go-32-		t.Fatalf("bootstrap: %v", err)
internal/httpapi/api_test.go-33-	}
internal/httpapi/api_test.go-34-	srv := httptest.NewServer(httpapi.NewHandler(pool, opts))
internal/httpapi/api_test.go-35-	t.Cleanup(srv.Close)
internal/httpapi/api_test.go-36-	return srv, pool
internal/httpapi/api_test.go-37-}
internal/httpapi/api_test.go-38-
internal/httpapi/api_test.go-39-// queryRow scans one row and fails the test on error, so a broken query can't
internal/httpapi/api_test.go-40-// masquerade as a zero count.
internal/httpapi/api_test.go-41-func queryRow(t *testing.T, pool *pgxpool.Pool, sql string, args []any, dest ...any) {
internal/httpapi/api_test.go-42-	t.Helper()
internal/httpapi/api_test.go-43-	if err := pool.QueryRow(context.Background(), sql, args...).Scan(dest...); err != nil {
internal/httpapi/api_test.go-44-		t.Fatalf("%s: %v", sql, err)
internal/httpapi/api_test.go-45-	}
internal/httpapi/api_test.go-46-}
internal/httpapi/api_test.go-47-
internal/httpapi/api_test.go-48-func getJSON(t *testing.T, url string, into any) int {
internal/httpapi/api_test.go-49-	t.Helper()
internal/httpapi/api_test.go-50-	res, err := http.Get(url)
internal/httpapi/api_test.go-51-	if err != nil {
internal/httpapi/api_test.go-52-		t.Fatalf("GET %s: %v", url, err)
internal/httpapi/api_test.go-53-	}
internal/httpapi/api_test.go-54-	defer res.Body.Close()
internal/httpapi/api_test.go-55-	if err := json.NewDecoder(res.Body).Decode(into); err != nil {
internal/httpapi/api_test.go-56-		t.Fatalf("decode %s: %v", url, err)
internal/httpapi/api_test.go-57-	}
internal/httpapi/api_test.go-58-	return res.StatusCode
internal/httpapi/api_test.go-59-}
internal/httpapi/api_test.go-60-
internal/httpapi/api_test.go-61-func TestHealthzIsHealthyWhenPostgresIsReachable(t *testing.T) {
internal/httpapi/api_test.go-62-	srv := startAPI(t, httpapi.Options{})
internal/httpapi/api_test.go-63-
internal/httpapi/api_test.go-64-	var body struct{ Status string }
internal/httpapi/api_test.go-65-	if code := getJSON(t, srv.URL+"/healthz", &body); code != http.StatusOK {
internal/httpapi/api_test.go-66-		t.Fatalf("status code = %d, want 200", code)
internal/httpapi/api_test.go-67-	}
… (6 more lines)
```
</details>

**Bash** — 

```bash
cd backend; sed -n 1,40p internal/httpapi/approve_test.go; grep -n "segment_rule\|INSERT INTO deliveries" -n internal/httpapi/approve.go | head; cat internal/db/migrations/0005_deliveries.sql 2>/dev/null || cat $(find . -name 0005_deliveries.sql)
```

<details><summary>Result</summary>

```
package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"campaigncomposer/internal/httpapi"
	"campaigncomposer/internal/llm"

	"github.com/jackc/pgx/v5/pgxpool"
)

type approval struct {
	ID         int64  `json:"id"`
	Status     string `json:"status"`
	Deliveries struct {
		SMS   int `json:"sms"`
		Email int `json:"email"`
	} `json:"deliveries"`
	Error   string `json:"error"`
	Reasons []struct {
		Code    string `json:"code"`
		Channel string `json:"channel"`
		Message string `json:"message"`
	} `json:"reasons"`
}

func approve(t *testing.T, baseURL string, id int64) (int, approval) {
	t.Helper()
	var a approval
	code := postJSON(t, baseURL+"/api/campaigns/"+strconv.FormatInt(id, 10)+"/approve", nil, &a)
	return code, a
}

// reviewedCampaign creates the example Campaign and streams its Content, as a
// Marketer does before approving.
… (31 more lines)
```
</details>

**Write** `~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a326591a31f810997/backend/internal/httpapi/ratelimit_test.go` (166 lines)

**Bash** — 

```bash
cd backend; grep -n "func (p\|Decode\|payload" internal/provider/provider.go | head -20
```

<details><summary>Result</summary>

```
37:	Payload        json.RawMessage `json:"payload"`
56:func (p *Provider) ServeHTTP(w http.ResponseWriter, r *http.Request) { p.mux.ServeHTTP(w, r) }
58:func (p *Provider) accept(channel string) http.HandlerFunc {
65:		payload, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
66:		if err != nil || !json.Valid(payload) {
75:			m = &message{Channel: channel, IdempotencyKey: key, Payload: payload}
92:func (p *Provider) list(w http.ResponseWriter, r *http.Request) {
```
</details>

Fine. Red: run the test (compile fails on `RateLimit`, as expected). Let me add a stub field to get a real red.

**Bash** — 

```bash
cd backend; perl -0pi -e 's|(\tRetryDelay time.Duration\n\})|\tRetryDelay time.Duration\n\t// RateLimit is the most Provider requests per second on each Channel.\n\tRateLimit float64\n}|' internal/worker/worker.go; cd ..; COMPOSE_PROJECT_NAME=smart-campaign-composer make postgres >/dev/null 2>&1; cd backend; TEST_DATABASE_URL='postgres://campaign:campaign@localhost:55432/campaign?sslmode=disable' go test -count=1 -run 'RateLimit|429' ./internal/httpapi/ 2>&1 | tail -20
```

<details><summary>Result</summary>

```
2026/10/28 15:49:52 worker: Delivery 1888: status 429, error <nil>; retrying
2026/10/28 15:49:52 worker: Delivery 1890: status 429, error <nil>; retrying
2026/10/28 15:49:52 worker: Delivery 1889: status 429, error <nil>; retrying
2026/10/28 15:49:52 worker: Delivery 1891: status 429, error <nil>; retrying
2026/10/28 15:49:52 worker: Delivery 1892: status 429, error <nil>; retrying
2026/10/28 15:49:52 worker: Delivery 1893: status 429, error <nil>; retrying
2026/10/28 15:49:52 worker: Delivery 1894: status 429, error <nil>; retrying
2026/10/28 15:49:52 worker: Delivery 1896: status 429, error <nil>; retrying
2026/10/28 15:49:52 worker: Delivery 1895: status 429, error <nil>; retrying
2026/10/28 15:49:52 worker: Delivery 1897: status 429, error <nil>; retrying
2026/10/28 15:49:52 worker: Delivery 1898: status 429, error <nil>; retrying
2026/10/28 15:49:52 worker: Delivery 1899: status 429, error <nil>; retrying
2026/10/28 15:49:52 worker: Delivery 1900: status 429, error <nil>; retrying
2026/10/28 15:49:52 worker: Delivery 1901: status 429, error <nil>; retrying
--- FAIL: TestABurstOf429sLowersTheRateWhichRecoversOnceThey (11.80s)
    ratelimit_test.go:159: 1000 requests in the second after the 429s, want at most 20 (half the limit)
    ratelimit_test.go:163: 0 requests in a second long after the 429s stopped, want the rate back near 40
FAIL
FAIL	campaigncomposer/internal/httpapi	14.187s
FAIL
```
</details>

**Bash** — 

```bash
cd backend; TEST_DATABASE_URL='postgres://campaign:campaign@localhost:55432/campaign?sslmode=disable' go test -count=1 -run 'RateLimit|429' ./internal/httpapi/ 2>&1 | grep -v "^20" | tail -20
```

<details><summary>Result</summary>

```
--- FAIL: TestNoChannelExceedsItsRateLimitAndChannelsDontSlowEachOther (1.90s)
    ratelimit_test.go:130: sms: 120 requests within one second, want at most 40
    ratelimit_test.go:130: email: 120 requests within one second, want at most 40
--- FAIL: TestABurstOf429sLowersTheRateWhichRecoversOnceThey (11.78s)
    ratelimit_test.go:159: 1000 requests in the second after the 429s, want at most 20 (half the limit)
    ratelimit_test.go:163: 0 requests in a second long after the 429s stopped, want the rate back near 40
FAIL
FAIL	campaigncomposer/internal/httpapi	13.938s
FAIL
```
</details>

Both red for the right reason. Now the bucket's table tests, with a fake clock.

**Write** `~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a326591a31f810997/backend/internal/worker/bucket_test.go` (102 lines)

**Write** `~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a326591a31f810997/backend/internal/worker/bucket.go` (109 lines)

**Bash** — 

```bash
cd backend; go test -count=1 ./internal/worker/ 2>&1 | tail -20
```

<details><summary>Result</summary>

```
ok  	campaigncomposer/internal/worker	0.257s
```
</details>

**Bash** — 

```bash
cd backend; python3 - <<'EOF'
p='internal/worker/worker.go'
s=open(p).read()
def r(a,b):
    global s
    assert s.count(a)==1, a
    s=s.replace(a,b)
r("""	// RateLimit is the most Provider requests per second on each Channel.
	RateLimit float64
}""","""	// RateLimit is the most Provider requests per second on each Channel,
	// shared by the Worker's sending on that Channel (#14). A burst of 429s
	// lowers it for a while; see Bucket.
	RateLimit float64
}""")
r("""	if o.RetryDelay <= 0 {
		o.RetryDelay = time.Second
	}
	return o""","""	if o.RetryDelay <= 0 {
		o.RetryDelay = time.Second
	}
	if o.RateLimit <= 0 {
		o.RateLimit = 100
	}
	return o""")
r("""	client *http.Client
}""","""	client *http.Client
	// buckets holds one token bucket per Channel; the map is not written
	// after New.
	buckets map[string]*Bucket
}""")
r("""	opts = opts.withDefaults()
	return &Worker{
		pool:   pool,
		opts:   opts,
		client: &http.Client{Timeout: opts.Timeout},
	}""","""	opts = opts.withDefaults()
	buckets := make(map[string]*Bucket, len(Channels))
	for _, ch := range Channels {
		buckets[ch] = NewBucket(opts.RateLimit, nil)
	}
	return &Worker{
		pool:    pool,
		opts:    opts,
		client:  &http.Client{Timeout: opts.Timeout},
		buckets: buckets,
	}""")
r("""			// The token bucket (#14) takes a token here, before each request.
			w.send(ctx, channel, d)""","""			// Shutting down: the claimed Delivery waits out its lease.
			if w.buckets[channel].Wait(ctx) != nil {
				return
			}
			w.send(ctx, channel, d)""")
r("""	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, res.Body)
	return res.StatusCode, nil""","""	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, res.Body)
	if res.StatusCode == http.StatusTooManyRequests {
		w.buckets[channel].Observe429()
	}
	return res.StatusCode, nil""")
open(p,'w').write(s)
EOF
gofmt -l . ; TEST_DATABASE_URL='postgres://campaign:campaign@localhost:55432/campaign?sslmode=disable' go test -count=1 -run 'RateLimit|429' ./internal/httpapi/ 2>&1 | grep -v "^20" | tail -20
```

<details><summary>Result</summary>

```
--- FAIL: TestNoChannelExceedsItsRateLimitAndChannelsDontSlowEachOther (4.84s)
    ratelimit_test.go:130: sms: 41 requests within one second, want at most 40
    ratelimit_test.go:130: email: 41 requests within one second, want at most 40
FAIL
FAIL	campaigncomposer/internal/httpapi	17.013s
FAIL
```
</details>

**Bash** — 

```bash
cd backend; python3 - <<'EOF'
p='internal/httpapi/ratelimit_test.go'
s=open(p).read()
s=s.replace("""		if most := busiestSecond(times); most > limit {""","""		slices.SortFunc(times, func(a, b time.Time) int { return a.Compare(b) })
		minSpan := time.Hour
		for i := limit; i < len(times); i++ {
			minSpan = min(minSpan, times[i].Sub(times[i-limit]))
		}
		t.Logf("%s: shortest span of %d requests %v", ch, limit+1, minSpan)
		if most := busiestSecond(times); most > limit {""")
open(p,'w').write(s)
EOF
for i in 1 2; do TEST_DATABASE_URL='postgres://campaign:campaign@localhost:55432/campaign?sslmode=disable' go test -count=1 -v -run 'RateLimit' ./internal/httpapi/ 2>&1 | grep -E "span|FAIL|ok"; done
```

<details><summary>Result</summary>

```
ratelimit_test.go:134: sms: shortest span of 41 requests 998.534ms
    ratelimit_test.go:134: email: shortest span of 41 requests 998.012875ms
--- FAIL: TestNoChannelExceedsItsRateLimitAndChannelsDontSlowEachOther (4.81s)
FAIL
FAIL	campaigncomposer/internal/httpapi	5.174s
FAIL
    ratelimit_test.go:134: sms: shortest span of 41 requests 998.8215ms
    ratelimit_test.go:134: email: shortest span of 41 requests 998.186416ms
--- FAIL: TestNoChannelExceedsItsRateLimitAndChannelsDontSlowEachOther (4.80s)
FAIL
FAIL	campaigncomposer/internal/httpapi	5.038s
FAIL
```
</details>

Jitter is about 2ms, so pacing exactly at the limit sits on the boundary. I'll pace with 2% headroom (about 20ms per window) in the worker wiring.

**Bash** — 

```bash
cd backend; python3 - <<'EOF'
p='internal/httpapi/ratelimit_test.go'
s=open(p).read()
s=s.replace("""		slices.SortFunc(times, func(a, b time.Time) int { return a.Compare(b) })
		minSpan := time.Hour
		for i := limit; i < len(times); i++ {
			minSpan = min(minSpan, times[i].Sub(times[i-limit]))
		}
		t.Logf("%s: shortest span of %d requests %v", ch, limit+1, minSpan)
""","")
open(p,'w').write(s)
p='internal/worker/worker.go'
s=open(p).read()
a="""		buckets[ch] = NewBucket(opts.RateLimit, nil)"""
assert a in s
s=s.replace(a,"""		buckets[ch] = NewBucket(opts.RateLimit*paceHeadroom, nil)""")
a="""// Worker claims and sends Deliveries."""
s=s.replace(a,"""// paceHeadroom paces requests 2% under RateLimit. Pacing exactly at the
// limit puts limit+1 requests exactly one second apart, so a few ms of
// network jitter would let the Provider see limit+1 within a second; 2%
// leaves about 20ms of slack.
const paceHeadroom = 0.98

"""+a)
open(p,'w').write(s)
EOF
grep -n "slices\." internal/httpapi/ratelimit_test.go | head -3; TEST_DATABASE_URL='postgres://campaign:campaign@localhost:55432/campaign?sslmode=disable' go test -count=1 -race -run 'RateLimit|429' ./internal/httpapi/ 2>&1 | grep -v "^20" | tail
```

<details><summary>Result</summary>

```
34:	return slices.Clone(a.times[channel])
75:	slices.SortFunc(times, func(a, b time.Time) int { return a.Compare(b) })
ok  	campaigncomposer/internal/httpapi	18.787s
```
</details>

Green, including under `-race`. Now I'll wire the limit to env config.

**Bash** — 

```bash
cd ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a326591a31f810997; cat .env.example; gh issue view 17 | tail -20
```

<details><summary>Result</summary>

```
# Copy to .env; docker compose reads it automatically.

# Claude API key. When set, the Segment and Content calls go to Claude; leave it
# empty to run on the scripted mock LLM (the UI then shows a "Mock LLM" badge).
# The key is never logged, and tests never need it.
ANTHROPIC_API_KEY=

[REDACTED] Claude model used when ANTHROPIC_API_KEY is set. Defaults to claude-sonnet-5-5;
# set it to override (e.g. claude-opus-5-5).
ANTHROPIC_MODEL=claude-sonnet-5-5

# Host ports published by docker compose (all bound to 127.0.0.1).
WEB_PORT=5173
API_PORT=8080
POSTGRES_PORT=55432

# Set by docker compose for each container; only needed when running the Go
# binaries outside compose.
# DATABASE_URL=postgres://campaign:campaign@localhost:55432/campaign?sslmode=disable
# HTTP_ADDR=:8080
# PROVIDER_URL=http://localhost:8081
blocking:	
number:	17
--
## Parent

#2

## What to build

Make the submission reviewable. The README covers one-command setup, using the mock vs the real model, `make test`, a short architecture summary (five containers, Postgres as the queue, the two-call LLM flow, how duplicates are prevented) and what was not done. `DECISIONS.md` indexes every ADR, including any added during implementation. Finally, a full demo pass of the case's example Brief on a fresh `docker compose up`, with and without an API key, fixing whatever rough edges turn up.

## Acceptance criteria

- [ ] The README covers setup, mock vs real model, `make test`, the architecture summary and what was not done
- [ ] `DECISIONS.md` links every ADR in `docs/adr/`
- [ ] On a fresh clone, `docker compose up` followed by the example Brief → approve → completed works end to end with the mock LLM
- [ ] The same flow works with a real `ANTHROPIC_API_KEY`
- [ ] `make test` and CI are green
- [ ] `ai-sessions/` exists, with exports scrubbed of API keys and personal data
```
</details>

**Bash** — 

```bash
cd ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a326591a31f810997; sed -n 20,50p docker-compose.yml; python3 - <<'EOF'
p='backend/internal/config/config.go'
import glob
p=[f for f in glob.glob('backend/internal/config/*.go') if not f.endswith('_test.go')][0]
s=open(p).read()
s=s.replace('import "os"','import (\n\t"os"\n\t"strconv"\n)')
s=s.replace("""	ProviderURL     string
}""","""	ProviderURL     string
	// RateLimit is the most Provider requests per second per Channel.
	RateLimit float64
}""")
s=s.replace("""		ProviderURL:     env("PROVIDER_URL", "http://localhost:8081"),
	}""","""		ProviderURL:     env("PROVIDER_URL", "http://localhost:8081"),
		RateLimit:       positiveFloat("RATE_LIMIT_PER_SECOND", 100),
	}""")
s+='''
// positiveFloat reads a positive number, falling back when it is unset or
// not a positive number.
func positiveFloat(key string, fallback float64) float64 {
	v, err := strconv.ParseFloat(os.Getenv(key), 64)
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}
'''
open(p,'w').write(s)
p=[f for f in glob.glob('backend/internal/config/*_test.go')][0]
s=open(p).read()
s+='''
func TestRateLimitDefaultsTo100PerSecond(t *testing.T) {
	t.Setenv("RATE_LIMIT_PER_SECOND", "")
	if got := config.FromEnv().RateLimit; got != 100 {
		t.Errorf("rate limit = %v, want 100", got)
	}
}

func TestRateLimitIsConfigurable(t *testing.T) {
	t.Setenv("RATE_LIMIT_PER_SECOND", "20")
	if got := config.FromEnv().RateLimit; got != 20 {
		t.Errorf("rate limit = %v, want 20", got)
	}
}
'''
open(p,'w').write(s)
p='backend/cmd/campaign/main.go'
s=open(p).read()
a="worker.Options{ProviderURL: cfg.ProviderURL}"
assert a in s
s=s.replace(a,"worker.Options{ProviderURL: cfg.ProviderURL, RateLimit: cfg.RateLimit}")
s=s.replace('''	log.Printf("worker sending to the Provider at %s", cfg.ProviderURL)''','''	log.Printf("worker sending to the Provider at %s, at most %v requests/s per Channel", cfg.ProviderURL, cfg.RateLimit)''')
open(p,'w').write(s)
p='.env.example'
s=open(p).read()
s=s.replace("""# Host ports published""","""# Most Provider requests per second on each Channel (SMS and email are limited
# independently). A burst of 429s halves it for a while; it then recovers.
RATE_LIMIT_PER_SECOND=100

# Host ports published""")
open(p,'w').write(s)
EOF
gofmt -l backend; git diff --stat
```

<details><summary>Result</summary>

```
command: ["campaign", "api"]
    environment:
      DATABASE_URL: postgres://campaign:campaign@postgres:5432/campaign?sslmode=disable
      HTTP_ADDR: :8080
      ANTHROPIC_API_KEY: ${ANTHROPIC_API_KEY:-}
      ANTHROPIC_MODEL: ${ANTHROPIC_MODEL:-}
      PROVIDER_URL: http://provider:8081
    depends_on:
      postgres:
        condition: service_healthy
    ports:
      - "127.0.0.1:${API_PORT:-8080}:8080"
    healthcheck:
      test: ["CMD", "wget", "-q", "-O", "/dev/null", "http://localhost:8080/healthz"]
      interval: 3s
      timeout: 3s
      retries: 40
      start_period: 5s
    restart: unless-stopped

  worker:
    build: ./backend
    command: ["campaign", "worker"]
    environment:
      DATABASE_URL: postgres://campaign:campaign@postgres:5432/campaign?sslmode=disable
      PROVIDER_URL: http://provider:8081
    depends_on:
      api:
        # The API runs migrations and the seed before it reports healthy.
        condition: service_healthy
      provider:
 .env.example                           |  4 ++++
 backend/cmd/campaign/main.go           |  4 ++--
 backend/internal/config/config.go      | 18 ++++++++++++++++-
 backend/internal/config/config_test.go | 14 ++++++++++++++
 backend/internal/worker/worker.go      | 35 ++++++++++++++++++++++++++++++----
 5 files changed, 68 insertions(+), 7 deletions(-)
```
</details>

**Bash** — 

```bash
cd ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a326591a31f810997; perl -0pi -e 's|(      DATABASE_URL: postgres://campaign:campaign\@postgres:5432/campaign\?sslmode=disable\n      PROVIDER_URL: http://provider:8081\n)|$1      RATE_LIMIT_PER_SECOND: \${RATE_LIMIT_PER_SECOND:-}\n|' docker-compose.yml; git diff docker-compose.yml; COMPOSE_PROJECT_NAME=smart-campaign-composer make test 2>&1 | grep -vE "^20[0-9]{2}/" | tail -25
```

<details><summary>Result</summary>

```
diff --git a/docker-compose.yml b/docker-compose.yml
index c794d18..4cbaecd 100644
--- a/docker-compose.yml
+++ b/docker-compose.yml
@@ -43,6 +43,7 @@ services:
     environment:
       DATABASE_URL: postgres://campaign:campaign@postgres:5432/campaign?sslmode=disable
       PROVIDER_URL: http://provider:8081
+      RATE_LIMIT_PER_SECOND: ${RATE_LIMIT_PER_SECOND:-}
     depends_on:
       api:
         # The API runs migrations and the seed before it reports healthy.

 RUN  v3.2.4 ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a326591a31f810997/web

(node:81620) ExperimentalWarning: localStorage is not available because --localstorage-file was not provided.
(Use `node --trace-warnings ...` to show where the warning was created)
(node:81616) ExperimentalWarning: localStorage is not available because --localstorage-file was not provided.
(Use `node --trace-warnings ...` to show where the warning was created)
(node:81617) ExperimentalWarning: localStorage is not available because --localstorage-file was not provided.
(Use `node --trace-warnings ...` to show where the warning was created)
(node:81618) ExperimentalWarning: localStorage is not available because --localstorage-file was not provided.
(Use `node --trace-warnings ...` to show where the warning was created)
(node:81615) ExperimentalWarning: localStorage is not available because --localstorage-file was not provided.
(Use `node --trace-warnings ...` to show where the warning was created)
 ✓ src/components/CampaignDashboard.test.ts (7 tests) 64ms
 ✓ src/App.test.ts (4 tests) 53ms
 ✓ src/components/CampaignComposer.test.ts (9 tests) 108ms
 ✓ src/components/Regeneration.test.ts (6 tests) 147ms
 ✓ src/components/ApprovalPanel.test.ts (11 tests) 147ms
 ✓ src/components/ContentReview.test.ts (14 tests) 185ms

 Test Files  6 passed (6)
      Tests  51 passed (51)
   Start at  15:54:43
   Duration  977ms (transform 338ms, setup 0ms, collect 1.09s, tests 704ms, environment 1.90s, prepare 365ms)
```
</details>

**Bash** — 

```bash
cd ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a326591a31f810997/backend; TEST_DATABASE_URL='postgres://campaign:campaign@localhost:55432/campaign?sslmode=disable' go test -count=1 ./... 2>&1 | grep -vE "^20[0-9]{2}/" | tail -15; go vet ./...
```

<details><summary>Result</summary>

```
?   	campaigncomposer/cmd/campaign	[no test files]
?   	campaigncomposer/cmd/provider	[no test files]
ok  	campaigncomposer/internal/app	2.547s
ok  	campaigncomposer/internal/config	0.111s
ok  	campaigncomposer/internal/content	0.118s
?   	campaigncomposer/internal/db	[no test files]
ok  	campaigncomposer/internal/httpapi	92.249s
ok  	campaigncomposer/internal/llm	2.642s
ok  	campaigncomposer/internal/provider	0.159s
?   	campaigncomposer/internal/seed	[no test files]
ok  	campaigncomposer/internal/segment	1.562s
?   	campaigncomposer/internal/testenv	[no test files]
ok  	campaigncomposer/internal/worker	0.141s
```
</details>

Everything passes. I'll fix the test name, then commit.

**Bash** — 

```bash
cd ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a326591a31f810997; sed -i '' 's/TestABurstOf429sLowersTheRateWhichRecoversOnceThey/TestABurstOf429sLowersTheRateAndItRecoversOnceTheyStop/' backend/internal/httpapi/ratelimit_test.go; git add -A; git status --short
```

<details><summary>Result</summary>

```
M  .env.example
M  backend/cmd/campaign/main.go
M  backend/internal/config/config.go
M  backend/internal/config/config_test.go
A  backend/internal/httpapi/ratelimit_test.go
A  backend/internal/worker/bucket.go
A  backend/internal/worker/bucket_test.go
M  backend/internal/worker/worker.go
M  docker-compose.yml
[This command modified 1 file you've previously read: backend/internal/httpapi/ratelimit_test.go. Call Read before editing.]
```
</details>

**Bash** — 

```bash
cd ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a326591a31f810997; git commit -q -F - <<'EOF'
Per-Channel rate limiting (#14)

Each Channel's sending in the worker now goes through its own token bucket
(worker.Bucket): drainBatch takes a token just before each send, so SMS and
email are limited independently. The bucket holds one token, so requests are
paced evenly rather than bursting, and the worker paces 2% under the limit so
network jitter can't let the Provider see limit+1 requests within a second.

A 429 from the Provider calls Bucket.Observe429 (in post, where the status
code is read): a burst of 429s halves the rate at most once per second, down
to 1/s; once 429s stop for a second the rate climbs back by a fifth of the
limit per second.

The limit is Options.RateLimit (default 100/s), set from
RATE_LIMIT_PER_SECOND for the worker process. Tests measure arrival times on
the Provider side: no Channel exceeds a 40/s limit in any one-second window,
both Channels send side by side, and a second of 429s lowers the rate, which
recovers afterwards. Bucket table tests use a fake clock.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
git fetch -q; git merge origin/spec-2-core-flow 2>&1 | tail -3; git log --oneline -3
```

<details><summary>Result</summary>

```
Already up to date.
6d1875a Per-Channel rate limiting (#14)
9f5a96f Merge branch 'worktree-agent-ab40c40da8c9ccbf9' into spec-2-core-flow
fc18733 Sending happy path and live dashboard (#12)
```
</details>

Nothing new came in from the merge. Next I'll write the notes file.

**Write** `<scratchpad>` (36 lines)

**SubagentHandback**

<details><summary>Result</summary>

```
{"success":true,"message":"Report delivered to your caller."}
```
</details>
