package lifecycle

// SupportsCopilotNativeSessionEvents reports whether a launch through this
// executor can forward the vendor native-event opt-in to an authenticated
// agentctl peer. Fresh containers, Kubernetes pods, remote hosts, sprites, and
// plugin runtimes all receive the current opt-in field with their create
// request; a retained peer that ignored it reports its own subscription state
// through the recovery inventory instead.

func (r *DockerExecutor) SupportsCopilotNativeSessionEvents() bool { return true }

func (r *RemoteDockerExecutor) SupportsCopilotNativeSessionEvents() bool { return true }

func (r *KubernetesExecutor) SupportsCopilotNativeSessionEvents() bool { return true }

func (r *SSHExecutor) SupportsCopilotNativeSessionEvents() bool { return true }

func (r *SpritesExecutor) SupportsCopilotNativeSessionEvents() bool { return true }

func (r *PluginRemoteExecutor) SupportsCopilotNativeSessionEvents() bool { return true }
