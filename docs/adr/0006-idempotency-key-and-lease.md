# Idempotency key and claim lease prevent duplicate sends

The unique Delivery per (Campaign, Recipient, Channel) from ADR-0001 stops duplicates at enqueue time, but not at send time. Two failure modes remain: a request that times out after the Provider already accepted it, and a worker that crashes while holding a claimed Delivery. So every Provider request carries `Idempotency-Key: <delivery id>`, and the Provider returns the original result for a key it has already seen. Retrying after a timeout is therefore always safe. Claimed Deliveries also carry a lease expiry: a Delivery stuck in `sending` past its lease is reclaimed by another worker, and the idempotency key makes that second Attempt harmless if the first one actually landed.

## Cancelling

Cancel needs the same care, because a Delivery in flight when its Campaign is cancelled may or may not have reached the Provider. A Delivery that was never handed to the Provider ends in its own state, `cancelled`, rather than staying `pending`: the api cancels the unsent ones when the Campaign is cancelled, and every worker poll cancels those that come back unsent later (released by a stopping worker, or answered with an error after the cancel). A Delivery whose outcome is unknown, because one of its Attempts got no answer or its lease expired while it was `sending`, is still claimed under the cancelled Campaign and retried with the same Idempotency-Key until the Provider answers. If the first request landed, the Provider replays the acceptance and the Delivery ends `sent`, so the sent/failed/cancelled breakdown is true. The cost is that a request which timed out without ever reaching the Provider is sent after the cancel.

## Considered Options

- **Never retry after a timeout**: no duplicates, but Deliveries that never reached the Provider would be lost.
- **Mark as sent before calling the Provider**: no duplicates, but a crash loses the message silently.
- **Cancel every Delivery not yet `sent` or `failed`**: nothing is requested after a cancel, but a Delivery whose request timed out after the Provider accepted it is counted as cancelled although the Recipient got it, so the final breakdown is wrong.

## Consequences

The mock Provider must implement idempotency-key deduplication, as real SMS and email providers do. The "zero duplicates" integration test relies on it.
