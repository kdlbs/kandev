# Chat components

Before adding a semantic predicate for chat or tool output, search `types.ts`
and existing shared helpers. Keep reusable domain predicates, including
projected shell-output presence, in `types.ts`; reuse them in rows and groups
instead of copying equivalent logic. Add focused tests for the shared predicate
and its consumers.
