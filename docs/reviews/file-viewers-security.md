# File viewer specification security review

Date: 2026-11-12. Revised: 2026-11-15. Source baseline: `074d0ceff`.
Scope: [requirements](../specs/plugins/requirements/file-viewers.md) and
[design](../specs/plugins/system-design/file-viewers.md), including host file
access, remote executors, native plugin lifecycle, STL parsing, rendering,
marketplace discovery, installation, and privacy.

## Verdict

The original feasibility notes were insufficient for implementation. The specification
adds controls for the risks below. The user accepted 64 MiB/one-million-triangle limits and trusted native plugins.
External files now use Ask/Allow/Deny, with Ask as default. The design remains
conditional on implementation evidence and is not approved for release. Implementation checks are recorded in
[file-viewers-implementation.md](file-viewers-implementation.md). This is not an
independent penetration test. Remote executor, Windows runtime, and physical-device
resource certification remain release gates.

The most important residual risk is explicit: installed native plugins are
trusted same-origin code, and native backend processes inherit host authority.
A file-scoped API cannot make a malicious plugin safe. The design does not
claim that it can. If hostile-plugin isolation is required, this architecture
requires a separate isolation design.

## Assets, actors, and trust boundaries

Protect source files, workspace separation, user credentials, host filesystem,
application availability, browser/GPU capacity, and marketplace integrity.

A repository contributor or agent can create hostile STL bytes and filenames.
An authenticated user can forge selectors or issue concurrent requests. A
compromised registry or maintainer can misrepresent a plugin or publish malicious
code. A remote executor can return corrupt or incomplete content. Other browser
origins can attempt unauthorized requests. Native installed code is trusted
under the existing product model, but compromise remains an explicit threat.

Data crosses these boundaries: filesystem to agentctl; agentctl to authenticated
backend; backend to browser; host to plugin component; component to worker and
GPU; catalog source to Kandev; installed package to privileged runtime. Each
boundary needs validation. A check in the browser cannot replace a server check.

## Findings and required controls

Severity describes the impact if the proposed control is absent. These are
specification findings, not claims of exploitable defects in shipped code.

