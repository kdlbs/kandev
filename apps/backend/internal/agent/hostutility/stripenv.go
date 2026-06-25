package hostutility

import "github.com/kandev/kandev/internal/agent/agents"

// stripEnvFor 返回 agent 在持久会话路径声明的 StripEnv，作为一次性
// probe/inference 子进程的唯一真相源（RuntimeConfig.StripEnv）。
//
// InferenceConfig 不再独立声明 StripEnv；inference 路径从此派生，避免
// 同一份 []string{"ACP_BACKEND"} 在两处声明导致漂移。
func stripEnvFor(ia agents.InferenceAgent) []string {
	if a, ok := ia.(agents.Agent); ok {
		if rt := a.Runtime(); rt != nil {
			return rt.StripEnv
		}
	}
	return nil
}
