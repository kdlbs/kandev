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
