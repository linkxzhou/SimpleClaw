// Agent 注册表：管理多 Agent 角色配置和团队编排。
// 支持 Agent 角色分工、团队成员查询、默认 Agent 路由。

package agent

import (
	"sync"
)

// AgentProfile 一个 Agent 的完整配置。
type AgentProfile struct {
	ID           string   `json:"id"`
	Model        string   `json:"model,omitempty"`
	SystemPrompt string   `json:"systemPrompt,omitempty"`
	Tools        []string `json:"tools,omitempty"`  // 允许使用的工具列表（空=全部）
	Teams        []string `json:"teams,omitempty"`  // 所属团队
}

// TeamConfig 团队配置。
type TeamConfig struct {
	ID      string   `json:"id"`
	Agents  []string `json:"agents"` // 成员 Agent ID 列表
	Leader  string   `json:"leader"` // 团队 leader
}

// AgentRegistry Agent 注册表。
type AgentRegistry struct {
	mu       sync.RWMutex
	agents   map[string]*AgentProfile
	teams    map[string]*TeamConfig
	defaultID string // 默认 agent ID
}

// NewAgentRegistry 创建空注册表。
func NewAgentRegistry() *AgentRegistry {
	return &AgentRegistry{
		agents: make(map[string]*AgentProfile),
		teams:  make(map[string]*TeamConfig),
	}
}

// RegisterAgent 注册一个 Agent。
func (r *AgentRegistry) RegisterAgent(profile AgentProfile) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.agents[profile.ID] = &profile
	if r.defaultID == "" {
		r.defaultID = profile.ID
	}
}

// SetDefault 设置默认 Agent。
func (r *AgentRegistry) SetDefault(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.defaultID = id
}

// GetAgent 获取指定 ID 的 Agent。
func (r *AgentRegistry) GetAgent(id string) *AgentProfile {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.agents[id]
}

// GetDefault 获取默认 Agent。
func (r *AgentRegistry) GetDefault() *AgentProfile {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.agents[r.defaultID]
}

// ListAgents 列出所有已注册 Agent ID。
func (r *AgentRegistry) ListAgents() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.agents))
	for id := range r.agents {
		ids = append(ids, id)
	}
	return ids
}

// HasAgent 检查 Agent 是否已注册。
func (r *AgentRegistry) HasAgent(id string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.agents[id]
	return ok
}

// RegisterTeam 注册一个团队。
func (r *AgentRegistry) RegisterTeam(team TeamConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.teams[team.ID] = &team

	// 将团队 ID 添加到成员的 Teams 列表
	for _, agentID := range team.Agents {
		if a, ok := r.agents[agentID]; ok {
			a.Teams = appendUnique(a.Teams, team.ID)
		}
	}
}

// GetTeam 获取指定 ID 的团队。
func (r *AgentRegistry) GetTeam(id string) *TeamConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.teams[id]
}

// GetTeammates 获取指定 Agent 在指定团队中的队友（不含自己）。
func (r *AgentRegistry) GetTeammates(agentID, teamID string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	team, ok := r.teams[teamID]
	if !ok {
		return nil
	}

	var mates []string
	for _, id := range team.Agents {
		if id != agentID {
			mates = append(mates, id)
		}
	}
	return mates
}

// AgentCount 返回注册的 Agent 数量。
func (r *AgentRegistry) AgentCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.agents)
}

// TeamCount 返回注册的团队数量。
func (r *AgentRegistry) TeamCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.teams)
}

func appendUnique(slice []string, item string) []string {
	for _, s := range slice {
		if s == item {
			return slice
		}
	}
	return append(slice, item)
}
