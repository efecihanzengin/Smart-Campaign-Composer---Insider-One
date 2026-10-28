# Two separate LLM calls, each behind a validate-and-repair loop

Generating a proposal makes two Claude calls: a non-streamed tool call that produces the Segment Rule, and a streamed tool call that produces Content per Channel. Each output is validated in the backend (the Segment Rule against its whitelists; Content against encoding-aware length limits, allowed placeholders and required fields). On failure, the validation errors are sent back to Claude and the call is retried, at most 2 repairs, before the error is surfaced to the Marketer. Splitting the calls lets Content stream to the screen while the Segment Rule stays strictly structured, lets either part be regenerated on its own, and keeps each schema small enough for Claude to follow reliably. Both calls sit behind an `LLM` interface with a scripted mock, so the repair loop is testable without an API key.

## Considered Options

- **One call returning Segment Rule and Content together**: fewer round trips, but the whole proposal must be regenerated to fix either part, and the Segment Rule would have to stream as partial JSON.
- **Validate and fail without repair**: simpler, but turns recoverable model mistakes (an SMS 12 septets too long) into user-facing errors.