| ID | Severity / threat | Required control and acceptance link | Verification and residual risk |
| --- | --- | --- | --- |
| SEC-FV-01 | Critical: malicious installed plugin reads secrets or executes code | State native trust explicitly; normal explicit install approval; local pinned official bundle. AC-PLUGINS-FILE-VIEWERS-005.3 | Inspect UI trust copy and actual capabilities. A malicious installed plugin remains able to bypass the viewer facade. This is not mitigated by the new API. |
| SEC-FV-02 | High: forged task/session/repository selects another user's file | Resolve authority server-side from authenticated principal and live execution; validate membership and ownership on admission and revoke on changes. AC-PLUGINS-FILE-VIEWERS-002.4 | Cross-user, cross-workspace, foreign repository, stale session, and execution-replacement tests must return no bytes. Never trust UUID secrecy. |
| SEC-FV-03 | High: traversal, symlink race, or existing absolute-path fallback exposes host files | Rooted open for internal files; resolved external target requires policy admission before bytes; bind approval to exact file identity. AC-PLUGINS-FILE-VIEWERS-002.4 and 006.2-006.5 | Test traversal, internal and external symlinks, ancestor replacement during open, device/FIFO, Windows drive/UNC paths, and executor OS variants. A lexical prefix check is insufficient. |
| SEC-FV-04 | High: file growth or transfer flood exhausts server memory | Stream raw bytes, enforce limit while reading, reserve capacity, bound concurrency and duration, propagate abort. AC-PLUGINS-FILE-VIEWERS-002.2 and 002.3 | Grow a file after stat; disconnect clients; issue concurrent requests across tabs/users; test queue-free busy response and reservation release on every exit. Measure backend peak memory. |
| SEC-FV-05 | High: count overflow or ASCII expansion exhausts browser memory | Preflight counts and record lengths before allocation; bound tokens/facets; account intermediates and outputs; terminate parser on timeout. AC-PLUGINS-FILE-VIEWERS-002.2 | Fuzz binary counts/truncation and ASCII grammar, huge tokens, whitespace, multiple solids, and degenerate facets. Measure worker peak memory. Stock loader use remains blocked until its path fits the budget. |
| SEC-FV-06 | High: poisoned geometry or repeated contexts destabilizes GPU/browser | Validate finite values and derived bounds; cap triangles, estimated GPU allocations, and active preview budget; dispose inactive resources. AC-PLUGINS-FILE-VIEWERS-003.3 and 005.4 | Test NaN/Infinity, extreme finite coordinates causing overflow, empty geometry, repeated open/close, context loss, and low-memory mobile. Driver bugs and unaccounted GPU overhead remain residual risks. A subsequent sandbox check stalled software rendering with 250,000 coincident faces; triangle count does not bound fragment overdraw. See the implementation record. |
| SEC-FV-07 | High: stale response reaches a different file/user after lifecycle change | Bind every result to full preview/plugin generation; abort transport and worker; recheck before publish. AC-PLUGINS-FILE-VIEWERS-001.4 | Switch files, log out, change task, revoke access, update/disable/uninstall plugin during each phase. Late responses must not mount or publish. Already delivered bytes cannot be recalled. |
| SEC-FV-08 | High: XSS or active content through names, errors, or file responses | Render labels as text; do not execute model content; octet-stream, nosniff, attachment, and no-store headers; sanitize diagnostics. AC-PLUGINS-FILE-VIEWERS-005.1 and 005.2 | Use HTML/script payloads in filename, STL solid names, catalog names, and parser exceptions. Assert no DOM execution or navigable inline model document. |
| SEC-FV-09 | High: CSRF, cross-origin read, SSRF, or arbitrary model resource fetch | Authenticated POST and existing Origin/CSRF checks; no permissive CORS, URL input, redirect following, or external references. AC-PLUGINS-FILE-VIEWERS-002.4 and 005.1 | Test unauthenticated and hostile-Origin requests and URL-shaped selectors. Capture network activity while opening malicious fixtures. Same-origin trusted plugin code still has ordinary session authority. |
| SEC-FV-10 | High: catalog metadata installs a misleading or incompatible plugin | Catalog format claims are hints; validate released manifest/digest and host floor at install; no auto-install; declarations constrain registration. AC-PLUGINS-FILE-VIEWERS-001.3 and 004.4 | Test listing/manifest mismatch, package replacement, incompatible host, undeclared registration, extension wildcard, and missing metadata. Hash integrity alone does not establish publisher trust. |
| SEC-FV-11 | Medium: extension spoofing hijacks a built-in viewer | Exact normalized suffix match, bounded declarations, built-in priority, explicit multiple-provider choice, and parser validation. AC-PLUGINS-FILE-VIEWERS-001.2 | Uppercase, double extensions, no extension, path-like metadata, and two matching providers. A valid extension is never proof of valid content. |
| SEC-FV-12 | Medium: discovery, telemetry, cache, or error reporting leaks files | Filter catalog locally; only normalized extension in host query; no model/path/hash in registry requests, storage, logs, or reports. AC-PLUGINS-FILE-VIEWERS-004.3 and 005.4 | Inspect network, browser storage, access/application logs, and error capture after success/failure. Host URLs can still reveal extension interest locally. |
| SEC-FV-13 | High: dependency or release compromise introduces privileged code | Pin dependency versions and lockfiles; package assets locally; inspect licenses/advisories and release provenance; preserve package integrity and permission checks. AC-PLUGINS-FILE-VIEWERS-005.1 and 005.3 | Review exact built artifact and dependencies at implementation/release. Dependency pins and package checks are recorded in the implementation record; release provenance remains a release check. |
| SEC-FV-14 | Medium: unit or stale-revision confusion produces misleading dimensions | Model units without physical-unit inference, original-space extents, mutation detection, and no stale result reuse. AC-PLUGINS-FILE-VIEWERS-002.5 and 003.2 | Known-dimension fixtures, camera rotation and modification during read. STL dimensions are not certified manufacturing measurements. |

## External permission findings added after feedback

| ID | Severity / threat | Required control | Verification and residual risk |
| --- | --- | --- | --- |
| SEC-FV-15 | High: approval replay or path substitution reads a different file | Single-use host receipt bound to user, workspace, execution, installation/digest, policy revision, file identity/revision, and preview generation. AC-PLUGINS-FILE-VIEWERS-006.2 and 006.3 | Replay, cross-user receipt, symlink retarget, same-path replacement, timeout, and duplicate-click tests must deliver no unauthorized bytes. Metadata-preserving content changes still require a safe parser. |
| SEC-FV-16 | High: plugin or agent grants itself persistent Allow | Store policy in host-owned state, authenticate human-facing mutations, enforce revisions, expose no plugin/agent mutation API, reset on digest/reinstall. AC-PLUGINS-FILE-VIEWERS-006.1 and 006.6 | Test stale writes, foreign users/workspaces, plugin storage/config writes, restart, upgrade, and uninstall/reinstall. Malicious same-origin code can still impersonate browser interactions; this is not a hard human-presence boundary. |
| SEC-FV-17 | Medium: malicious file links cause prompt flooding or leak sensitive paths | Bound pending handles/prompts, suppress retry after cancel, show prompts only for visible selected previews, redact paths from telemetry. AC-PLUGINS-FILE-VIEWERS-006.2 and 006.7 | Repeated agent-generated links, restored tabs, Cancel, mobile long paths, and approval expiry must preserve navigation and release resources. The authorized human necessarily sees the selected path. |

