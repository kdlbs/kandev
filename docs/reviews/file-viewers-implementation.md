# File viewer implementation record

Date: 2026-11-15. Implementation was explicitly authorized after the design review.
PR creation was authorized on 2026-11-17. Release and marketplace publication
remain outside this change.

## Delivered code

The host now validates manifest-owned file viewer declarations, exposes
`registerFileViewer`, and routes supported files through a metadata-only opening
path. File-tree, chat/editor, desktop, phone, and restoration paths share the
selection rules. Existing source editors retain their default and their unsaved
text when a user selects a viewer. Explicit provider choices use file-tab storage.
Unknown formats offer an extension-filtered marketplace link. Format metadata in
the official index comes from the verified archive manifest.

Preview admission resolves the authenticated user, workspace, task session,
repository, installation revision, and current executor. Agentctl opens one regular
file and retains its identity. External paths, including symlink targets and the
executor user's home, follow a host-owned Ask/Allow/Deny policy. Browser-session
approval is one-use. API tokens cannot change policy or approve an Ask request.
The existing trusted native-plugin model remains unchanged.

The standalone `kandev-plugin-stl` worktree sits beside the Kandev worktree. It was
created from the official template and attached to this task. Its package contains
a minimal managed executable, local Three.js assets, a bounded parser worker, and
host-React controls. It supports rotation, zoom, pan, Fit, original XYZ extents,
labelled grid spacing, and explicit unit assumptions. It has no model URL loader,
webhook, analytics, or declared Host capability.

## Completed checks

- Targeted Go race tests pass in the process, agentctl HTTP, agentctl client,
  plugin admission/policy, and backend execution-binding packages.
- Changed Go packages pass the repository's pinned golangci-lint v2.9.0.
- Frontend unit tests cover manifest-backed registration, lifecycle rollback,
  bounded byte framing, consent and cancellation, workspace policy races,
  provider-choice persistence, built-in priority, and existing API/storage behavior.
- Regression tests prove that the file tree skips the old text reader for STL and
  that startup preserves viewer declarations after a page reload.
- Web type checking, localization validation, and production builds pass at the
  recorded implementation checkpoints. Browser results are below. Full web lint and SDK type/contract checks also pass.
- The parser tests cover ASCII and binary dimensions, misleading `solid` binary
  headers, malformed input, count/byte limits, non-finite coordinates, bounded
  tokens, inert names, multiple solids, and zero-extent geometry.
- Host-platform packaging validates the manifest, exact package file inventory,
  and checksums. Go plugin tests and JavaScript syntax checks pass.
- A 30-second file-path fuzz run completed 148,599 executions before the final
  error-message changes; those changes do not alter path resolution.

## Resource measurements

Each parser measurement ran in a separate Node process. Source generation was
followed by garbage collection before parsing. RSS includes the process and input
generation; it is not an isolated parser allocation measurement.

| Case | Source bytes | Triangles | Parse time (ms) | Accounted bytes | Peak process RSS (KiB) |
| --- | ---: | ---: | ---: | ---: | ---: |
| Binary ceiling | 50,000,084 | 1,000,000 | 194 | 246,777,300 | 189,020 |
| ASCII near byte ceiling | 67,108,837 | 780,335 | 3,668 | 224,346,353 | 238,260 |

Both pass the ten-second parser deadline and 256 MiB accounting calculation on
this machine. The accounting reserves source bytes, five geometry arrays, and
16 MiB for framebuffer attachments. The renderer caps the canvas at two million
pixels without multisampling. Browser, worker-runtime, and driver overhead remain
outside the accounting. These are not physical-phone or GPU-memory certifications.

## Browser results

The installed-package smoke test passes in Chromium and mobile Chromium. It uses
the real host and native package installation. It verifies external-file consent,
known dimensions, unchanged model-unit dimensions after camera movement,
pointer/keyboard and touch input, 44px phone controls, and graphics-context loss.
Desktop and phone screenshots were inspected. Both show the viewport, dimensions,
labelled scale and reachable controls without horizontal overflow.
The package initially failed dimension interpolation; the plugin now passes values
through the SDK's scoped translation options.

The host fixture tests pass for an external 11 MiB file, Cancel, no read before
consent, Ask/Allow/Deny, and revocation of an active preview. Exact extension
filtering, the no-match state, and return to the same file pass on desktop and phone.
Downloads pass byte-for-byte verification in both browser projects after the
placeholder fix. Each host browser project passes two tests. The installed STL
package adds one smoke test per project.

Discovery uses the normal client router so Settings can return to the original
file. A regression test first demonstrated that a plain anchor reloaded the page.
Discovery flushes the desktop layout before departure. Phone returns keep one
session-bound metadata selection in memory for up to five minutes. Neither path
retains file bytes or approval. Returning to an external preview requires a new
request. Download explicitly fetches real bytes through the existing 10 MiB reader;
it never saves the empty metadata placeholder.
Unavailable installed viewers expose their status and a link to plugin settings.
Text-fallback tests cover a file change while a read is pending and after failure.
The full inventory of planned lifecycle and executor cases is not certified by
these focused tests.

