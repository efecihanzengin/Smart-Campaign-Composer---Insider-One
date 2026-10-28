# Smart Campaign Composer

A tool where a Marketer describes a campaign in natural language, Claude proposes the audience and channel content, the Marketer approves, and the system delivers to (fake) Recipients while showing progress live.

## People

**Marketer**:
The human operating the tool: writes the Brief, reviews Claude's proposal, edits and approves the Campaign.
_Avoid_: User, operator, customer

**Recipient**:
A (fake, seeded) end person who may receive messages; has profile data, per-channel consent, and a history of Events.
_Avoid_: User, customer, contact, subscriber

**Event**:
A recorded behaviour of a Recipient at a point in time, such as adding to cart or purchasing.
_Avoid_: Activity, action, log

## Campaign

**Brief**:
The Marketer's free-text description of what the campaign should do and to whom.
_Avoid_: Prompt, request, input

**Campaign**:
The unit of work that ties together one Brief, its Segment Rule, its Content per channel, and its lifecycle from draft to completion.
_Avoid_: Job, blast, send

**Segment Rule**:
The structured, validated definition of who a Campaign targets, expressed over Recipient fields and Events.
_Avoid_: Query, filter, SQL, segment

**Audience**:
The concrete set of Recipients a Segment Rule resolves to for a Channel at a given moment, always limited to Recipients who consented to that Channel. Consent is never part of the Segment Rule.
_Avoid_: Segment, list, targets

**Channel**:
A medium a Campaign delivers through: SMS or email.
_Avoid_: Medium, transport

**Content**:
The message template for one Channel (SMS body; email subject and body), possibly containing personalization placeholders.
_Avoid_: Message, copy, text, template

## Delivery

**Delivery**:
The sending of one Campaign's Content to one Recipient on one Channel, tracked across all its Attempts. Exists at most once per (Campaign, Recipient, Channel).
_Avoid_: Message, send, job

**Attempt**:
One try at handing a Delivery to the Provider; a Delivery may have several before it is sent or fails.
_Avoid_: Retry, try

**Provider**:
The (mock) external service that accepts Deliveries for a Channel.
_Avoid_: Gateway, vendor, sender
