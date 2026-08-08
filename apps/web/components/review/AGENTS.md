# Review components

Review file identity is repository-scoped, not path-scoped. For aggregate
multi-repository results, preserve `repository_name` and use `reviewFileKey`
(repository plus relative path) or an equivalent composite identity. Files
with the same relative path in different repositories must remain distinct,
including when entries from the same repository are interleaved.

Preserve explicit `base_ref` and `is_submodule` metadata. Do not infer
submodule state from repository or path names; nested submodule changes must
remain anchored to the parent repository's gitlink. Cover root and nested
child repositories, same-scope interleaving, and identical paths across
scopes in focused tests.
