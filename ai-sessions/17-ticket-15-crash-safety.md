# AI session 17 — Ticket #15 crash safety (subagent)

<!-- session: agent-a785d3de5beeae594 -->

Claude Code session export (desktop app). Thinking blocks and internal UI tool calls are omitted; tool outputs are truncated; secrets and personal data are redacted.

---

## 🧑 Orchestrator

You are an implementer for GitHub issue #15 ("Crash safety: leases and zero duplicates"). Parent spec: issue #2. Read them with `gh issue view`, plus GLOSSARY.md, docs/adr/ (esp. ADR-0006), and the notes in <scratchpad> — ticket-12.md, ticket-13.md, ticket-14.md, ticket-16.md. Known issues to address from those notes: (a) on shutdown an errored request leaves its Delivery in `sending` with attempt_count raised and no Attempt row; (b) `send`'s UPDATEs are guarded only by `state='sending'` — add an `attempt_count` (or lease) guard so a slow original worker can't overwrite a reclaimed Delivery; (c) lease must exceed Provider timeout; (d) at a lowered rate a 100-Delivery batch can outlast the lease — size batches or renew leases accordingly.

Steps:
1. Confirm your worktree is based on `origin/spec-2-core-flow` (fetch; `git merge-base --is-ancestor origin/spec-2-core-flow HEAD`); if not, `git reset --hard origin/spec-2-core-flow`. It must contain 79be184.
2. Call the Skill tool with `mattpocock-skills:tdd` and build test-first through the HTTP API seam with worker in-process and the mock Provider scripted: zero duplicates (what the Provider received, per Idempotency-Key and per (Campaign, Recipient, Channel)) under scripted 429s, 5xx, timeouts-after-accept and a worker restart mid-send; lease reclaim of Deliveries stuck in `sending`. Short lease/backoff/timeout via config.
3. Run tests with `COMPOSE_PROJECT_NAME=smart-campaign-composer make test`; also run the worker/httpapi packages with `-race` a few times to check for flakiness.
4. One commit titled `Crash safety: leases and zero duplicates (#15)` with a summary body ending with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Don't push or close the issue.
5. Before reporting, fetch and merge `origin/spec-2-core-flow` into your branch; resolve conflicts and re-run tests if anything came in.

Report in under 120 words: branch, SHA, worktree path, anything #17 should know. Longer notes go to the notes dir as ticket-15.md.

_(skill instructions loaded: skill)_

---

## 🤖 Claude

**Bash** — 

```bash
git fetch -q origin && (git merge-base --is-ancestor origin/spec-2-core-flow HEAD && echo ANC || echo NOT) ; git log --oneline -1 origin/spec-2-core-flow; git branch --contains 79be184 2>&1 | head -3
```

<details><summary>Result</summary>

```
NOT
79be184 Merge #13 into spec-2-core-flow
+ merge-13
+ spec-2-core-flow
```
</details>

**Bash** — 

```bash
git reset -q --hard origin/spec-2-core-flow && git log --oneline -3 && gh issue view 15 && gh issue view 2 | head -150 && ls docs/adr && N=<scratchpad> ls $N; cat $N/ticket-12.md $N/ticket-13.md $N/ticket-14.md $N/ticket-16.md
```

**Skill** `mattpocock-skills:tdd`

<details><summary>Error</summary>

```
This agent is isolated in the worktree ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a785d3de5beeae594, but this command names git in a form too complex to verify that it stays inside the worktree. Refusing to run it — a worktree-isolated agent's git operations must target its own worktree. Split it into plain, separate commands and run them from ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a785d3de5beeae594.
```
</details>

_(skill instructions loaded: skills)_

**Bash** — 

```bash
git -C ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a785d3de5beeae594 reset --hard origin/spec-2-core-flow
```

**Bash** — 

```bash
gh issue view 15; gh issue view 2 | head -200
```

**Bash** — 

```bash
N=<scratchpad> ls $N; cat $N/ticket-12.md $N/ticket-13.md $N/ticket-14.md $N/ticket-16.md
```

<details><summary>Result</summary>

```
HEAD is now at 79be184 Merge #13 into spec-2-core-flow
```
</details>

<details><summary>Result</summary>

```
title:	Crash safety: leases and zero duplicates
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
blocked-by:	[redacted]/Smart-Campaign-Composer---Insider-One#13
blocking:	[redacted]/Smart-Campaign-Composer---Insider-One#17
number:	15
--
## Parent

#2

## What to build

Prove "the same message never goes to the same person twice" under the failures that usually break it (ADR-0006). Claiming a Delivery sets a lease expiry. A Delivery stuck in `sending` past its lease (e.g. its worker crashed) is reclaimed by another worker. Because every request carries the idempotency key, a second Attempt for a message the Provider already accepted is harmless. Lease duration is a configuration value.

## Acceptance criteria

- [ ] Scripted "Provider accepts, then the response is slower than the client timeout" → the retry gets the original result, the Delivery is `sent`, and the Provider recorded exactly one message
- [ ] A worker killed mid-send leaves Deliveries in `sending`; after the lease expires, another worker completes them
- [ ] Restarting the worker container during a Campaign under the default Provider failure rates ends with every Delivery `sent` or `failed`, and zero duplicate messages recorded by the Provider
- [ ] Lease duration is configurable, so these tests run fast

title:	Spec: Smart Campaign Composer — core flow
state:	OPEN
author:	[redacted] ([redacted name])
labels:	ready-for-agent
comments:	0
assignees:	
projects:	
milestone:	
issue-type:	
… (191 more lines)
```
</details>

<details><summary>Result</summary>

```
ticket-10.md
ticket-11.md
ticket-12.md
ticket-13.md
ticket-14.md
ticket-16.md
ticket-6.md
ticket-7.md
ticket-8.md
ticket-9.md
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
… (134 more lines)
```
</details>

**Bash** — 

```bash
cd ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a785d3de5beeae594; cat docs/adr/0006*; ls internal/worker internal/httpapi; wc -l internal/worker/*.go; cat internal/worker/worker.go
```

<details><summary>Error</summary>

```
Exit code 1
# Idempotency key and claim lease prevent duplicate sends

The unique Delivery per (Campaign, Recipient, Channel) from ADR-0001 stops duplicates at enqueue time, but not at send time. Two failure modes remain: a request that times out after the Provider already accepted it, and a worker that crashes while holding a claimed Delivery. So every Provider request carries `Idempotency-Key: <delivery id>`, and the Provider returns the original result for a key it has already seen. Retrying after a timeout is therefore always safe. Claimed Deliveries also carry a lease expiry: a Delivery stuck in `sending` past its lease is reclaimed by another worker, and the idempotency key makes that second Attempt harmless if the first one actually landed.

## Considered Options

- **Never retry after a timeout**: no duplicates, but Deliveries that never reached the Provider would be lost.
- **Mark as sent before calling the Provider**: no duplicates, but a crash loses the message silently.

## Consequences

The mock Provider must implement idempotency-key deduplication, as real SMS and email providers do. The "zero duplicates" integration test relies on it.
ls: internal/httpapi: No such file or directory
ls: internal/worker: No such file or directory
(eval):1: no matches found: internal/worker/*.go
cat: internal/worker/worker.go: No such file or directory
```
</details>

**Bash** — 

```bash
cd ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a785d3de5beeae594; git ls-files | grep -v '^web/' | grep -v ai-sessions
```

