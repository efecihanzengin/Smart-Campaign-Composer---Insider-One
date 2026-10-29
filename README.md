# Smart Campaign Composer

A Marketer writes a Brief in plain Turkish. Claude turns it into a Segment Rule and into SMS and email Content. The Marketer reviews, edits and approves the Campaign. A worker then sends it to a mock Provider through a rate-limited, retrying queue that does not send duplicates, and a live dashboard shows the progress.

The case is in [docs/case.md](docs/case.md). Domain terms (Brief, Segment Rule, Audience, Delivery, Attempt, …) are defined in [GLOSSARY.md](GLOSSARY.md), and decisions are indexed in [DECISIONS.md](DECISIONS.md).

## Setup

You need Docker with Compose v2. No API key is required.

```sh
cp .env.example .env    # optional; every variable has a default
docker compose up --build
```

- UI: http://localhost:5173
- API: http://localhost:8080 (`/healthz`, `/metrics`, `/api/...`)
- Postgres: `localhost:55432` (user, password and database are all `campaign`)

On first start the API runs the migrations and seeds about 5,000 Recipients and 50k Events. The seed is deterministic ([ADR-0009](docs/adr/0009-seed-anchored-to-seed-day.md)). The Brief shown as the placeholder in the UI is the case's example Brief:

> Son 30 günde sepetine ürün ekleyip alışveriş yapmayan İstanbul'daki kullanıcılara %30 indirim hatırlatması gönder. Samimi ama kısa olsun.

With the seed it gives the rule "Şehir: İstanbul · Son 30 günde sepete ekledi · Son 30 günde satın almadı", with an Audience of 387 Recipients for SMS and 514 for email after consent.

If a port is taken, set `WEB_PORT`, `API_PORT` or `POSTGRES_PORT` in `.env`. All ports bind to 127.0.0.1. `.env.example` documents every variable, including the mock Provider's failure rates and the worker's retry, lease and rate-limit settings.

### Mock LLM vs real Claude

- **`ANTHROPIC_API_KEY` empty (default):** a scripted mock LLM answers both calls and the UI shows a "Mock LLM" badge. It parses the city, the "son N gün" period and the cart/purchase intent from the Brief. It streams fixed, valid Content, so its Content does not follow the Brief closely. It also understands a few regeneration instructions: "daha kısa", "sadece Ankara", and "kadın"/"erkek", which come back as an unsupported criterion so you can see the warning.
- **`ANTHROPIC_API_KEY` set:** the Segment and Content calls go to Claude. The model is `claude-sonnet-5-5` unless `ANTHROPIC_MODEL` overrides it. Restart the stack after changing either variable (`docker compose up -d`).

## Testing

```sh
make test
```

This runs the Go tests (unit tests, plus integration tests against the compose Postgres, which it starts) and the Vitest suite for the frontend. No API key is needed, because the Claude client is tested against a local fake of the Messages API. If Go is not installed on the host, the Go tests run in a container. The full run takes a few minutes: the sending, rate-limit and crash tests run real workers against a mock Provider at 100 requests per second. CI (`.github/workflows/ci.yml`) runs `make test` on every push.

Notable suites:

- `backend/internal/segment`: the Segment Rule whitelist, the compiler, and the injection and consent-override tests
- `backend/internal/content`: GSM-7/UCS-2 measurement, transliteration and the limits checked against the longest name and city in the Audience
- `backend/internal/httpapi/*_test.go`: the HTTP flow end to end, including the repair loop, approval guards, retries, rate limiting, cancel, `crash_test.go`, which SIGKILLs and SIGSTOPs worker processes mid-send and then checks for zero duplicates, and `generation_test.go`, which SIGKILLs an api process mid-generation

## Architecture

```
web (Vue 3, nginx) ──/api──▶ api (Go) ──▶ Claude / mock LLM
                               │
                               ▼
                           postgres  ◀── worker (Go) ──▶ provider (mock, Go)
```

