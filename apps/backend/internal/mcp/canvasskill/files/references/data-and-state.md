# Data and instance state

Kandev domain data is authoritative. Read it on demand and derive filters,
sorting, and display summaries in memory. Do not persist a second copy of
domain records in instance state.

Use instance state only for small application values shared between the task's
canvas views, such as a selected tab, a filter, or a compact collapsed-group
map. Give each value a stable key and tolerate a missing key on first load.

Read the current revision before a conditional update. If the host reports a
revision conflict, reload the value and let the user choose whether to apply
the change again. Keep draft input in memory until the user submits it.
