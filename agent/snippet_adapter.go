// SnippetStoreAdapter 桥接 agent.SnippetStore → tools.SnippetProvider，
// 解决 agent ↔ agent/tools 的循环依赖。
package agent

import (
	"github.com/linkxzhou/SimpleClaw/agent/tools"
)

// SnippetStoreAdapter 将 SnippetStore 适配为 tools.SnippetProvider 接口。
type SnippetStoreAdapter struct {
	Store *SnippetStore
}

// NewSnippetStoreAdapter 创建适配器。
func NewSnippetStoreAdapter(store *SnippetStore) *SnippetStoreAdapter {
	return &SnippetStoreAdapter{Store: store}
}

func (a *SnippetStoreAdapter) SaveSnippet(id, name, description, code, funcName string, tags []string) error {
	return a.Store.Save(&Snippet{
		ID: id, Name: name, Description: description,
		Code: code, FuncName: funcName, Tags: tags,
	})
}

func (a *SnippetStoreAdapter) GetSnippet(id string) (*tools.Snippet, bool) {
	sn, ok := a.Store.Get(id)
	if !ok {
		return nil, false
	}
	return toToolsSnippet(sn), true
}

func (a *SnippetStoreAdapter) GetSnippetByName(name string) (*tools.Snippet, bool) {
	sn, ok := a.Store.GetByName(name)
	if !ok {
		return nil, false
	}
	return toToolsSnippet(sn), true
}

func (a *SnippetStoreAdapter) DeleteSnippet(id string) error {
	return a.Store.Delete(id)
}

func (a *SnippetStoreAdapter) SearchSnippets(query string) []*tools.Snippet {
	list := a.Store.Search(query)
	out := make([]*tools.Snippet, len(list))
	for i, sn := range list {
		out[i] = toToolsSnippet(sn)
	}
	return out
}

func (a *SnippetStoreAdapter) RecordSnippetUse(id string) {
	a.Store.RecordUse(id)
}

func toToolsSnippet(sn *Snippet) *tools.Snippet {
	return &tools.Snippet{
		ID:          sn.ID,
		Name:        sn.Name,
		Description: sn.Description,
		Code:        sn.Code,
		FuncName:    sn.FuncName,
		Tags:        sn.Tags,
		UseCount:    sn.UseCount,
	}
}