## Large-file analysis

The current `readFileContent` performs a stat check followed by `io.ReadAll`.
The new path must enforce its limit during reading. Raising the old constant
would also preserve base64/JSON duplication and would not bound mesh expansion.

The accepted 64 MiB source cap and one-million-triangle cap are independent.
A binary STL facet uses 50 source bytes, while non-indexed positions and normals
alone use 72 bytes per triangle before parser arrays, GPU copies, and browser
overhead. ASCII source size does not predict temporary object allocation.
The 256 MiB accounted budget is an admission target, not an OS memory sandbox.

Three.js parsing in a worker improves cancellation and responsiveness. Workers
can still fetch and allocate memory. Use transferable buffers and bounded parsing.
Reject before large allocations where possible. If tests cannot bound the stock
ASCII loader, use a bounded parser or lower the draft limits before acceptance.
A timer cannot reliably prevent out-of-memory termination.

Use one request owner, with no shared waiter semantics. Cancellation must close
the opened file, stream, worker, and resource reservation. The backend and
executor both need bounded admission, including multi-replica deployments.

## Existing-contract reconciliation

The source permits read-only external absolute paths through ADR 0016 behavior.
The new preview path supports them through explicit host policy rather than
silently inheriting that fallback. Resolve paths in the selected execution only.
Ask defaults to a single-file approval before any content read or delivery.
Allow is scoped to the current user, workspace, installation, and package digest.
Deny blocks the host-mediated preview. No policy grants additional OS permissions.

Agent-created files and symlinks can steer users toward sensitive external data.
The host prompt must show the canonical target and execution, with no truncated
identity or plugin-supplied approval text. Metadata inspection is permitted before
approval; content inspection is not. Permission expiry, replacement, and repeated
requests must not create a prompt loop or consume unbounded server handles.

The native plugin trust model is documented in
[Authoring a Plugin](../public/plugins-authoring.md#security-and-capabilities).
Neither a worker nor a same-origin canvas should be described as a hostile-code
sandbox. The design needs no new ambient filesystem or secret grant for STL.
The no-op managed executable still creates installation and supply-chain exposure.

## Review gates

1. Preserve the accepted native trust model and limits. Do not claim that a
   policy setting isolates malicious installed code.
2. Prove Ask/Allow/Deny ownership, revision checks, and one-use approvals. Test
   policy tightening while loading, parsing, and ready; no bytes before approval.
3. Prove race-resistant opens and file identity across local and remote executors.
   Include symlink retargeting while a prompt is open and same-path replacement.
4. Implement the adversarial, allocation, authorization, lifecycle, and mobile
   tests in the matrix. Benchmark the accepted scope before declaring support.
5. Review exact plugin artifacts/dependencies and publish the authoring guidance
   with the API. Re-review any change in trust, transport, or parser architecture.

## Evidence and limitations

The original 2026-11-12 review inspected source, public contracts, and proposed
documents without changing production code or running runtime checks. The
2026-11-15 implementation record adds the completed tests and resource measurements.
Documentation validators test document structure, not security controls.

The analysis uses these primary references:

- [OWASP File Upload Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/File_Upload_Cheat_Sheet.html): apply layered extension/content checks, authorization, and resource limits to untrusted model input. This feature reads files; it does not add uploads.
- [MDN Web Workers](https://developer.mozilla.org/en-US/docs/Web/API/Web_Workers_API/Using_web_workers): background execution, network access, and transfer semantics. Workers are not a network sandbox.
- [Three.js STLLoader](https://threejs.org/docs/pages/STLLoader.html): ASCII/binary geometry decoding. Loader support alone does not prove adversarial-input safety.

## PR review corrections (2026-11-18)

Policy writes now reset a changed package digest under the policy mutex before
checking the supplied revision. A stale allowance cannot transfer to the changed
package without a fresh decision. The regression test covers a write without an
intervening policy read and verifies that a fresh revision can still be saved.
Lifecycle transitions obtain the grant store before taking the status persistence
locks. Revocation does not reacquire the service mutex in the opposite order.
Missing recovery artifacts no longer block unrelated model files. Existing
excluded paths and symlink targets remain blocked, and other resolution errors
still fail closed. Focused race tests cover policy and exclusion behavior.

Cancelled preparations close their selected file before publication. Failed or
cancelled metadata responses release the executor token. A regression test
performs five failed response writes, then proves the bounded pool can still
admit a valid request. Fuzz input excludes home selectors so the fuzzer cannot
open files outside its disposable fixture through home expansion.
