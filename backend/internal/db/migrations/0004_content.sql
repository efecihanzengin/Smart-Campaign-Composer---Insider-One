-- The Channels a Campaign delivers through. Claude infers them from the Brief
-- when it writes the Content; the Marketer can toggle each one.
ALTER TABLE campaigns
    ADD COLUMN enabled_channels text[] NOT NULL DEFAULT '{sms,email}'
        CHECK (enabled_channels <@ ARRAY['sms', 'email']);

-- Exactly one Content per (Campaign, Channel) (ADR-0008). SMS Content uses only
-- body; email Content uses every column.
CREATE TABLE contents (
    campaign_id bigint      NOT NULL REFERENCES campaigns (id) ON DELETE CASCADE,
    channel     text        NOT NULL CHECK (channel IN ('sms', 'email')),
    subject     text        NOT NULL DEFAULT '',
    preheader   text        NOT NULL DEFAULT '',
    body        text        NOT NULL DEFAULT '',
    cta_text    text        NOT NULL DEFAULT '',
    updated_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (campaign_id, channel)
);
