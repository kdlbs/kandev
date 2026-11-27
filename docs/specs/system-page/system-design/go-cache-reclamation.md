---
status: current
system: system-page
requirements:
  - REQ-SYSTEM-PAGE-GO-CACHE-001
  - REQ-SYSTEM-PAGE-GO-CACHE-004
  - REQ-SYSTEM-PAGE-GO-CACHE-005
created: 2026-10-02
updated: 2026-11-03
owners:
  - cfl
---

# Go cache reclamation design

## Boundary

Use one selected shared `GOCACHE`, direct deletion, and an explicit busy-cleanup policy.
There are no cache generations, consumer records, or process-lifetime reconciliation.
The user accepts disrupted builds when busy cleanup is enabled.
The [decision](../../../decisions/2026-10-02-direct-go-cache-reclamation.md) records this tradeoff.

Preserve the [optional launch fallback](managed-go-cache-launch-fallback.md).
Do not alter execution environments, process supervision, or task retry behavior.

| Requirement | Design section |
| --- | --- |
| `REQ-SYSTEM-PAGE-GO-CACHE-001` | Shared cache and build defaults |
| `REQ-SYSTEM-PAGE-GO-CACHE-004` | Policy; Admission; Direct deletion |
| `REQ-SYSTEM-PAGE-GO-CACHE-005` | Interface and reporting |

## Policy

Add the boolean `go_cache.allow_cleanup_while_busy` to
`GoCacheSettings` in `internal/system/storage/types.go` and matching web types.
Persist it in the existing settings JSON. Missing values resolve to false.
No new database table or runtime feature flag is needed.
Existing permissions protect writes. Existing settings save/reload and validation remain authoritative.

Use the existing schedule interval and `go_cache.max_bytes` threshold. Keep the displayed physical
cache size inclusive of the fuzz corpus, but report `cleanup_eligible_size_bytes` separately and
exclude fuzz-corpus bytes from threshold eligibility.
Do not add a parallel timer or change default schedule enablement.

| Trigger | Busy option off | Busy option on |
| --- | --- | --- |
| Scheduled, Go enabled and above threshold | Existing idle period and activity gate | Go runs despite activity and quiet period |
| Scheduled, schedule or Go disabled | No Go cleanup | No Go cleanup |
| Explicit Go cleanup above threshold | Existing manual activity gate; existing explicit force remains | Runs without another force prompt |
| Full manual run | Existing admission | Go can run; other providers retain their own existing admission |
| Any cleanup below threshold | No-op | No-op |

The saved option is persistent acceptance of build interruption, not a grant to
delete arbitrary paths. A snapshot controls a queued run; changing the setting
applies to subsequent runs. Display the snapshot policy in run details.

## Admission

Current `Operations.RunNow` calls global `preflight` before the runner starts.
Current `Runner.Run` returns early when global activity is busy. Both boundaries
must recognize the eligible Go-only phase; changing the provider alone cannot work.

When the option is enabled, partition the known Go provider from remaining
providers. Run Go under a shared storage-mutation mutex without the task activity
lease. Then attempt normal admission for remaining providers. Preserve completed
Go results if another provider is busy. Record skipped providers and busy reasons
rather than returning a whole-run `skipped_busy` result after deletion.

The shared mutex belongs to the storage composition and is passed to scheduled
and manual runners, even though Operations constructs a new Runner for each job.
It also serializes direct cleanup with adoption and Go quarantine restore/delete.
An existing process-local lock can be extended; do not add per-consumer persistence.
Preserve single-install ownership and existing shutdown/restore/reset cancellation.

Do not implement this by calling `TryAcquireMaintenanceForce` for a scheduled full
run. That grants the wrong scope and new task admission still cancels its lease.
The opted-in Go phase must not be cancelled by `activity.Coordinator.AcquireTask`.
It remains cancellable on service shutdown, operation cancellation, or its deadline.
Keep the current coordinator unchanged for normal task work and other providers.

With the option off, retain the existing idle/preemption behavior. Existing manual
force is a one-run choice; the new setting does not broaden that choice's scope.
A new task is permitted to overlap opted-in deletion. No task is stopped or retried automatically.

## Direct deletion

Replace `gocache.Provider`'s quarantine rotation with deletion of build-cache contents.
Keep the selected root and its ownership marker. Existing executions keep the same
absolute path, and Go can recreate cache entries on subsequent commands.
Do not rename the root, create a replacement generation, or move bytes into trash.

Retain ownership/adoption and exact-root validation. Use the repository's
handle-relative directory safety patterns so path replacement cannot redirect deletion.
Do not follow symlinks, junctions, or nested mounts. Preserve an unexpected entry
and report a partial result when containment cannot be established.
Preserve the Kandev marker and root `fuzz` subtree. The physical measurement includes its bytes,
but cleanup-eligible size and threshold discovery exclude that subtree.
Never erase a parent directory or an unadopted default cache.

