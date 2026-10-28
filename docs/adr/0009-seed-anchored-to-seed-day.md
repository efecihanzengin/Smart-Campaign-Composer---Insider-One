# The seed is deterministic relative to the day it runs

The seed draws every Recipient and Event from a fixed random seed, but stores timestamps as offsets back from an anchor: the start of the UTC day the seed runs on. Two fresh runs on the same day produce identical rows; runs on different days produce the same Recipients and the same Events shifted in time. Anchoring to the seed day keeps "last 30 days" Audiences meaningful whenever a reviewer first runs `docker compose up`, which a fixed calendar date would not: after a few weeks, every Event would be older than 30 days and the example Brief would match no one.

## Considered Options

- **Fixed calendar anchor**: byte-identical across all days, but the example Brief's Audience decays to zero as the anchor ages.
- **Re-seed on every startup**: always fresh, but violates "restarting the stack does not re-seed", and changes data under running Campaigns.

## Consequences

The example Brief's Audience count is stable on the seed day and drifts slowly afterwards on a long-lived volume, because Audience queries measure "last N days" from `now()`. `docker compose down -v` re-seeds against today.
