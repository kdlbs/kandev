---
status: draft
system: executors
requirements:
  - REQ-EXECUTORS-NOMAD-002
  - REQ-EXECUTORS-NOMAD-003
  - REQ-EXECUTORS-NOMAD-004
---

# Native Nomad job identity and normalized comparison

## Purpose and mapping

This is the bounded job-policy comparison contract supporting the
[native executor design](nomad-native-executor.md), not another inventory,
scheduler or provider framework. It closes the loss of fields when a full Nomad
response is decoded into the current intentionally small outbound Job model.

| Requirement | Contract |
| --- | --- |
| REQ-EXECUTORS-NOMAD-002 | Pre-dispatch snapshot, full policy validation and index authority |
| REQ-EXECUTORS-NOMAD-003 | Unknown state cannot authorize cleanup; disconnect/restart policy stays owned |
| REQ-EXECUTORS-NOMAD-004 | Exact staged runtime/bootstrap paths and secret-free immutable policy |

## Pre-dispatch state

Extend the existing typed Nomad JSON envelope with non-secret AgentctlPath,
BootstrapPath, NativeJobPolicyVersion and NativeJobSpecSHA256. Bootstrap env
path is exactly BootstrapPath + `.env`; validate that derivation wherever used.
Paths are normalized absolute paths below the exact owned allocation staging
root, bound to original environment/generation/operation and admitted artifact.
Keep the existing exact WorkspacePath distinct from the profile base. Retain the
verified artifact digest when needed to establish staged-byte continuity.

Before RegisterCreateOnly, persist the chosen paths and the SHA-256 of the
expected normalized fixed job under a versioned domain such as
`kandev.nomad-native-job.v1` plus a NUL separator. The digest comes from the local
owned builder and reviewed normalization contract, never an unvalidated GET or
an arbitrary server response trusted on first use. Do not put that digest inside
its own hashed metadata. No secrets, nonce, token, resolved credential values or
raw credential-bearing diagnostics belong in this state or hash input.

Recovery rebuilds the expected job from the original admitted state/profile
snapshot and these stored paths. It must not choose the latest resolver artifact,
restage files, silently derive another bootstrap name or substitute a changed
profile. Missing/invalid path, digest or policy version in pre-release retained
state means unknown: retain the resource/fence and use exact operator cleanup,
not a guessed upgrade or fresh registration. These are JSON fields, not a SQL
schema migration.

## Validate before projecting

Read bounded full job JSON and validate it before converting to the small Job
or JobRecord structures. Reject duplicate keys at every object level, trailing
JSON, invalid types and oversized/deep input. Preserve integer precision with
UseNumber or exact typed integer decoding; durations are integer nanoseconds,
not a float-rounding or duration-string equivalence.

Use a recursive, explicit, versioned response-field schema. Every path must be
one of: an owned behavior field compared exactly; a recognized behavior field
permitted only at its documented disabled/default value; or a specifically
named server observation field checked separately. Unknown fields reject,
including unknown fields whose values happen to be false, zero, null, empty
maps or arrays. Merely adding fields to the outbound struct does not satisfy
this rule. A full Nomad API type is also insufficient if its decoder silently
ignores unknown keys or a projection subsequently drops behavioral fields.

Normalize only declared path-specific equivalences from the supported server
version. Never recursively erase zero/null/empty values or discard every field
that the Nomad API labels server-set. Do not learn an exception automatically
from the job being recovered. A newly encountered server default requires a
focused source/real-response fixture and compatibility decision; otherwise the
operation stays unknown without mutation.

After validation, canonical JSON uses sorted map keys, exact integer tokens and
stable string encoding. Preserve list order unless a path is explicitly a keyed
set; the two DynamicPorts can be sorted by unique Label after checking exact
labels/count and rejecting duplicates. One task/group/constraint and one
datacenter need no broad order-insensitive comparison. Both the rebuilt expected
job and the actual response pass the same normalization. Compare their complete
canonical behavior and digest; comparing only the currently owned tiny decoder
fields would let foreign behavior disappear.

