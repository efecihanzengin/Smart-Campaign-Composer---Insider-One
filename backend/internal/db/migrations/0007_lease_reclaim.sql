-- The worker reclaims Deliveries left in `sending` past their lease, e.g. by
-- a worker that crashed mid-send (ADR-0006).
CREATE INDEX deliveries_leased ON deliveries (channel, lease_expires_at) WHERE state = 'sending';
