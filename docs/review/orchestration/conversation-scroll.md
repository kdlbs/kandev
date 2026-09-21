# Open Orchestrator conversations at the latest messages

Opening or reopening an Orchestrator conversation now scrolls to the bottom.
The same behavior applies when selecting a different conversation or opening
the mobile Chat tab after history loaded while the tab was hidden.

The shared chat scroll hook previously read the browser's initial `scrollTop`
as a deliberate choice to read older messages. It now initializes following
the latest messages for each conversation/container. A resize observer handles
containers becoming visible, and comment-count changes follow arriving history
and new comments. Scrolling up still stops following; explicit comment links
retain their target. The observer and scroll listener are removed on cleanup.

Validation uses generic example requests and an isolated test database:

- The browser regression failed before the fix with the conversation at the top.
- A second failure exposed the initially hidden mobile container.
- The focused browser test passes at desktop and phone widths: initial opening,
  incoming comments, reading older messages, and leaving/reopening the chat.
- 29 focused frontend tests pass, including initial scroll, task changes,
  asynchronously arriving comments, user scroll intent, and comment-link intent.
- Frontend typechecking and lint pass.

The implementation and its tests are committed on the private review branch.
Live cutover is performed only when existing sessions are quiescent, with the
matched cold backup and health/authentication/history checks used for this pilot.

## Live update

Candidate `0.94.0-orchestration.20260921.sha41b9afc0aae6` deployed successfully
on September 21, 2026 at 17:41:50 UTC. The queued deployment found an idle window
and completed the pilot's cold-backup procedure. Health, web readiness,
authentication, retained history and the running binary/service selection all
passed verification. See the [live receipt](conversation-scroll-live-receipt.json)
and the earlier [candidate receipt](conversation-scroll-candidate.json).