<details><summary>Result</summary>

```
.claude/skills/export-session/SKILL.md
.env.example
.github/workflows/ci.yml
.gitignore
CLAUDE.md
DECISIONS.md
GLOSSARY.md
Makefile
backend/.dockerignore
backend/Dockerfile
backend/cmd/campaign/main.go
backend/cmd/provider/main.go
backend/go.mod
backend/go.sum
backend/internal/app/app.go
backend/internal/app/app_test.go
backend/internal/config/config.go
backend/internal/config/config_test.go
backend/internal/content/content.go
backend/internal/content/email.go
backend/internal/content/email.html
backend/internal/content/partial_test.go
backend/internal/content/sms.go
backend/internal/content/sms_test.go
backend/internal/db/db.go
backend/internal/db/migrations/0001_recipients_events.sql
backend/internal/db/migrations/0002_campaigns.sql
backend/internal/db/migrations/0003_tr_fold.sql
backend/internal/db/migrations/0004_content.sql
backend/internal/db/migrations/0005_deliveries.sql
backend/internal/db/migrations/0006_attempts.sql
backend/internal/httpapi/api_test.go
backend/internal/httpapi/approve.go
backend/internal/httpapi/approve_guards_test.go
backend/internal/httpapi/approve_test.go
backend/internal/httpapi/campaigns.go
backend/internal/httpapi/campaigns_test.go
backend/internal/httpapi/cancel.go
backend/internal/httpapi/cancel_test.go
backend/internal/httpapi/content.go
… (50 more lines)
```
</details>

**Bash** — 

```bash
cd ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a785d3de5beeae594/backend; cat internal/worker/worker.go; cat internal/db/migrations/0005_deliveries.sql; cat internal/config/config.go; grep -n "Worker\|worker" cmd/campaign/main.go; cat ../Makefile
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
	"math/rand/v2"
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
… (500 more lines)
```
</details>

Now the test helpers.

**Bash** — 

```bash
cd ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a785d3de5beeae594/backend; cat internal/httpapi/retries_test.go; sed -n 1,400p internal/httpapi/sending_test.go; sed -n 80,110p cmd/campaign/main.go
```

<details><summary>Result</summary>

```
package httpapi_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"campaigncomposer/internal/provider"
	"campaigncomposer/internal/worker"
)

// startScriptedProvider serves the mock Provider with every request's
// outcome pinned by script.
func startScriptedProvider(t *testing.T, script provider.Script) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(provider.New(provider.Options{Script: script}))
	t.Cleanup(srv.Close)
	return srv
}

// fastRetries keeps backoff and the client timeout small so retry tests run
// in milliseconds. The rate limit is set far out of reach: these scripts
// throttle every Delivery, and the 429 slow-down (#14) would otherwise
// stretch the test to minutes; pacing has its own tests.
func fastRetries(providerURL string) worker.Options {
	return worker.Options{
		ProviderURL:  providerURL,
		PollInterval: 5 * time.Millisecond,
		BackoffBase:  time.Millisecond,
		Timeout:      100 * time.Millisecond,
		RateLimit:    1e6,
	}
}

type attempt struct {
	Number     int       `json:"number"`
… (509 more lines)
```
</details>

**Bash** — 

```bash
cd ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a785d3de5beeae594/backend; cat internal/testenv/testenv.go; grep -n "func reviewedCampaign\|func startAPI\|func approve\b\|func approve(" -A25 internal/httpapi/*_test.go | head -90; sed -n 1,80p internal/provider/provider.go; grep -n "func OptionsFromEnv" -A40 internal/provider/provider.go
```

<details><summary>Result</summary>

```
// Package testenv gives each test its own fresh Postgres database on the
// server named by TEST_DATABASE_URL (the compose Postgres, via `make test`).
package testenv

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewDatabase creates an empty database, returns a pool connected to it, and
// drops it when the test ends.
func NewDatabase(t *testing.T) *pgxpool.Pool {
	t.Helper()
	adminURL := os.Getenv("TEST_DATABASE_URL")
	if adminURL == "" {
		t.Fatal("TEST_DATABASE_URL is not set; run the tests with `make test`")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connect to test Postgres: %v", err)
	}
	defer admin.Close(ctx)

	b := make([]byte, 6)
	_, _ = rand.Read(b)
	name := "test_" + hex.EncodeToString(b)
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("create database: %v", err)
	}

… (216 more lines)
```
</details>

**Bash** — 

```bash
cd ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a785d3de5beeae594/backend; grep -n "^func\|^// " internal/worker/bucket.go | head -40; grep -n "state" internal/httpapi/deliveries.go | head; sed -n 80,210p internal/provider/provider.go; grep -n "exampleBrief\|SMS\b" internal/httpapi/approve_test.go | head -5; cat internal/db/migrations/0006_attempts.sql; grep -n WORKER -B2 -A2 ../.env.example ../docker-compose.yml
```

<details><summary>Result</summary>

```
22:// Bucket is a token bucket that paces one Channel's Provider requests. It
23:// holds at most one token, so requests are spread evenly at the rate and
24:// never burst above it. A burst of 429s halves the rate (at most once per
25:// second); once the 429s stop for a second, the rate climbs back towards the
26:// limit by a fifth of the limit per second.
39:// NewBucket returns a Bucket allowing limit requests per second. now is the
40:// clock; nil means time.Now.
41:func NewBucket(limit float64, now func() time.Time) *Bucket {
48:// Wait blocks until the caller may make one request, or ctx ends.
49:func (b *Bucket) Wait(ctx context.Context) error {
64:// Reserve takes a token and returns how long the caller must wait before
65:// using it.
66:func (b *Bucket) Reserve() time.Duration {
80:// Observe429 reports that the Provider throttled a request on this Channel.
81:func (b *Bucket) Observe429() {
92:// Rate is the current rate in requests per second.
93:func (b *Bucket) Rate() float64 {
99:func (b *Bucket) rateAt(now time.Time) float64 {
24:	State        string        `json:"state"`
30:// operator can see why one was retried or failed. `state` and `channel`
59:		SELECT d.id, d.channel, d.state, d.attempt_count,
66:			AND ($2 = '' OR d.state = $2)
69:		LIMIT $4`, id, q.Get("state"), q.Get("channel"), limit)
	key     string
}

type message struct {
	Channel        string          `json:"channel"`
	IdempotencyKey string          `json:"idempotency_key"`
	ID             string          `json:"id"`
	Requests       int             `json:"requests"`
	Payload        json.RawMessage `json:"payload"`

	// The response returned for this key, replayed for every repeat.
	code int
	body []byte
}

// New returns an empty mock Provider behaving as opts says.
func New(opts Options) *Provider {
… (152 more lines)
```
</details>

**Bash** — 

```bash
cd ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a785d3de5beeae594/backend; sed -n 1,140p internal/httpapi/ratelimit_test.go; grep -n "func startGatedProvider\|func waitUntil" -A30 internal/httpapi/cancel_test.go | head -70; ls ~/go/bin 2>/dev/null; which go
```

<details><summary>Result</summary>

```
package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"campaigncomposer/internal/provider"
	"campaigncomposer/internal/worker"

	"github.com/jackc/pgx/v5/pgxpool"
)

// arrivals records when each request reached the Provider, per Channel.
type arrivals struct {
	mu    sync.Mutex
	times map[string][]time.Time
}

