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
