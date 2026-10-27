# Scoped Nomad ACL policy for the Kandev executor. Runner fixture copy; it must stay equal in content to the
# deployment's kandev-executor policy. Anything broader than this is a fixture defect.
namespace "kandev" {
  capabilities = ["submit-job", "read-job"]

  variables {
    path "kandev/quarantine/*" {
      capabilities = ["read", "write"]
    }
  }
}

node_pool "kandev" {
  policy = "read"
}

node {
  policy = "write"
}

operator {
  policy = "read"
}