func (a *arrivals) record(channel string, at time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.times[channel] = append(a.times[channel], at)
}

func (a *arrivals) of(channel string) []time.Time {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.times[channel])
}

// startRecordingProvider serves the mock Provider behind a recorder. While
// throttle returns true, POSTs are answered 429 before reaching the Provider.
func startRecordingProvider(t *testing.T, throttle func() bool) (*httptest.Server, *arrivals) {
	t.Helper()
… (155 more lines)
```
</details>

**Write** `~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a785d3de5beeae594/backend/internal/httpapi/crash_test.go` (240 lines)

**Bash** — 

```bash
cd ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a785d3de5beeae594; COMPOSE_PROJECT_NAME=smart-campaign-composer docker compose up -d --wait postgres 2>&1 | tail -1; cd backend && TEST_DATABASE_URL='postgres://campaign:campaign@localhost:55432/campaign?sslmode=disable' go test -count=1 -run 'TestDeliveriesHeldByAKilled' ./internal/httpapi/ 2>&1 | tail -15
```

<details><summary>Result</summary>

```
Container smart-campaign-composer-postgres-1  Healthy
# campaigncomposer/internal/httpapi_test [campaigncomposer/internal/httpapi.test]
internal/httpapi/approve_test.go:56:6: exec already declared through import of package exec ("os/exec")
	internal/httpapi/crash_test.go:9:2: other declaration of exec
FAIL	campaigncomposer/internal/httpapi [build failed]
FAIL
```
</details>

**Bash** — 

```bash
cd ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a785d3de5beeae594/backend; sed -i '' 's|^\t"os/exec"|\toexec "os/exec"|; s|cmd  \*exec.Cmd|cmd  *oexec.Cmd|; s|cmd := exec.Command|cmd := oexec.Command|' internal/httpapi/crash_test.go; TEST_DATABASE_URL='postgres://campaign:campaign@localhost:55432/campaign?sslmode=disable' go test -count=1 -run 'TestDeliveriesHeldByAKilled' ./internal/httpapi/ 2>&1 | tail -15
```

<details><summary>Result</summary>

```
2026/10/28 16:16:41 seeded Recipients and Events
--- FAIL: TestDeliveriesHeldByAKilledWorkerAreCompletedByAnotherAfterTheirLease (62.02s)
    crash_test.go:215: Campaign is sending with map[email:{Pending:10 Sent:504 Failed:0 Total:514} sms:{Pending:10 Sent:377 Failed:0 Total:387}], want completed
FAIL
FAIL	campaigncomposer/internal/httpapi	62.523s
FAIL
[This command modified 1 file you've previously read: backend/internal/httpapi/crash_test.go. Call Read before editing.]
```
</details>

Red for the right reason. Now the reclaim.

**Bash** — 

```bash
cd ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a785d3de5beeae594/backend; python3 - <<'EOF'
p='internal/worker/worker.go'
s=open(p).read()
s=s.replace("""			WHERE d.channel = $1 AND d.state = 'pending' AND d.next_attempt_at <= now()
				AND c.status IN ('approved', 'sending')""","""			WHERE d.channel = $1
				AND ((d.state = 'pending' AND d.next_attempt_at <= now())
					OR (d.state = 'sending' AND d.lease_expires_at < now()))
				AND c.status IN ('approved', 'sending')""")
open(p,'w').write(s)
EOF
cat > internal/db/migrations/0007_lease_reclaim.sql <<'EOF'
-- The worker reclaims Deliveries left in `sending` past their lease, e.g. by
-- a worker that crashed mid-send (ADR-0006).
CREATE INDEX deliveries_leased ON deliveries (channel, lease_expires_at) WHERE state = 'sending';
EOF
grep -rn "0006" internal/db/*.go | head; TEST_DATABASE_URL='postgres://campaign:campaign@localhost:55432/campaign?sslmode=disable' go test -count=1 -run 'TestDeliveriesHeldByAKilled' ./internal/httpapi/ 2>&1 | tail -15
```

<details><summary>Result</summary>

```
2026/10/28 16:17:56 seeded Recipients and Events
--- FAIL: TestDeliveriesHeldByAKilledWorkerAreCompletedByAnotherAfterTheirLease (4.06s)
    crash_test.go:231: reclaimed Delivery 388: 0 Attempts, log []; want Attempt 2 sent
    crash_test.go:231: reclaimed Delivery 389: 0 Attempts, log []; want Attempt 2 sent
    crash_test.go:231: reclaimed Delivery 390: 0 Attempts, log []; want Attempt 2 sent
    crash_test.go:231: reclaimed Delivery 391: 0 Attempts, log []; want Attempt 2 sent
    crash_test.go:231: reclaimed Delivery 392: 0 Attempts, log []; want Attempt 2 sent
    crash_test.go:231: reclaimed Delivery 393: 0 Attempts, log []; want Attempt 2 sent
    crash_test.go:231: reclaimed Delivery 394: 0 Attempts, log []; want Attempt 2 sent
    crash_test.go:231: reclaimed Delivery 395: 0 Attempts, log []; want Attempt 2 sent
    crash_test.go:231: reclaimed Delivery 396: 0 Attempts, log []; want Attempt 2 sent
    crash_test.go:231: reclaimed Delivery 397: 0 Attempts, log []; want Attempt 2 sent
FAIL
FAIL	campaigncomposer/internal/httpapi	4.506s
FAIL
```
</details>

**Bash** — 

```bash
cd ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a785d3de5beeae594/backend; sed -n 25,75p internal/httpapi/deliveries.go
```

<details><summary>Result</summary>

```
AttemptCount int           `json:"attempt_count"`
	Attempts     []attemptJSON `json:"attempts"`
}

// listDeliveries lists a Campaign's Deliveries with their Attempt log, so an
// operator can see why one was retried or failed. `state` and `channel`
// filter; `limit` (default 50, at most 500) bounds the list.
func (s *server) listDeliveries(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Kampanya bulunamadı."})
		return
	}
	q := r.URL.Query()
	limit := 50
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 500 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "limit 1 ile 500 arasında olmalı."})
			return
		}
		limit = n
	}
	var exists bool
	if err := s.pool.QueryRow(r.Context(), `SELECT EXISTS (SELECT 1 FROM campaigns WHERE id = $1)`, id).Scan(&exists); err != nil {
		writeError(w, err)
		return
	}
	if !exists {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Kampanya bulunamadı."})
		return
	}

	rows, err := s.pool.Query(r.Context(), `
		SELECT d.id, d.channel, d.state, d.attempt_count,
			COALESCE((SELECT json_agg(json_build_object(
					'number', a.number, 'status_code', a.status_code, 'latency_ms', a.latency_ms,
					'error', a.error, 'outcome', a.outcome, 'at', a.started_at) ORDER BY a.number, a.id)
				FROM attempts a WHERE a.delivery_id = d.id), '[]')
		FROM deliveries d
… (11 more lines)
```
</details>

**Bash** — 

```bash
cd ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a785d3de5beeae594/backend; python3 - <<'EOF'
p='internal/httpapi/crash_test.go'
s=open(p).read()
s=s.replace("""	for _, d := range campaignDeliveries(t, baseURL, c.ID, "sent") {
		sent[d.ID] = d
	}""","""	for _, ch := range []string{"sms", "email"} {
		for _, d := range campaignDeliveries(t, baseURL, c.ID, "sent&limit=500&channel="+ch) {
			sent[d.ID] = d
		}
	}""")
open(p,'w').write(s)
EOF
TEST_DATABASE_URL='postgres://campaign:campaign@localhost:55432/campaign?sslmode=disable' go test -count=1 -run 'TestDeliveriesHeldByAKilled' ./internal/httpapi/ 2>&1 | tail -15
```

<details><summary>Result</summary>

```
ok  	campaigncomposer/internal/httpapi	4.304s
```
</details>

**Bash** — 

```bash
cd ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a785d3de5beeae594/backend; cat >> internal/httpapi/crash_test.go <<'EOF'

func TestAStalledWorkerWakingUpAfterItsLeaseCannotOverwriteTheReclaimedDelivery(t *testing.T) {
	baseURL, pool, c := reviewedCampaign(t)
	// The stalled worker's requests fail; once it is frozen, the reclaiming
	// worker's requests for the same Deliveries are accepted but answered
	// only after the stalled worker has woken up and recorded its failure.
	var stalled atomic.Bool
	var mu sync.Mutex
	held := map[string]bool{}
	reclaimed := 0
	prov := startScriptedProvider(t, func(channel, key string, request int) provider.Outcome {
		mu.Lock()
		defer mu.Unlock()
		if !stalled.Load() {
			held[channel+"/"+key] = true
			return provider.Outcome{Code: http.StatusServiceUnavailable, Delay: 300 * time.Millisecond}
		}
		if held[channel+"/"+key] {
			reclaimed++
			return provider.Outcome{Delay: time.Second}
		}
		return provider.Outcome{}
	})
	opts := fastRetries(prov.URL)
	opts.MaxAttempts = 1 // the stalled worker's failure is final
	opts.Timeout = time.Second
	opts.Lease = 2 * time.Second
	opts.BatchSize, opts.Concurrency = 10, 10

	if code, a := approve(t, baseURL, c.ID); code != http.StatusOK {
		t.Fatalf("approve: status code %d, body %+v", code, a)
	}
	slow := startWorkerProcess(t, pool, opts)
	waitUntil(t, "both Channels have a batch in flight", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(held) == 20
	})
	slow.signal(t, syscall.SIGSTOP)
	stalled.Store(true)
	time.Sleep(opts.Lease + 200*time.Millisecond)

	reclaim := opts
	reclaim.Timeout = 2 * time.Second
	reclaim.Lease = 4 * time.Second
	startWorkersWith(t, pool, reclaim, 1)
	waitUntil(t, "the held Deliveries are reclaimed", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return reclaimed == 20
	})
	slow.signal(t, syscall.SIGCONT)
	got := waitForStatus(t, baseURL, c.ID, "completed")

	if got.Deliveries["sms"].Sent != c.Audience.SMS || got.Deliveries["email"].Sent != c.Audience.Email {
		t.Errorf("Deliveries = %+v, want all sent: the Provider accepted every one", got.Deliveries)
	}
	assertZeroDuplicates(t, baseURL, pool, c.ID, providerMessages(t, prov.URL))
}
EOF
TEST_DATABASE_URL='postgres://campaign:campaign@localhost:55432/campaign?sslmode=disable' go test -count=1 -run 'TestAStalled' ./internal/httpapi/ 2>&1 | tail -15
```

<details><summary>Error</summary>

```
This agent is isolated in the worktree ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a785d3de5beeae594, but this command is too complex to verify that it stays inside the worktree. Refusing to run it — a worktree-isolated agent's git operations must target its own worktree. Split it into plain, separate commands and run them from ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a785d3de5beeae594.
```
</details>

**Read** `~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a785d3de5beeae594/backend/internal/httpapi/crash_test.go`

<details><summary>Result</summary>

```
235			}
236			at, _ := first.get(h.Channel, strconv.FormatInt(h.ID, 10))
237			if gap := d.Attempts[0].At.Sub(at); gap < opts.Lease-100*time.Millisecond {
238				t.Errorf("Delivery %d was reclaimed %v after the killed worker sent it, want no sooner than its lease of %v", h.ID, gap, opts.Lease)
239			}
240		}
241	}
242
```
</details>

**Edit** `~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a785d3de5beeae594/backend/internal/httpapi/crash_test.go`

**Bash** — 

```bash
cd ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a785d3de5beeae594/backend; TEST_DATABASE_URL='postgres://campaign:campaign@localhost:55432/campaign?sslmode=disable' go test -count=1 -run 'TestAStalled' ./internal/httpapi/ 2>&1 | tail -15
```

<details><summary>Result</summary>

```
2026/10/28 16:18:58 seeded Recipients and Events
--- FAIL: TestAStalledWorkerWakingUpAfterItsLeaseCannotOverwriteTheReclaimedDelivery (4.78s)
    crash_test.go:297: Deliveries = map[email:{Pending:0 Sent:504 Failed:10 Total:514} sms:{Pending:0 Sent:377 Failed:10 Total:387}], want all sent: the Provider accepted every one
FAIL
FAIL	campaigncomposer/internal/httpapi	5.127s
FAIL
```
</details>

**Bash** — 

```bash
cd ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a785d3de5beeae594/backend; python3 - <<'EOF'
p='internal/worker/worker.go'
s=open(p).read()
old_sent="""			_, err = tx.Exec(db, `UPDATE deliveries SET state = 'sent', lease_expires_at = NULL, updated_at = now()
				WHERE id = $1 AND state = 'sending'`, d.id)"""
new_sent="""			_, err = tx.Exec(db, `UPDATE deliveries SET state = 'sent', lease_expires_at = NULL, updated_at = now()
				WHERE id = $1 AND state = 'sending' AND attempt_count = $2`, d.id, d.attempt)"""
old_fail=old_sent.replace("'sent'","'failed'")
new_fail=new_sent.replace("'sent'","'failed'")
assert old_sent in s and old_fail in s
s=s.replace(old_sent,new_sent).replace(old_fail,new_fail)
old_retry="""				next_attempt_at = now() + make_interval(secs => $2), updated_at = now()
				WHERE id = $1 AND state = 'sending'`, d.id, delay.Seconds())"""
assert old_retry in s
s=s.replace(old_retry,"""				next_attempt_at = now() + make_interval(secs => $3), updated_at = now()
				WHERE id = $1 AND state = 'sending' AND attempt_count = $2`, d.id, d.attempt, delay.Seconds())""")
s=s.replace("""	db := context.WithoutCancel(ctx)
	txErr""","""	// The Attempt is logged whatever happens: it was a real request. The
	// Delivery is only updated while this claim still holds it: once its
	// lease expired and another worker reclaimed it, attempt_count has moved
	// on, and that worker's outcome is the one that counts.
	db := context.WithoutCancel(ctx)
	txErr""")
open(p,'w').write(s)
EOF
TEST_DATABASE_URL='postgres://campaign:campaign@localhost:55432/campaign?sslmode=disable' go test -count=1 -run 'TestAStalled|TestDeliveriesHeld' ./internal/httpapi/ 2>&1 | tail -15
```

<details><summary>Result</summary>

```
ok  	campaigncomposer/internal/httpapi	9.329s
```
</details>

**Edit** `~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a785d3de5beeae594/backend/internal/httpapi/crash_test.go`

**Bash** — 

```bash
cd ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a785d3de5beeae594/backend; TEST_DATABASE_URL='postgres://campaign:campaign@localhost:55432/campaign?sslmode=disable' go test -count=1 -run 'TestAWorkerStopped' ./internal/httpapi/ 2>&1 | tail -15
```

<details><summary>Result</summary>

```
2026/10/28 16:19:45 seeded Recipients and Events
--- FAIL: TestAWorkerStoppedMidSendHandsItsDeliveriesBackWithoutWaitingForTheLease (1.85s)
    crash_test.go:289: after a graceful stop 20 Deliveries are still sending, want none
FAIL
FAIL	campaigncomposer/internal/httpapi	2.202s
FAIL
```
</details>

**Bash** — 

```bash
cd ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a785d3de5beeae594/backend; python3 - <<'EOF'
p='internal/worker/worker.go'
s=open(p).read()
old="""			// Shutting down: the claimed Delivery waits out its lease.
			if w.buckets[channel].Wait(ctx) != nil {
				return
			}"""
new="""			if w.buckets[channel].Wait(ctx) != nil {
				// Shutting down before the request: hand the Delivery back.
				w.release(context.WithoutCancel(ctx), d)
				return
			}"""
assert old in s; s=s.replace(old,new)
old="""	res, err := w.post(ctx, channel, d)
	if ctx.Err() != nil && err != nil {
		return // Shutting down: the lease hands the Delivery to the next worker.
	}
	latency := time.Since(started)
	out := classify(res, err, d.attempt, w.opts.MaxAttempts)
"""
new="""	res, err := w.post(ctx, channel, d)
	latency := time.Since(started)
	out := classify(res, err, d.attempt, w.opts.MaxAttempts)
	// Shutting down cut the request off. It may have reached the Provider,
	// so it is logged as an Attempt, and the Delivery goes straight back to
	// `pending` for the next worker instead of waiting out its lease; the
	// Idempotency-Key makes the repeat harmless.
	cutOff := err != nil && ctx.Err() != nil
	if cutOff {
		out = outcomeRetry
		err = fmt.Errorf("cut off by worker shutdown: %w", err)
	}
"""
assert old in s; s=s.replace(old,new)
old="""			delay := w.backoff(d.attempt)
			if res.retryAfter > delay {"""
new="""			delay := w.backoff(d.attempt)
			if cutOff {
				delay = 0
			}
			if res.retryAfter > delay {"""
assert old in s; s=s.replace(old,new)
old="""// outcome is what an Attempt means for its Delivery."""
new="""// release hands a claimed Delivery back to `pending` without an Attempt, so
// its claim does not count against MaxAttempts. Like send's updates it only
// applies while this claim still holds the Delivery.
func (w *Worker) release(ctx context.Context, d delivery) {
	if _, err := w.pool.Exec(ctx, `UPDATE deliveries SET state = 'pending', attempt_count = attempt_count - 1,
			lease_expires_at = NULL, updated_at = now()
		WHERE id = $1 AND state = 'sending' AND attempt_count = $2`, d.id, d.attempt); err != nil {
		log.Printf("worker: release Delivery %d: %v", d.id, err)
	}
}

// outcome is what an Attempt means for its Delivery."""
assert old in s; s=s.replace(old,new)
s=s.replace("""// Run sends until ctx is cancelled. A request cut off by cancellation leaves
// its Delivery in `sending`, to be reclaimed once its lease expires.""","""// Run sends until ctx is cancelled. On the way out, claimed Deliveries are
// handed back to `pending` (see send and release); a worker that dies
// without that chance leaves them `sending` until their lease expires and
// another worker reclaims them.""")
open(p,'w').write(s)
EOF
TEST_DATABASE_URL='postgres://campaign:campaign@localhost:55432/campaign?sslmode=disable' go test -count=1 -run 'TestAWorkerStopped' ./internal/httpapi/ 2>&1 | tail -15
```

<details><summary>Result</summary>

```
2026/10/28 16:20:06 seeded Recipients and Events
--- FAIL: TestAWorkerStoppedMidSendHandsItsDeliveriesBackWithoutWaitingForTheLease (2.49s)
    crash_test.go:307: Delivery 6 Attempts = [{Number:1 StatusCode:0x7baed236e318 LatencyMS:0 Error: Outcome:sent At:2026-10-28 13:20:07.740893 +0000 UTC}], want a cut-off retry then sent
    crash_test.go:307: Delivery 7 Attempts = [{Number:1 StatusCode:0x7baed236e320 LatencyMS:0 Error: Outcome:sent At:2026-10-28 13:20:07.740887 +0000 UTC}], want a cut-off retry then sent
    crash_test.go:307: Delivery 8 Attempts = [{Number:1 StatusCode:0x7baed236e328 LatencyMS:0 Error: Outcome:sent At:2026-10-28 13:20:07.740901 +0000 UTC}], want a cut-off retry then sent
    crash_test.go:307: Delivery 9 Attempts = [{Number:1 StatusCode:0x7baed236e330 LatencyMS:0 Error: Outcome:sent At:2026-10-28 13:20:07.740916 +0000 UTC}], want a cut-off retry then sent
    crash_test.go:307: Delivery 10 Attempts = [{Number:1 StatusCode:0x7baed236e338 LatencyMS:0 Error: Outcome:sent At:2026-10-28 13:20:07.74088 +0000 UTC}], want a cut-off retry then sent
    crash_test.go:307: Delivery 393 Attempts = [{Number:1 StatusCode:0x7baed27040a0 LatencyMS:0 Error: Outcome:sent At:2026-10-28 13:20:07.742492 +0000 UTC}], want a cut-off retry then sent
    crash_test.go:307: Delivery 394 Attempts = [{Number:1 StatusCode:0x7baed27040a8 LatencyMS:0 Error: Outcome:sent At:2026-10-28 13:20:07.742545 +0000 UTC}], want a cut-off retry then sent
    crash_test.go:307: Delivery 395 Attempts = [{Number:1 StatusCode:0x7baed27040b0 LatencyMS:0 Error: Outcome:sent At:2026-10-28 13:20:07.742507 +0000 UTC}], want a cut-off retry then sent
    crash_test.go:307: Delivery 396 Attempts = [{Number:1 StatusCode:0x7baed27040b8 LatencyMS:0 Error: Outcome:sent At:2026-10-28 13:20:07.74251 +0000 UTC}], want a cut-off retry then sent
    crash_test.go:307: Delivery 397 Attempts = [{Number:1 StatusCode:0x7baed27040c0 LatencyMS:0 Error: Outcome:sent At:2026-10-28 13:20:07.742471 +0000 UTC}], want a cut-off retry then sent
FAIL
FAIL	campaigncomposer/internal/httpapi	2.835s
FAIL
```
</details>

**Bash** — 

```bash
cd ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a785d3de5beeae594/backend; python3 - <<'EOF'
p='internal/httpapi/crash_test.go'
s=open(p).read()
s=s.replace("""func (f *firstRequests) count() int {""","""// keys returns the requested Channel/key pairs so far.
func (f *firstRequests) keys() map[string]bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]bool{}
	for k := range f.at {
		out[k] = true
	}
	return out
}

func (f *firstRequests) count() int {""")
s=s.replace("""	stop()
	holding.Store(false)
""","""	stop()
	holding.Store(false)
	inFlight := first.keys()
""")
s=s.replace("""			if _, requested := first.get(ch, strconv.FormatInt(d.ID, 10)); !requested {""","""			if !inFlight[ch+"/"+strconv.FormatInt(d.ID, 10)] {""")
s=s.replace("""			if _, requested := first.get(ch, strconv.FormatInt(d.ID, 10)); requested {""","""			if inFlight[ch+"/"+strconv.FormatInt(d.ID, 10)] {""")
open(p,'w').write(s)
EOF
TEST_DATABASE_URL='postgres://campaign:campaign@localhost:55432/campaign?sslmode=disable' go test -count=1 -run 'TestAWorkerStopped|TestAStalled|TestDeliveriesHeld' ./internal/httpapi/ 2>&1 | tail -15
```

<details><summary>Result</summary>

```
ok  	campaigncomposer/internal/httpapi	12.158s
```
</details>

**Edit** `~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a785d3de5beeae594/backend/internal/httpapi/crash_test.go`

**Bash** — 

```bash
cd ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a785d3de5beeae594/backend; TEST_DATABASE_URL='postgres://campaign:campaign@localhost:55432/campaign?sslmode=disable' go test -count=1 -run 'TestABatchPaced' ./internal/httpapi/ 2>&1 | tail -8
```

<details><summary>Result</summary>

```
crash_test.go:348: Delivery 947 was requested 2 times, want 1
    crash_test.go:348: Delivery 956 was requested 2 times, want 1
    crash_test.go:348: Delivery 959 was requested 2 times, want 1
    crash_test.go:348: Delivery 960 was requested 2 times, want 1
    crash_test.go:348: Delivery 961 was requested 2 times, want 1
FAIL
FAIL	campaigncomposer/internal/httpapi	4.677s
FAIL
```
</details>

**Bash** — 

```bash
cd ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a785d3de5beeae594/backend; sed -n 40,100p internal/worker/bucket.go
```

<details><summary>Result</summary>

```
// clock; nil means time.Now.
func NewBucket(limit float64, now func() time.Time) *Bucket {
	if now == nil {
		now = time.Now
	}
	return &Bucket{limit: limit, now: now, tokens: 1, last: now(), base: limit}
}

// Wait blocks until the caller may make one request, or ctx ends.
func (b *Bucket) Wait(ctx context.Context) error {
	d := b.Reserve()
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Reserve takes a token and returns how long the caller must wait before
// using it.
func (b *Bucket) Reserve() time.Duration {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now()
	rate := b.rateAt(now)
	b.tokens = math.Min(1, b.tokens+now.Sub(b.last).Seconds()*rate)
	b.last = now
	b.tokens--
	if b.tokens >= 0 {
		return 0
	}
	return time.Duration(math.Round(-b.tokens / rate * float64(time.Second)))
}

… (21 more lines)
```
</details>

**Bash** — 

```bash
cd ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a785d3de5beeae594/backend; python3 - <<'EOF'
p='internal/worker/worker.go'
s=open(p).read()
def rep(old,new):
    global s
    assert old in s, old
    s=s.replace(old,new)
rep("""	// Lease is how long a claimed Delivery is held before another worker may
	// reclaim it (reclaiming arrives with #15).
	Lease time.Duration""","""	// Lease is how long a claimed Delivery is held before another worker may
	// reclaim it (ADR-0006). It must be well above Timeout, or requests
	// still in flight would be reclaimed: a Lease under twice the Timeout is
	// raised to that.
	Lease time.Duration""")
rep("""	if o.RateLimit <= 0 {
		o.RateLimit = 100
	}
	return o
}""","""	if o.RateLimit <= 0 {
		o.RateLimit = 100
	}
	if o.Lease < 2*o.Timeout {
		o.Lease = 2 * o.Timeout
	}
	return o
}

// startWindow is how long after its claim a Delivery's request may still
// start. A request takes at most Timeout, so one started within the window
// is answered and recorded before the lease expires, with a quarter of the
// rest of the lease to spare for the database writes. A Delivery whose turn
// comes later (its batch was paced slower than planned) is handed back
// instead of being sent under a lease that is about to run out.
func (o Options) startWindow() time.Duration {
	return (o.Lease - o.Timeout) * 3 / 4
}""")
rep("""func New(pool *pgxpool.Pool, opts Options) *Worker {
	opts = opts.withDefaults()""","""func New(pool *pgxpool.Pool, opts Options) *Worker {
	if opts.Lease > 0 && opts.Lease < 2*opts.withDefaults().Timeout {
		log.Printf("worker: lease %v is under twice the Provider timeout; using %v", opts.Lease, opts.withDefaults().Lease)
	}
	opts = opts.withDefaults()""")
rep("""func (w *Worker) drainBatch(ctx context.Context, channel string) (int, error) {
	batch, err := w.claim(ctx, channel)""","""func (w *Worker) drainBatch(ctx context.Context, channel string) (int, error) {
	startBy := time.Now().Add(w.opts.startWindow())
	batch, err := w.claim(ctx, channel, w.batchSize(channel))""")
rep("""			if w.buckets[channel].Wait(ctx) != nil {
				// Shutting down before the request: hand the Delivery back.
				w.release(context.WithoutCancel(ctx), d)
				return
			}""","""			wait, cancel := context.WithDeadline(ctx, startBy)
			err := w.buckets[channel].Wait(wait)
			cancel()
			if err != nil || time.Now().After(startBy) {
				// Shutting down, or the lease would run out mid-request:
				// hand the Delivery back unsent.
				w.release(context.WithoutCancel(ctx), d)
				return
			}""")
rep("""// claim moves up to BatchSize due""","""// batchSize is how many Deliveries to claim: BatchSize, but no more than the
// Channel's bucket lets through in half the start window at its current
// rate, so a batch slowed by 429s (#14) is still sent within its lease.
func (w *Worker) batchSize(channel string) int {
	fit := int(w.buckets[channel].Rate() * w.opts.startWindow().Seconds() / 2)
	return max(1, min(w.opts.BatchSize, fit))
}

// claim moves up to n due""")
rep("""func (w *Worker) claim(ctx context.Context, channel string) ([]delivery, error) {""","""//
// Deliveries left `sending` past their lease (their worker died or stalled)
// are claimed again like due `pending` ones. The claim raises
// attempt_count, which is what tells the stalled worker, should it wake up,
// that the Delivery is no longer its own (see send).
func (w *Worker) claim(ctx context.Context, channel string, n int) ([]delivery, error) {""")
rep("""		channel, w.opts.BatchSize, w.opts.Lease.Seconds())""","""		channel, n, w.opts.Lease.Seconds())""")
open(p,'w').write(s)
EOF
grep -n "Deliveries of the Channel to" internal/worker/worker.go; go vet ./internal/worker/ && TEST_DATABASE_URL='postgres://campaign:campaign@localhost:55432/campaign?sslmode=disable' go test -count=1 -run 'TestABatchPaced' ./internal/httpapi/ 2>&1 | tail -8
```

<details><summary>Result</summary>

```
223:// claim moves up to n due Deliveries of the Channel to `sending`
ok  	campaigncomposer/internal/httpapi	3.773s
```
</details>

**Edit** `~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a785d3de5beeae594/backend/internal/httpapi/crash_test.go`

**Bash** — 

```bash
cd ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a785d3de5beeae594/backend; sed -i '' 's|^\t"net/http"$|\t"net/http"\n\t"net/http/httptest"|' internal/httpapi/crash_test.go; TEST_DATABASE_URL='postgres://campaign:campaign@localhost:55432/campaign?sslmode=disable' go test -count=1 -run 'TestRestarting' -v ./internal/httpapi/ 2>&1 | grep -v "^2026" | tail -15
```

<details><summary>Result</summary>

```
=== RUN   TestRestartingTheWorkerMidCampaignUnderTheDefaultFailureRatesSendsEveryMessageOnce
PASS
--- PASS: TestRestartingTheWorkerMidCampaignUnderTheDefaultFailureRatesSendsEveryMessageOnce (3.65s)
PASS
ok  	campaigncomposer/internal/httpapi	4.009s
[This command modified 1 file you've previously read: backend/internal/httpapi/crash_test.go. Call Read before editing.]
```
</details>

**Bash** — 

```bash
cd ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a785d3de5beeae594; grep -n "Timeout\|TIMEOUT" backend/internal/config/config_test.go | head; sed -n 28,45p .env.example
```

<details><summary>Result</summary>

```
26:	t.Setenv("WORKER_PROVIDER_TIMEOUT_MS", "1500")
29:	if c.BackoffBase != 250*time.Millisecond || c.ProviderTimeout != 1500*time.Millisecond || c.MaxAttempts != 7 {
30:		t.Errorf("backoff %v, timeout %v, max Attempts %d; want 250ms, 1.5s, 7", c.BackoffBase, c.ProviderTimeout, c.MaxAttempts)
36:	t.Setenv("WORKER_PROVIDER_TIMEOUT_MS", "abc")
39:	if c.BackoffBase != 0 || c.ProviderTimeout != 0 || c.MaxAttempts != 0 {
40:		t.Errorf("backoff %v, timeout %v, max Attempts %d; want zero (worker defaults)", c.BackoffBase, c.ProviderTimeout, c.MaxAttempts)
# Seed for the random outcomes; empty means a new seed on every start.
PROVIDER_SEED=

# Worker retries. 429s (honouring Retry-After), 5xx and timeouts are retried
# with exponential backoff and jitter (base doubled per Attempt, capped at one
# minute) up to WORKER_MAX_ATTEMPTS Attempts, then the Delivery is failed; any
# other 4xx fails it at once. Empty means the default shown.
WORKER_BACKOFF_BASE_MS=500
WORKER_PROVIDER_TIMEOUT_MS=5000
WORKER_MAX_ATTEMPTS=5

# Most Provider requests per second on each Channel (SMS and email are limited
# independently). A burst of 429s halves it for a while; it then recovers.
RATE_LIMIT_PER_SECOND=100

# Host ports published by docker compose (all bound to 127.0.0.1).
WEB_PORT=5173
API_PORT=8080
```
</details>

**Bash** — 

```bash
cd ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a785d3de5beeae594; sed -n 15,45p backend/internal/config/config_test.go
```

<details><summary>Result</summary>

```
}

func TestAnthropicModelOverridesTheDefault(t *testing.T) {
	t.Setenv("ANTHROPIC_MODEL", "claude-opus-5-5")
	if got := config.FromEnv().AnthropicModel; got != "claude-opus-5-5" {
		t.Errorf("model = %q, want claude-opus-5-5", got)
	}
}

func TestWorkerRetryTuningComesFromTheEnvironment(t *testing.T) {
	t.Setenv("WORKER_BACKOFF_BASE_MS", "250")
	t.Setenv("WORKER_PROVIDER_TIMEOUT_MS", "1500")
	t.Setenv("WORKER_MAX_ATTEMPTS", "7")
	c := config.FromEnv()
	if c.BackoffBase != 250*time.Millisecond || c.ProviderTimeout != 1500*time.Millisecond || c.MaxAttempts != 7 {
		t.Errorf("backoff %v, timeout %v, max Attempts %d; want 250ms, 1.5s, 7", c.BackoffBase, c.ProviderTimeout, c.MaxAttempts)
	}
}

func TestUnsetWorkerTuningLeavesTheWorkerDefaults(t *testing.T) {
	t.Setenv("WORKER_BACKOFF_BASE_MS", "")
	t.Setenv("WORKER_PROVIDER_TIMEOUT_MS", "abc")
	t.Setenv("WORKER_MAX_ATTEMPTS", "")
	c := config.FromEnv()
	if c.BackoffBase != 0 || c.ProviderTimeout != 0 || c.MaxAttempts != 0 {
		t.Errorf("backoff %v, timeout %v, max Attempts %d; want zero (worker defaults)", c.BackoffBase, c.ProviderTimeout, c.MaxAttempts)
	}
}

func TestMockLLMOnlyWithoutAKey(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
```
</details>

Config test first (red), then wiring.

**Bash** — 

```bash
cd ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a785d3de5beeae594; python3 - <<'EOF'
p='backend/internal/config/config_test.go'
s=open(p).read()
s=s.replace("""func TestMockLLMOnlyWithoutAKey(""","""func TestTheLeaseComesFromTheEnvironment(t *testing.T) {
	t.Setenv("WORKER_LEASE_MS", "1200")
	if got := config.FromEnv().Lease; got != 1200*time.Millisecond {
		t.Errorf("lease = %v, want 1.2s", got)
	}
	t.Setenv("WORKER_LEASE_MS", "")
	if got := config.FromEnv().Lease; got != 0 {
		t.Errorf("unset lease = %v, want zero (worker default)", got)
	}
}

func TestMockLLMOnlyWithoutAKey(""")
open(p,'w').write(s)
EOF
cd backend && go test ./internal/config/ 2>&1 | tail -3
```

<details><summary>Result</summary>

```
internal/config/config_test.go:50:29: config.FromEnv().Lease undefined (type config.Config has no field or method Lease)
FAIL	campaigncomposer/internal/config [build failed]
FAIL
```
</details>

**Bash** — 

```bash
cd ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a785d3de5beeae594; python3 - <<'EOF'
def rep(p,old,new):
    s=open(p).read(); assert old in s,(p,old); open(p,'w').write(s.replace(old,new))
rep('backend/internal/config/config.go',"""	MaxAttempts     int
}""","""	MaxAttempts     int
	// Lease is how long a claimed Delivery is held before another worker may
	// reclaim it. Zero leaves the worker's default of 30s; the worker raises
	// it to at least twice the Provider timeout.
	Lease time.Duration
}""")
rep('backend/internal/config/config.go',"""		MaxAttempts:     positiveInt("WORKER_MAX_ATTEMPTS"),
""","""		MaxAttempts:     positiveInt("WORKER_MAX_ATTEMPTS"),
		Lease:           time.Duration(positiveInt("WORKER_LEASE_MS")) * time.Millisecond,
""")
rep('backend/cmd/campaign/main.go',"""		MaxAttempts: cfg.MaxAttempts,
""","""		MaxAttempts: cfg.MaxAttempts,
		Lease:       cfg.Lease,
""")
rep('docker-compose.yml',"""      WORKER_MAX_ATTEMPTS: ${WORKER_MAX_ATTEMPTS:-}
""","""      WORKER_MAX_ATTEMPTS: ${WORKER_MAX_ATTEMPTS:-}
      WORKER_LEASE_MS: ${WORKER_LEASE_MS:-}
""")
rep('.env.example',"""WORKER_MAX_ATTEMPTS=5
""","""WORKER_MAX_ATTEMPTS=5

# How long a claimed Delivery is held. A Delivery left `sending` past its lease
# (its worker crashed) is reclaimed by another worker; the Idempotency-Key makes
# the repeated request harmless. Values under twice the Provider timeout are
# raised to that. A stopped worker hands its Deliveries back at once.
WORKER_LEASE_MS=30000
""")
EOF
cd backend && gofmt -l . ; go vet ./... && go test ./internal/config/ 2>&1 | tail -3; grep -n "stop_grace\|worker:" ../docker-compose.yml
```

<details><summary>Result</summary>

```
internal/config/config.go
ok  	campaigncomposer/internal/config	0.233s
40:  worker:
```
</details>

**Bash** — 

```bash
cd ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a785d3de5beeae594/backend; gofmt -w internal/config/config.go; grep -n "signal" cmd/campaign/main.go; git -C ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a785d3de5beeae594 diff --stat
```

<details><summary>Result</summary>

```
12:	"os/signal"
29:	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
 .env.example                           |  6 +++
 backend/cmd/campaign/main.go           |  1 +
 backend/internal/config/config.go      |  7 ++-
 backend/internal/config/config_test.go | 11 ++++
 backend/internal/worker/worker.go      | 99 ++++++++++++++++++++++++++++------
 docker-compose.yml                     |  1 +
 6 files changed, 107 insertions(+), 18 deletions(-)
```
</details>

**Bash** — 

```bash
COMPOSE_PROJECT_NAME=smart-campaign-composer make test 2>&1 | grep -v "^2026" | tail -25
```

<details><summary>Result</summary>

```
RUN  v3.2.4 ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a785d3de5beeae594/web

(node:91220) ExperimentalWarning: localStorage is not available because --localstorage-file was not provided.
(Use `node --trace-warnings ...` to show where the warning was created)
(node:91224) ExperimentalWarning: localStorage is not available because --localstorage-file was not provided.
(Use `node --trace-warnings ...` to show where the warning was created)
(node:91225) ExperimentalWarning: localStorage is not available because --localstorage-file was not provided.
(Use `node --trace-warnings ...` to show where the warning was created)
(node:91221) ExperimentalWarning: localStorage is not available because --localstorage-file was not provided.
(Use `node --trace-warnings ...` to show where the warning was created)
(node:91222) ExperimentalWarning: localStorage is not available because --localstorage-file was not provided.
(Use `node --trace-warnings ...` to show where the warning was created)
 ✓ src/App.test.ts (4 tests) 54ms
 ✓ src/components/CampaignDashboard.test.ts (10 tests) 93ms
 ✓ src/components/CampaignComposer.test.ts (9 tests) 95ms
 ✓ src/components/Regeneration.test.ts (6 tests) 213ms
 ✓ src/components/ApprovalPanel.test.ts (13 tests) 217ms
 ✓ src/components/ContentReview.test.ts (14 tests) 243ms

 Test Files  6 passed (6)
      Tests  56 passed (56)
   Start at  16:24:42
   Duration  1.11s (transform 336ms, setup 0ms, collect 1.13s, tests 916ms, environment 2.04s, prepare 349ms)
```
</details>

**Bash** — 

```bash
COMPOSE_PROJECT_NAME=smart-campaign-composer make test-go 2>&1 | grep -E "^(ok|FAIL|---|panic)" 
```

<details><summary>Result</summary>

```
ok  	campaigncomposer/internal/app	3.488s
ok  	campaigncomposer/internal/config	0.100s
ok  	campaigncomposer/internal/content	0.106s
ok  	campaigncomposer/internal/httpapi	139.426s
ok  	campaigncomposer/internal/llm	2.634s
ok  	campaigncomposer/internal/provider	0.176s
ok  	campaigncomposer/internal/segment	2.289s
ok  	campaigncomposer/internal/worker	0.160s
```
</details>

Now race runs of the sending-related tests, repeated.

**Bash** — 

```bash
cd ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a785d3de5beeae594/backend && TEST_DATABASE_URL='postgres://campaign:campaign@localhost:55432/campaign?sslmode=disable' go test -race -count=3 ./internal/worker/ 2>&1 | tail -2; TEST_DATABASE_URL='postgres://campaign:campaign@localhost:55432/campaign?sslmode=disable' go test -race -count=3 -run 'Crash|Killed|Stalled|Stopped|Restarting|Paced|Timeout|Retr|Fail|Cancel|Concurrent|Sent|Rate|429' ./internal/httpapi/ 2>&1 | grep -E "^(ok|FAIL|---|panic|WARNING|    )" | head -30
```

<details><summary>Result</summary>

```
ok  	campaigncomposer/internal/worker	1.412s
ok  	campaigncomposer/internal/httpapi	243.649s
```
</details>

**Bash** — 

```bash
cd ~/Documents/Projects/smart-campaign-composer/.claude/worktrees/agent-a785d3de5beeae594/backend && TEST_DATABASE_URL='postgres://campaign:campaign@localhost:55432/campaign?sslmode=disable' go test -race -count=5 -run 'TestDeliveriesHeld|TestAStalled|TestAWorkerStopped|TestRestarting|TestABatchPaced' ./internal/httpapi/ 2>&1 | grep -E "^(ok|FAIL|---|panic|WARNING|    )" | head -30
```

<details><summary>Result</summary>

```
ok  	campaigncomposer/internal/httpapi	107.017s
```
</details>

**Bash** — 

```bash
git add -A && git status --short
```

<details><summary>Result</summary>

```
M  .env.example
M  backend/cmd/campaign/main.go
M  backend/internal/config/config.go
M  backend/internal/config/config_test.go
A  backend/internal/db/migrations/0007_lease_reclaim.sql
A  backend/internal/httpapi/crash_test.go
M  backend/internal/worker/worker.go
M  docker-compose.yml
```
</details>

**Bash** — 

```bash
git commit -q -F - <<'EOF'
Crash safety: leases and zero duplicates (#15)

- claim also takes Deliveries left `sending` past their lease (a crashed or
  stalled worker's) and re-leases them; migration 0007 indexes them.
- send's Delivery updates are guarded by attempt_count, so a stalled worker
  waking after a reclaim logs its Attempt but cannot overwrite the outcome.
- A stopped worker hands its Deliveries back at once: a cut-off request is
  logged as a retry Attempt and goes back to pending; a claimed Delivery not
  yet requested is released without costing an Attempt.
- Lease is raised to at least twice the Provider timeout; a request only
  starts within the first part of its lease, and the claim size follows the
  bucket's current rate, so slow batches are not sent twice.
- WORKER_LEASE_MS configures the lease.
- Tests run workers as child processes and SIGKILL, SIGSTOP or SIGTERM them
  mid-send, checking what the Provider received per Idempotency-Key and per
  (Campaign, Recipient, Channel).

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
git fetch -q origin && git merge --no-edit origin/spec-2-core-flow 2>&1 | tail -2; git log --oneline -2
```

<details><summary>Result</summary>

```
Already up to date.
88f7a41 Crash safety: leases and zero duplicates (#15)
79be184 Merge #13 into spec-2-core-flow
```
</details>

Now the notes file.

**Write** `<scratchpad>` (52 lines)

**SubagentHandback**

<details><summary>Result</summary>

```
{"success":true,"message":"Report delivered to your caller."}
```
</details>
