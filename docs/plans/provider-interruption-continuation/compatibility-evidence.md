# Native Cursor continuation compatibility evidence

## Probe

- Date: 2026-10-02
- CLI: `cursor-agent 2026.10.01-e373342`
- Host: macOS arm64
- Opt-in: `KANDEV_TEST_CURSOR_NATIVE_CONTINUATION=1`
- Isolation: temporary HOME and workspace, one disposable file containing a
  synthetic sentinel, and an owned Cursor account credential read into memory
  only. No user conversation or workspace was opened. The test checks that the
  credential is absent from the temporary HOME during cleanup.
- ACP initialization advertised `agentCapabilities.loadSession=true` and did
  not advertise `agentCapabilities.sessionCapabilities.resume`.

## Results

The probe created a native ACP session, saved its provider session ID, and
interrupted a read prompt after Cursor emitted a `kind=read`,
`status=completed` tool update. It then stopped that process and loaded the
same session ID in a replacement process. `session/load` succeeded and replayed
the earlier output seed and both user prompts. A follow-up prompt in the
restored conversation could not recover the synthetic read result and made no
new tool call. The provider said the interrupted read had returned no file
contents.

A separate fresh conversation emitted a synthetic assistant-output marker and
was interrupted after that output arrived. `session/load` accepted the same
provider ID, but did not replay the interrupted assistant marker. A follow-up
prompt in the restored conversation could not recall it.

The test logs only the CLI version, negotiated load support, tool kind/status,
and boolean restore outcomes. It does not log the account credential, raw
provider frames, workspace path, or prompt contents.

The opt-in test is an observational diagnostic and exits successfully when
continuation is unavailable. A passing test run records the evidence; it does
not by itself prove provider support. The positive same-ID continuation
check below must also assert successful read-only completion.

## Follow-up: abrupt disconnect and native session identity

The follow-up `TestCursorNativeAbruptDisconnectRestore` kills only the owned
scratch ACP process immediately after completed read output or assistant output,
without sending `session/cancel`. The replacement process accepts the same
provider session ID through `session/load`. The interrupted read result and
assistant output are still unavailable after restoration. This is a process
disconnect probe, not a reproduction of Cursor's upstream HTTP/2 reset; those
failure paths can have different persistence behavior.

`TestCursorNativeSessionResume` also tries `session/resume` directly against
the installed CLI. It returns JSON-RPC `-32601` (method not found). This does
not mean native conversation recovery is unsupported: Cursor implements that
operation through `session/load`, which Kandev already selects when the
resume capability is absent.

A further prompt in the restored read conversation says to continue the
unfinished request and allows read-only inspection when earlier results are
missing. It produces the original requested sentinel using one completed read
tool, retaining the same native session ID. It uses no new conversation,
original-prompt replay, or transcript injection. Therefore same-ID continuation
from saved history with safe re-reading is demonstrated for this fixture;
preservation of interrupted tool results is not.

Sanitized outcomes:

| Probe | Result |
| --- | --- |
| Advertised load / resume | `true` / `false` |
| Direct `session/resume` | `-32601` |
| Abrupt disconnect, same ID restored | `true` |
| Interrupted read result retained | `false` |
| Interrupted assistant output retained | `false` |
| Same-ID continuation allowing safe re-read | `true`, one completed read |

## Delivery consequence

The user explicitly requires recovery in the current Cursor ACP session ID.
That identity contract is supported through `session/load`. The updated
requirements/design accept same-ID continuation with safe re-reading after the
user requested continuation. Interrupted-result preservation is not promised.
The positive result above satisfies that native read-continuation prerequisite;
automatic continuation is implemented behind a default-off runtime toggle. Completed writes and
uncertain outcomes remain outside the conservative scope.
