package main

import acp "github.com/coder/acp-go-sdk"

func mockPiThinkingOptions(model string) []acp.SessionConfigOption {
	modelCategory := acp.SessionConfigOptionCategoryModel
	thinkingCategory := acp.SessionConfigOptionCategory("thought_level")
	models := acp.SessionConfigSelectOptionsUngrouped{{Value: "mock-fast", Name: "Off-only model"}, {Value: "mock-smart", Name: "Thinking model"}}
	levels := acp.SessionConfigSelectOptionsUngrouped{{Value: "off", Name: "Off"}}
	if model == "mock-smart" {
		levels = append(levels, acp.SessionConfigSelectOption{Value: "medium", Name: "Medium"})
	}
	return []acp.SessionConfigOption{
		{Select: &acp.SessionConfigOptionSelect{Type: "select", Id: "model", Name: "Model", Category: &modelCategory, CurrentValue: acp.SessionConfigValueId(model), Options: acp.SessionConfigSelectOptions{Ungrouped: &models}}},
		{Select: &acp.SessionConfigOptionSelect{Type: "select", Id: "thought_level", Name: "Thinking level", Category: &thinkingCategory, CurrentValue: "off", Options: acp.SessionConfigSelectOptions{Ungrouped: &levels}}},
	}
}
