# One Content per Channel for now

A Campaign has exactly one Content per Channel. Keying Content by (Channel, language, variant) from day one would make the per-language and A/B bonuses additive instead of a migration, but it would push variant and language handling into every layer of the core flow (generation, validation, review UI, Delivery assignment) before anything works end to end. The core flow comes first; if those bonuses are built, Content gets its wider key then.

## Consequences

Adding per-language Content or A/B variants later requires a schema migration and changes to how Deliveries pick their Content. That cost is accepted.
