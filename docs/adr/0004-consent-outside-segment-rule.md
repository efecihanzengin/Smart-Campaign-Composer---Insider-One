# Consent is enforced per Channel outside the Segment Rule

Channel consent (`sms_consent`, `email_consent`) is not a field the Segment Rule can reference. It is applied unconditionally when a Segment Rule resolves to an Audience for a Channel, and again when Deliveries are frozen at approval. Putting consent inside the rule would make compliance depend on Claude remembering a condition, or on the Marketer not removing it; keeping it outside makes "no message on a Channel without consent" impossible to bypass from the prompt or the UI.

## Consequences

The Audience count is always per Channel and can differ between SMS and email for the same Segment Rule. A Channel whose Audience is empty after consent is disabled for that Campaign.
