package plugins

import (
	"context"

	"github.com/kandev/kandev/pkg/pluginsdk"
)

// automationReader adapts the late-wired Automation source to the public Host
// SDK. The source owns workspace filtering; the reader only bounds paging so
// every Host list surface has the same opaque-cursor behaviour.
type automationReader struct {
	source automationSource
}

func (r automationReader) List(ctx context.Context, workspaceID string, page pluginsdk.Page) ([]pluginsdk.Automation, *pluginsdk.PageInfo, error) {
	items, err := r.source.ListPluginAutomations(ctx, workspaceID)
	if err != nil {
		return nil, nil, err
	}
	return pageAutomations(items, page)
}

func (r automationReader) Get(ctx context.Context, workspaceID, automationID string) (*pluginsdk.Automation, error) {
	return r.source.GetPluginAutomation(ctx, workspaceID, automationID)
}

type deniedAutomationReader struct{}

func (deniedAutomationReader) List(context.Context, string, pluginsdk.Page) ([]pluginsdk.Automation, *pluginsdk.PageInfo, error) {
	return nil, nil, permissionDenied(apiReadCapability(resourceAutomations))
}

func (deniedAutomationReader) Get(context.Context, string, string) (*pluginsdk.Automation, error) {
	return nil, permissionDenied(apiReadCapability(resourceAutomations))
}

func pageAutomations(items []pluginsdk.Automation, page pluginsdk.Page) ([]pluginsdk.Automation, *pluginsdk.PageInfo, error) {
	out, info := paginate(items, page)
	return out, info, nil
}