Use a bounded traversal with a 30-second operation deadline, batches of at most
1,000 entries, and a maximum of 100,000 examined entries per pass. These are internal
limits, not new settings. Check cancellation between batches. Retain an in-memory
directory cursor and accumulated size across calls for the same cache root and
threshold. Do not delete any build data until accumulated measurement proves the
threshold is exceeded. Once exceeded, delete saved candidates and continue bounded
deletion in later calls. Do not restart the scan repeatedly to catch concurrent
writes. Mark budget-exhausted passes partial. A backend restart or changed cache
root, ownership mode, threshold, or mount identity may discard the cursor and
start a fresh scan.

Compare mount identity as well as filesystem/device identity at traversal and
removal boundaries. Linux uses mount IDs, macOS uses the mounted-on path, and
Windows uses the resolved volume mount path. Unsupported platforms or unavailable
mount identity fail closed. Preserve and report nested mounts, including bind
mounts that share the cache root's device.

Successful deletion can overlap new writes. New entries can remain, and a build
can fail when an expected file disappears. Both are accepted in busy mode.
Missing entries during traversal are benign; permission or identity failures are
partial failures. Avoid an unbounded `os.RemoveAll` or shell `go clean` call.

Revalidate root identity at each mutation boundary. The storage mutex prevents
Kandev settings/adoption/restore races; directory handles protect against external
path replacement. At completion, preserve a usable owned root for subsequent builds.
If the root was externally replaced, stop and do not recreate through the replacement.

Count removed logical bytes only for successful unlink operations. Do not infer
removed bytes from before-minus-after while writers are active. A bounded final
measurement can report remaining bytes or incomplete status. Open files and
sparse files mean these bytes are not an exact filesystem-free-space delta.

### Resumed scan validation

`cleanupSession` retains directory handles and `directories` identities across
bounded calls. `prepareCleanupSession` currently validates only the root when
reusing a session. The root can remain unchanged while a cache shard is recreated.
An open frame then describes the old directory, while path inspection sees its
replacement. `openEntryDirectory` correctly refuses this identity mismatch.

Before resuming discovery or deletion, validate the retained active frame chain
against its saved identities through the anchored root. This check is bounded by
`cleanupMaxDepth`. Check cancellation during validation. Preserve inode identity,
symlink, and path/descriptor mount checks. Modification time alone is not identity.

If an active directory disappeared or was recreated as a regular directory on the
same validated mount, close the old session and start fresh discovery once in that
invocation. Reuse the original deadline and pass budget. Do not carry the old
threshold measurement or candidates into the new session. A replacement below the
threshold must survive. Unchanged sessions continue from their existing cursors.

A symlink, changed root, mount mismatch, or unavailable mount identity remains a
safety failure. Do not classify those conditions as ordinary cache churn. A fresh
attempt still uses all existing ownership and containment checks.

Directory replacement can also occur after validation or affect saved candidates
whose frames already closed. Keep removal-time checks authoritative. On such an
identity mismatch, stop stale traversal and retire its session rather than emitting
one error per remaining file. Return a partial result with the stable issue
`cache_parent_changed` and reason `cache_scan_invalidated`. Retain bytes successfully
removed during that pass. The next eligible call starts fresh threshold discovery.
Do not restart repeatedly within one call or schedule an unbounded catch-up loop.

These rules implement `AC-SYSTEM-PAGE-GO-CACHE-004.8`, `.11`, `.12`, and `.13`.
They preserve the existing direct-deletion decision and safety authority.

No quarantine intent means no retention delay and no `active_quarantine` blocker.
Historical quarantine records still use their old controller and retention rules.
Preserve historical result decoding and keep `quarantine_entry: null` in new results.

## Compatibility

The current managed root and explicitly adopted root remain valid selections.
The new busy policy applies to either after existing ownership checks. For an
adopted path, the warning includes builds outside Kandev that share that cache.
The default user cache remains read-only unless the operator explicitly adopted it.
Container and remote executors retain their existing local cache behavior.

Disabling management stops new managed overrides but does not erase data.
Explicit Go cleanup remains available under the existing resource-selection contract.
Existing quarantine contents remain restorable until their recorded deletion conditions apply.
No historical retention deadline is shortened and no other provider's policy changes.

## Shared cache and build defaults

