# Security rules

Canvas source runs in an opaque origin. Never depend on browser storage,
service workers, origin-wide cookies, or a stable origin identifier.

Keep source and assets inside the assigned canvas source directory. Use
workspace-relative paths only. Do not create symlinks, device files, sockets,
or parent-directory references. Do not attempt to read another canvas,
another task, or the host filesystem.

Escape text through the framework or DOM APIs. Do not build HTML from untrusted
domain values. Never put access tokens, credentials, or private task data in
source, URLs, or diagnostics. Request only the permissions needed by the
application and handle denial without exposing sensitive details.
