# Invalid Git Ref Diagnostic: Reproduction Evidence

On 2026-10-22, three new sessions failed during required base-branch fetch,
before worktree creation. An existing session started on a prepared workspace.
Three replacements from an independent source reached agent execution.

The original source contained zero-byte loose refs. Git reported one as a bad
ref with a zero OID. The original fetch logs retained exit code 1 and the
`unknown` diagnostic category. They did not retain raw command output.

## Controlled reproduction

A disposable Git 2.54.0 fixture contained a local origin and a separate clone.
Fetching its valid `main` branch succeeded. Adding an unrelated zero-byte ref
caused `git fetch --no-tags origin main` to exit 1:

```text
fatal: bad object refs/heads/unrelated-broken
error: <fixture-origin> did not send all necessary objects
```

`git show-ref` exited 128 and reported a zero OID. Removing only the fixture's
empty ref restored fetch success. No user repository or remote service was
modified by the experiment.

## Finding and limits

An invalid unrelated local ref is a sufficient cause for this fetch failure.
The incident evidence strongly supports this mechanism, but cannot establish
the original stderr bytes or identify the writer that damaged the refs.
An initial missing-blob error in a separate partial-clone copy was a different
setup issue. The generic missing-objects suffix alone cannot distinguish them.

The confirmed product gap is that explicit bad-ref errors map to `unknown` in
`refresh_diagnostics.go`. The [plan](plan.md) and
[work order](task-01-classify-invalid-local-refs.md) extend the existing safe
classification contract. They do not authorize automatic ref repair or change
required-refresh decisions. Full instance evidence remains local.