## Security evidence and residual risks

The [security matrix](file-viewers-security.md) remains the threat inventory.
The implementation adds no sandbox. Native same-origin plugins and their native
processes remain trusted; the preview interface cannot prevent malicious installed
code from using other authority. Delivered bytes cannot be recalled or securely
erased. File revision checks detect ordinary edits and path replacement, but do
not detect a hostile writer that restores all observed metadata.

Selections expire after five minutes. Transfer deadlines are thirty seconds.
Agentctl caps pending and active handles together at four. Backend admission caps
pending requests at four per user and sixty-four total, with one active transfer
per user and four total. A page reserves one active preview until cleanup. The
browser requires the admitted length plus a private completion byte before it
mounts the plugin. No incomplete response becomes parser input.

Policy writes revoke pending and active backend selections. Other browser pages
notice policy revision changes on a two-second poll; failed policy checks also
cancel the preview. Session authorization and exact execution/client identity are
checked during admission and transfer. These controls protect supported host
operations; they are not revocation guarantees against trusted code retaining bytes.

## Remaining release gates

- Certify Docker, SSH, and Kubernetes transports with the adversarial file and
  cancellation fixtures. Docker is not installed in this environment. No remote
  executor is certified by the local HTTP or fake-client tests.
- Run filesystem identity and cancellation cases on supported Windows runners.
  Cross-compilation is not runtime evidence.
- Measure the accepted ceilings on representative physical phones and GPUs,
  including repeated open/close and low-memory behavior.
- Release the host contract and matching SDK, then replace the plugin manifest's
  development minimum with the actual supported release. The current `0.100.0`
  placeholder is not a compatibility claim for released 0.100.0 hosts.
- Publish the dedicated repository/package and submit its verified marketplace
  entry only when requested. Add the curated 3D category at that point.

A broader unfiltered process/API test run reported failures in existing Git
capture/numstat and LSP bridge tests in this environment. The feature-focused race
suites pass. This record does not claim that the full backend suite is green.

## Manual sandbox check

The user requested a disposable instance bound directly to the Tailscale IP,
without Tailscale Serve. The instance uses a fresh database, isolated home and
repositories, mock agents/providers, and the locally verified plugin archive.
The main instance on port 9998 was not changed.

A synthetic 250,000-triangle model with coincident repeated box faces stalled
headless Chromium's software renderer. The canvas visibility check timed out,
and a later snapshot also timed out. Closing the owned browser released it.
The triangle and allocation limits do not bound fragment overdraw or render time.
This case remains part of the physical-device/GPU release gate. The manual
large-file fixture now uses a distributed 250,000-triangle surface instead.

Browser checks through the direct Tailscale HTTP origin passed for ASCII and
binary boxes (20 × 30 × 40 model units), the distributed large model, external
file consent, invalid STL rejection, the 64 MiB limit, and the one-million-triangle
limit. Marketplace discovery applied the `3mf` extension filter, and browser
Back returned to the original file. The `.3mf` discovery fixture contains UTF-8
placeholder bytes, so its fallback is the text editor, not a binary preview.
The shutdown helper verified the sandbox PID, process start time, isolated data
directory, and port without stopping the test instance.

## Automatic preview revision (2026-11-16)

Visible selected files now start admission automatically. Workspace files load
without a confirmation. External files retain Ask/Allow/Deny and the approval
prompt's Cancel action. Loading and ready previews have no manual Cancel control;
closing, switching, hiding, or invalidating a preview still releases resources.
Failures and dismissed approvals require explicit Retry preview.

The hook and byte-client suites passed 11 tests. Focused Oxlint, web typecheck,
i18n checks, public-doc validation, specification catalog/lint, and the E2E web
build passed. Desktop and phone tests passed for both the standalone STL package
and the host fixture (four tests). These runs covered automatic internal loading,
external consent, policy changes, and absent preview Cancel controls. The existing
Tailscale sandbox also passed the automatic-load and external-approval checks.

## Shared rich-preview integration (2026-11-17)

Rich-output FilePreview now mounts the same PluginFilePreview used by opened
files. No second registration or STL-specific rich renderer was added. The
host retains built-in/source defaults, pins an active plugin route on unload,
passes the message session/repository/path, and skips the legacy text read for
plugin routes. Collapse unmounts the preview; shared visibility and lifecycle
cleanup remain in force. The one-live-preview browser limit remains unchanged.

The template has a separate local worktree at `../kandev-plugin-template` with
an opt-in file-viewer recipe, manifest and rich-payload examples, lifecycle
helper/tests, and a test target wired into `make test-ui`. It explicitly requires
the development contract until a supporting host and SDK release exists.

Validation passed: 77 focused rich-output/hook tests, focused Oxlint, web
typecheck, E2E web build, standalone STL package desktop and phone tests (both
now render and interact in chat before checking file panels), three template
lifecycle tests, public-doc validation, spec lint, and whitespace checks. The
previous sandbox on :48429 was already stopped; this turn did not restart it
or change the main instance. The host and template changes remain unpublished.
