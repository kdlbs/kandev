package agents

func (*OpenCodeACP) RuntimeReleaseSource() RuntimeReleaseSource {
	return RuntimeReleaseSource{NPM: opencodeACPPackage, GuidanceURL: "https://opencode.ai/docs/cli/"}
}
func (*CursorACP) RuntimeReleaseSource() RuntimeReleaseSource {
	return RuntimeReleaseSource{GuidanceURL: "https://docs.cursor.com/en/cli/installation"}
}
func (*KimiACP) RuntimeReleaseSource() RuntimeReleaseSource {
	return RuntimeReleaseSource{GitHubRepository: "MoonshotAI/kimi-cli", GuidanceURL: "https://moonshotai.github.io/kimi-cli/en/guides/getting-started.html"}
}
func (*MiniMaxACP) RuntimeReleaseSource() RuntimeReleaseSource {
	return RuntimeReleaseSource{NPM: "@minimax-ai/code", GuidanceURL: "https://www.npmjs.com/package/@minimax-ai/code"}
}
func (*KiroACP) RuntimeReleaseSource() RuntimeReleaseSource {
	return RuntimeReleaseSource{GuidanceURL: "https://kiro.dev/docs/cli/"}
}
func (*QoderACP) RuntimeReleaseSource() RuntimeReleaseSource {
	return RuntimeReleaseSource{GuidanceURL: "https://qoder.com"}
}
func (*TraeACP) RuntimeReleaseSource() RuntimeReleaseSource {
	return RuntimeReleaseSource{GuidanceURL: "https://trae.ai"}
}
func (*OmpACP) RuntimeReleaseSource() RuntimeReleaseSource {
	return RuntimeReleaseSource{NPM: "@oh-my-pi/pi-coding-agent", GuidanceURL: "https://www.npmjs.com/package/@oh-my-pi/pi-coding-agent"}
}
func (*DevinACP) RuntimeReleaseSource() RuntimeReleaseSource {
	return RuntimeReleaseSource{GuidanceURL: "https://docs.devin.ai/cli"}
}
func (*GrokACP) RuntimeReleaseSource() RuntimeReleaseSource {
	return RuntimeReleaseSource{NPM: "@xai-official/grok", GuidanceURL: "https://www.npmjs.com/package/@xai-official/grok"}
}
func (*HermesACP) RuntimeReleaseSource() RuntimeReleaseSource {
	return RuntimeReleaseSource{GitHubRepository: "NousResearch/hermes-agent", GuidanceURL: "https://hermes-agent.nousresearch.com/docs/getting-started/installation"}
}
func (*GooseACP) RuntimeReleaseSource() RuntimeReleaseSource {
	return RuntimeReleaseSource{GitHubRepository: "aaif-goose/goose", GuidanceURL: "https://block.github.io/goose/"}
}
func (*AntigravityACP) RuntimeReleaseSource() RuntimeReleaseSource {
	return RuntimeReleaseSource{GuidanceURL: "https://antigravity.google/"}
}
