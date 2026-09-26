# Kubernetes guide

Use the current [Kubernetes guide](public/k8s.md) for control-plane deployment,
executor setup, Pod templates, workspace storage and cleanup.

To run task Pods from an existing Kandev instance, configure its Kubernetes
executor. Do not bulk-apply the repository's Kubernetes directory: it contains
separate control-plane deployment examples as well as task-worker inputs.