## Owned and disabled behavior

| Level | Exact owned behavior and prohibited additions |
| --- | --- |
| Job | ID/Name, batch Type, admitted Namespace/Region/Datacenter, effective NodePool and Priority, AllAtOnce, exact ownership metadata/input digest and exact NodeID constraint. Stop must be false for attachment. No parent/dispatched job, payload, periodic/parameterized/multiregion policy, affinities/spreads or additional constraints. No inherited top-level restart/reschedule/migrate/update change. |
| Group | Exactly one named group/count=1; exact group restart, reschedule and disconnect policy; one host network and exactly the two owned dynamic port labels with requested zero values. No extra tasks, lifecycle/dependency, services, volumes, scaling, extra constraints/affinities/spreads, shutdown/lifetime or migration policy. |
| Task | Exactly the owned name and raw_exec driver. Config contains only command `/bin/sh` and ordered args containing the exact stored BootstrapPath; no additional driver config. Env exactly equals the owned generated map, including stored agentctl/bootstrap/workspace/worker-fence and identity paths. Exact CPU/memory, KillTimeout, inherited effective restart policy and supported default resource/log/identity fields. No User override, lifecycle hook, services, artifacts, templates, Vault/Consul/secrets, mounts/devices, extra identity grants, actions, schedule, scaling, extra constraints or alternate command. |
| Ports/resources | Job ports remain requested dynamic zero values; actual assigned ports are validated separately in the full allocation and must match retained ports. Reject host-network overrides, extra/reserved ports, nondefault DNS/CNI, host/IP/device alterations and resource shapes outside the reviewed fixed job. Do not confuse allocation fields with job-spec defaults. |

Ownership metadata is an exact map, not a subset test that permits additional
Kandev policy metadata. Identical ownership labels do not authorize a different
specification. Server-injected runtime environment variables in an allocation
are not permission to accept additions to Task.Env in the job specification.

## Legitimate default normalization

The initial server target is Nomad 2.0.7. The checked reference tree identifies
as 2.0.8, so these source-grounded examples must be confirmed by the real 2.0.7
round-trip fixture before that version is declared supported. This is a local
closed compatibility table, not a general Nomad jobspec canonicalizer.

- API Job.Canonicalize supplies Priority=50 and AllAtOnce=false. The node-pool
  hook/state supplies NodePool=`default` when empty. Require that effective
  value, not any nonempty node pool returned by the server.
- Group default EphemeralDisk is SizeMB=300, Sticky=false, Migrate=false. Explicit
  supported defaults can compare equal to their absent request form. Any
  nondefault storage/migration value fails unless the owned builder chooses it.
- Task RestartPolicy inherits the group. Compare the full effective policy,
  including RenderTemplates=false. Current builder explicitly serializes Delay=0;
  do not replace that explicit zero with the batch default 15 seconds. If the
  builder later omits Delay, the policy version/default table must reflect that.
- Task LogConfig defaults to MaxFiles=10, MaxFileSizeMB=10, Disabled=false.
  Task Resources can contain known default-zero fields such as Cores=0 alongside
  the explicitly owned CPU/memory. Only named, typed defaults are accepted.
- structs.Task.Canonicalize creates the built-in default workload Identity.
  Accept only its documented default name/audience and disabled env/file/other
  grants; no extra identities. Do not broadly ignore the Identity object or
  import a Vault/Consul implicit-identity mutation into this native job.
- Disconnect Replace=false and Reconcile=keep_original remain exact. LostAfter
  is the explicit profile-configured window (default 24h, 1m to 168h) and must
  match the retained `disconnect_lost_after_seconds` exactly; zero normalizes
  only for a retained environment whose state has no snapshot (legacy). Crucially,
  StopOnClientAfter is a duration pointer: absent/null is not numeric zero,
  because an explicit zero can change disconnected-client termination behavior.
