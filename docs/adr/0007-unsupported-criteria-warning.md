# Claude reports what it could not target instead of guessing

The Segment Rule tool schema has an `unsupported_criteria: string[]` field. When a Brief asks for something the whitelist cannot express (for example "people who bought shoes" when Events carry no product category), Claude lists it there, and the UI shows it as a visible warning above the Segment Rule labels. Without this field, Claude has two bad options: invent a field (rejected by validation, then repair attempts until it gives up) or quietly drop the condition and produce a broader Audience than the Marketer asked for, with nobody noticing. A visible "couldn't apply" lets the Marketer decide whether to send anyway, rephrase or regenerate.

## Consequences

The mock LLM and tests must cover Briefs with unsupported criteria. The warning does not block approval; it is the Marketer's call.
