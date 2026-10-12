# Security rules

Canvas source runs in a trusted same-origin iframe. It can use the viewing
browser's ordinary user-session authority, including same-origin storage,
cookies, and host DOM access. Treat the source as trusted user-session code.

Capability grants still govern Kandev protocol operations. A session cookie does
not replace the capability URL, release binding, scope checks, or per-operation
permission checks.

Keep source and assets inside the assigned canvas source directory. Use
workspace-relative paths only. Do not create symlinks, device files, sockets,
or parent-directory references. Do not attempt to read another canvas,
another task, or the host filesystem.

Escape text through the framework or DOM APIs. Do not build HTML from untrusted
domain values. Never put access tokens, credentials, or private task data in
source, URLs, or diagnostics. Request only the permissions needed by the
application and handle denial without exposing sensitive details.
