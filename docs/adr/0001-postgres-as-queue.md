# Postgres is both the database and the delivery queue

Deliveries are rows in Postgres, and workers claim them with `SELECT … FOR UPDATE SKIP LOCKED`; there is no separate broker (Redis, RabbitMQ, NATS). A unique constraint on (campaign, recipient, channel) makes "never send the same message to the same person twice" a transactional guarantee at enqueue time, which a broker alongside Postgres would split across two stores. The required throughput (100 msg/s per channel) is far below what Postgres handles, so the extra infrastructure would buy nothing but moving parts.

## Considered Options

- **Postgres + Redis streams**: faster queue, but deduplication and queue state live in different stores and can disagree after a crash.
- **Postgres + RabbitMQ/NATS**: a conventional broker, but more infrastructure for `docker compose up`, and deduplication would still have to live in Postgres.
