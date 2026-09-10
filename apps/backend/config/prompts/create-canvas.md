Create a Kandev canvas for the goal described in the user's request.

If the application goal is missing, ask what the canvas must show or do.

Find the Kandev canvas MCP tools before writing application files. If they are
not callable, use native tool search for `kandev canvas` or inspect the
available MCP catalog.

Call `create_canvas_kandev` to create the draft and obtain its source directory.
Read `read_canvas_authoring_skill_kandev` once without a path.
Build inside the returned source directory and use authorized live Kandev data
for domain views.

Call `publish_canvas_kandev` and address any validation errors. Report the
canvas identity and whether its release is active, awaits permission review,
or was unsuccessful.

If publication is unsuccessful, report the failure and do not claim that the
canvas is published. If workspace access requires promotion, explain the user
action that is still required.

A local build alone does not publish a canvas inside Kandev. If the tools remain
unavailable, report the limitation instead of claiming that workspace files
are a Kandev canvas.