- Empty/null equivalence for known empty Constraints, maps or arrays is allowed
  only at the specified path. Nonempty group/task constraints, Lifecycle,
  templates, services, mounts and other behavior never disappear in a projection.

For every other returned default field, the implementation must enumerate its
path, type and exact permitted value from supported-version source/fixture.
An unknown server version or unreviewed default fails closed; neither a global
allow-extra flag nor logging a warning makes recovery compatible.

## Create and modify authority

Decode and retain true CreateIndex and definition JobModifyIndex separately.
RegisterCreateOnly returns JobModifyIndex, not a field that should be relabeled
JobCreateIndex. GET ModifyIndex is a volatile Raft observation and cannot stand
in for either creation or definition identity. Initial creation commonly makes
all three equal; that coincidence is not the contract.

After successful registration, preserve its returned definition index, GET and
fully validate the actual job, then checkpoint its true creation index plus
matching definition index. After a lost response, bind those indexes only after
full exact expected-policy/ownership validation of the original immutable job.
Thereafter require the same CreateIndex and JobModifyIndex before attachment or
destructive decisions. A changed definition index conflicts even if someone
updates/reverts the job back to identical policy; changed creation index means
purge/recreate. ModifyIndex may advance due to status changes without changing
definition and is not included in the behavior digest. Never regress an observed
index or accept another job merely because its labels match.

Specific runtime observations can be excluded from the behavior digest after
typed validation: CreateIndex/JobModifyIndex/ModifyIndex under the rules above,
SubmitTime, Status/StatusDescription, Stable and Version bookkeeping. The closed
schema must explicitly name any additional supported observation fields. Stop,
ParentID, Dispatched, Payload and namespace/identity settings are behavioral
even when an API comment calls them server-set; do not strip them blindly.

Validate the running allocation's embedded Job/JobVersion against the same
expected policy and original job-definition authority, in addition to exact
allocation UUID/create index/NodeID and full resource ports. Valid current GET
job policy alone cannot prove what policy an older running allocation received.
Recheck current authority at the existing mutation/attach boundary. A mismatch,
missing embedded evidence or concurrent definition change retains unknown;
never repairs the job, picks another allocation or grants cleanup authority.

## Focused evidence

WO05 tests must include a real supported-version register/GET/full-allocation
round trip, volatile ModifyIndex-only changes, every declared default, missing
paths/digest, changed CreateIndex/JobModifyIndex, and at least these adversarial
mutations retaining identical ownership metadata: raw_exec command/args/path,
extra Config/Env key, task/group constraint, lifecycle hook, restart/template
render setting, disconnect pointer duration/replacement, resource/port, extra
service/template/mount/identity and an unrecognized nested field. Validate wrong
types, duplicate keys, integer precision and ambiguous default forms. A metadata-
only response fixture cannot establish full policy identity.

## Source references

Paths below are relative to the checked Nomad reference repository alongside
Kandev; runtime support still requires the actual binary-version fixture.

- `api/jobs.go:Job.Canonicalize`, `api/tasks.go:TaskGroup.Canonicalize` and
  `Task.Canonicalize`: API defaults, inheritance and optional fields.
- `command/agent/job_endpoint.go:ApiJobToStructJob`, `ApiTgToStructsTG` and
  `ApiTaskToStructsTask`: pointer/scalar conversion and effective policy.
- `nomad/structs/structs.go:Job.Canonicalize`, `TaskGroup.Canonicalize`,
  `Task.Canonicalize`, DefaultEphemeralDisk and DefaultLogConfig; plus
  `nomad/structs/workload_id.go:DefaultWorkloadIdentity`.
- `nomad/job_endpoint_hook_node_pool_ce.go` and
  `nomad/state/state_store.go:upsertJob`: default node pool and distinct
  creation/definition/volatile indexes.
