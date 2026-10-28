# Claude emits a whitelisted JSON Segment Rule, never SQL

Claude produces the Segment Rule as a JSON AST through a tool schema: logical nodes (`all` / `any` / `not`, max depth 3), profile conditions over whitelisted fields (`city`, `language`, `created_at`) with typed operators, and Event conditions (`{event, within_days, has, min_count?, min_amount?}`). The backend validates every node against these whitelists and compiles the tree to parameterized SQL itself; no text from the LLM ever reaches the database as SQL. This removes injection and "creative" queries (unbounded scans, writes, access to other tables) by construction, and makes the rule renderable as readable labels for the Marketer.

## Considered Options

- **Let Claude write SQL and run it read-only or sandboxed**: more expressive, but validating arbitrary SQL is effectively impossible, and the case explicitly forbids running LLM output as SQL.
- **Free-form JSON filters without a whitelist**: easy to emit, but field and operator names would drift and could not be compiled safely.

## Consequences

Anything the whitelist cannot express cannot be targeted; see ADR-0007 for how that is surfaced instead of being silently dropped. Adding a new targetable field means extending the whitelist, the compiler and the tool schema together.