Keep existing shared `GOCACHE` injection and fallback.
[PR #4160](https://github.com/kdlbs/kandev/pull/4160) merged at
`341f8941376e29f21b0873ae94a989cf2a595c58`. Repository-owned build and test
commands pass `-trimpath` directly. Backend Make targets retain caller-supplied
`GOFLAGS`; backend CI sets `GOFLAGS=-trimpath` for direct Go commands. Root Make
targets delegate to the backend recipes, and the reuse probe checks both paths.
The implementation fixes path-dependent fixtures and exercises two source
directories with one cache, matching package inputs, and source invalidation.
Build and test reuse is checked through artifacts and build IDs, not elapsed time.
No host-global `go env -w` setting or flags for unrelated repositories are used.

Go's own normal age-based pruning remains active. This feature supplements it;
it does not enforce a quota or replace the cache format.

## Interface and reporting

Add the switch to the existing Go policy card, below the size threshold.
Default it off. Show its warning visibly next to the control, not only in help.
Saving the switch is sufficient; do not add a typed confirmation or repeated prompt.
The switch can be saved even when automatic scheduling is disabled because it
also applies to explicit cleanup. Explain the remaining schedule requirements.

Update schedule help to state that opted-in Go cleanup is exempt from idle checks.
Explain direct deletion and build retries. Keep the threshold unit convention.
Use the existing expanded Go row, cleanup action, and run history. No new page,
consumer count, generation display, or retained-generation metric is needed.

Extend results additively with removed bytes, remaining measurement status,
partial/error details, and whether busy cleanup was permitted. Keep historical JSON
readable. In mixed runs, show that Go completed and other providers were skipped.
Use bounded reason codes and existing localized rendering. Preserve administrator gating.

Desktop uses the current SettingRow with its switch. Phone uses the existing
stacked card composition and page scroll owner. Wrap warning text, preserve touch
help and at least 44-pixel phone targets, and do not enlarge desktop controls.
The nearest exemplar is the existing Storage policy card and storage-setting-help.
Localize all new copy in seven languages and regenerate Traditional Chinese.

### Maintenance history presentation

Keep persisted run states and historical JSON unchanged. Derive a display outcome
from `StorageMaintenanceRun` in a pure, feature-local history summary helper.
Use structured provider results before raw error strings. Preserve queued,
running, cancelled, and busy states. A terminal run with only Go entry-budget
progress can show **Incomplete**, even when its stored legacy state is `failed`.
Expected progress requires `incomplete_scan` or `cleanup_in_progress`, no issue
codes except `entry_limit_reached`, and no error from another provider.
If a raw provider error exists, it must contain only the recognized budget issue.
An unknown error must prevent progress classification even beside known codes.

Any genuine provider error takes precedence over progress. Show **Partial** when
structured results prove some cleanup succeeded, otherwise show **Failed**.
Nonzero removed or quarantined bytes prove useful work. A successful no-op alone
does not. A stored failed run without explanatory provider data remains Failed.
Unknown or malformed error results remain failures. Missing remaining measurements
remain unknown. Skipped resources are separate context, never evidence of failure
or a reason to reinterpret an unknown error as harmless progress.

`StorageRunHistory` owns status, trigger, timestamp, and the result disclosure.
`StorageGoCacheResult` owns removed/remaining facts and concise Go explanations.
Deduplicate issue codes without claiming exact failure counts: the backend retains
only a bounded sample. Recognize legacy parent-change messages for their localized
explanation, but do not classify outcomes from English text alone. Use a generic
localized explanation for unknown errors. Read and delete failures keep their own
explanations; only explicit parent-change or directory-identity evidence may fold
them into the changed-entry explanation. Retain stored `partial` and raw provider
error evidence so a result without a recognized issue category still shows one
generic explanation, including in the inline overview. Preserve full `run.message`
and JSON inside a separately labelled **Technical details** disclosure, initially
closed.

Use `text-sm` for result facts and explanations. Keep metadata and badges at the
existing metadata size. Override the feature's inherited accordion paragraph
margins with compact, explicit gaps. The shared accordion defaults to `text-xs`
and adds paragraph margins, so setting the result child's text size alone is
insufficient. Keep this override local to storage history. Do not change shared
accordion, card, or global typography for this repair.

Desktop places removed and remaining facts together. Phone uses labelled facts
on separate lines, followed by issue context and the details control. The entry
point is Settings > System > Storage > Host > Maintenance history. The existing
storage history accordion and policy card supply the inline phone precedent.
No drawer is needed for this short, read-only result. The page owns normal vertical
scrolling. The optional diagnostic block contains its own overflow and height.
Phone disclosure controls have at least 44px touch targets. Ordinary desktop
buttons retain the shared 28px size. Content-sized run rows can wrap naturally.
Both viewports share outcome derivation, data, permissions, and disclosure state.

Map labels through i18next at render time. Add copy in English, Portuguese,
Simplified Chinese, Japanese, and Korean. Generate the Traditional Chinese pair
and the pseudo-locale with the repository scripts. No new API or database schema
is required. `AC-SYSTEM-PAGE-GO-CACHE-005.5` through `.9` define the visible results.

## Verification

The completed [implementation package](../../../plans/go-cache-reclamation/plan.md)
defines four sequential work orders. Tests prove opted-in deletion occurs during
active task work, not merely that an API returns success. Also prove the off
setting blocks cleanup, another provider stays protected, and ownership checks
still reject a malicious path. Do not require an active build to succeed in busy mode.

Public operations documentation describes the shipped behavior. Implementation
changes do not alter live settings or storage during development.

The [scan recovery and history repair package](../../../plans/storage-cleanup-recovery/plan.md)
adds resumed-directory replacement regressions and desktop/phone history checks.
