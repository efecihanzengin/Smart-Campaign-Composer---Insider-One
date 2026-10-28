# Idempotency key and claim lease prevent duplicate sends

The unique Delivery per (Campaign, Recipient, Channel) from ADR-0001 stops duplicates at enqueue time, but not at send time. Two failure modes remain: a request that times out after the Provider already accepted it, and a worker that crashes while holding a claimed Delivery. So every Provider request carries `Idempotency-Key: <delivery id>`, and the Provider returns the original result for a key it has already seen. Retrying after a timeout is therefore always safe. Claimed Deliveries also carry a lease expiry: a Delivery stuck in `sending` past its lease is reclaimed by another worker, and the idempotency key makes that second Attempt harmless if the first one actually landed.

## Considered Options

- **Never retry after a timeout**: no duplicates, but Deliveries that never reached the Provider would be lost.
- **Mark as sent before calling the Provider**: no duplicates, but a crash loses the message silently.

## Consequences

The mock Provider must implement idempotency-key deduplication, as real SMS and email providers do. The "zero duplicates" integration test relies on it.