- **Five containers.** `postgres`. `api` serves HTTP and SSE, makes the LLM calls, and runs the migrations and seed on startup. `worker` is the same Go binary in worker mode and sends Deliveries. `provider` is the mock SMS/email Provider. `web` is the built Vue app on nginx, which proxies `/api` to the API.
- **Postgres is the only store and also the queue** ([ADR-0001](docs/adr/0001-postgres-as-queue.md)). Approval freezes the Audience into one `deliveries` row per (Campaign, Recipient, Channel), with a unique constraint on that triple. The row holds the rendered payload and re-checks consent inside the approval transaction. Workers claim due rows with `FOR UPDATE SKIP LOCKED`.
- **Two LLM calls, each with a validate-and-repair loop** ([ADR-0005](docs/adr/0005-two-llm-calls-with-repair-loop.md)). The first is a structured call for the Segment Rule. That rule is whitelisted JSON, never SQL. The backend validates it and compiles it to parameterized SQL ([ADR-0003](docs/adr/0003-segment-rule-is-whitelisted-json-not-sql.md)), and consent is applied outside the rule ([ADR-0004](docs/adr/0004-consent-outside-segment-rule.md)). The second is a streamed call for the Content, which is forwarded to the browser over SSE as it arrives. Both calls stream over SSE, so the UI shows a repair while it runs. If the output is malformed or breaks a rule, the errors are sent back to Claude, up to 2 times, before the Marketer sees an error. Approval is refused while a generation call runs: each call registers itself and renews the registration every few seconds, so a long call keeps blocking and one left by a crashed api expires within 15 seconds. SMS is held to one GSM-7 segment, with the longest real first name and city of the Audience substituted ([ADR-0002](docs/adr/0002-sms-gsm7-by-default.md)).
- **How duplicates are prevented** ([ADR-0006](docs/adr/0006-idempotency-key-and-lease.md)):
  - The unique constraint means each Recipient has exactly one Delivery per Campaign and Channel.
  - Every request carries `Idempotency-Key: <delivery id>`, so a retry after a timeout replays the Provider's original acceptance instead of sending again.
  - A claim holds a lease. A crashed worker's Deliveries are reclaimed once the lease expires.
  - Each result is written only if the `attempt_count` still matches the claim, so a stalled worker cannot overwrite a newer outcome.
- **Sending.** The limit is 100 requests per second per Channel, enforced by a token bucket that halves its rate when the Provider answers with bursts of 429s. 429s (honouring Retry-After), 5xx and timeouts are retried with exponential backoff and jitter, up to 5 Attempts. Other 4xx fail the Delivery at once. Every Attempt is logged with its status code, latency and error in the `attempts` table.
- **Cancelling.** Cancel stops new claims and moves every Delivery not yet handed to the Provider to `cancelled` ([ADR-0006](docs/adr/0006-idempotency-key-and-lease.md)). Deliveries in flight finish. One the Provider rejected is cancelled. One whose outcome is unknown (its request timed out, or its worker died) is retried with the same Idempotency-Key until the Provider says whether it was accepted, so the final counts are true.
- **Monitoring.** The dashboard is an SSE stream of pending, sent, failed and cancelled counts per Channel. `/metrics` serves Prometheus counters for sent, failed and retries per Channel, read from the database, so they cover every worker. `/healthz` pings the database.

## Not done and known limitations

- **The real model is checked by hand, not by the tests.** On 29 October 2026 the real Claude client was run manually with the example Brief and with a Brief containing an unsupported criterion (product category). Both gave a correct Segment Rule and Content. That Campaign was approved and sent, with the worker stopped and restarted mid-send: 1220 Deliveries, exactly 1220 accepted Attempts, zero duplicates. Regeneration and repair prompts were not part of that run. `make test` still uses a fake of the Messages API, so it needs no key. Sonnet thinks adaptively, so expect a pause before the first streamed token.
- **The mock LLM's Content is scripted.** It is valid and in Turkish, but generic. For example, a Brief that only names a city still gets cart-reminder Content.
- **The rate limit is per worker process.** Running several workers multiplies the 100/s. A distributed limiter was out of scope.
- **The Provider can accept a Delivery that is marked `failed`.** If the last allowed Attempt times out after the Provider accepted it, the Delivery is marked failed. Nothing is duplicated, but the dashboard under-reports sent messages.
- **A SIGKILLed worker's in-flight Attempt is never logged**, so `attempt_count` can exceed the number of Attempt rows. A graceful stop does log it.
- **A cancel can still send a Delivery that was in flight.** If its request timed out without reaching the Provider, the retry that finds out its outcome sends it. The alternative was to cancel it while it might already have been sent, which makes the final counts wrong.
- Lease expiry uses the database clock and the start window uses the worker's clock. Clock skew between the two hosts eats into the safety margin.
- **Not built:** authentication (there is a single implicit Marketer), per-language and A/B Content ([ADR-0008](docs/adr/0008-one-content-per-channel.md)), Brief criteria the Segment Rule cannot express (they are reported as warnings, [ADR-0007](docs/adr/0007-unsupported-criteria-warning.md)), real SMS or email Providers, and the case's bonus items (scheduled sending and so on).

## Repository layout

- `backend/`: the Go module. `cmd/campaign` is the API and worker, `cmd/provider` is the mock Provider, and the packages are under `internal/`.
- `web/`: the Vue 3, Vite, TypeScript, Pinia and Tailwind frontend.
- `docs/adr/`: the ADRs, indexed in `DECISIONS.md`.
- `ai-sessions/`: the Claude Code session exports, scrubbed of keys and personal data.
