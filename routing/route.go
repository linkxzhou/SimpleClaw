// Package routing 提供多 Agent 消息路由解析。
// 支持 7 级优先级级联路由：peer > parent_peer > guild > team > account > channel_wildcard > default。

package routing

import "github.com/linkxzhou/SimpleClaw/utils"

// BindingConfig 描述一条 Agent 路由绑定规则。
type BindingConfig struct {
	Agent   string `json:"agent"`             // 目标 Agent ID
	Channel string `json:"channel"`           // 渠道名（"*" 表示通配）
	Peer    string `json:"peer,omitempty"`    // 对话方 ID
	Guild   string `json:"guild,omitempty"`   // 群组/服务器 ID
	Team    string `json:"team,omitempty"`    // 团队 ID
	Account string `json:"account,omitempty"` // 账号 ID
}

// ResolvedRoute 是路由解析的结果。
type ResolvedRoute struct {
	AgentID    string // 匹配到的 Agent ID
	SessionKey string // 会话键
	MatchedBy  string // 匹配方式描述
}

// RouteResolver 解析入站消息应路由到哪个 Agent。
type RouteResolver struct {
	bindings       []BindingConfig
	defaultAgentID string
}

// NewRouteResolver 创建路由解析器。
func NewRouteResolver(bindings []BindingConfig, defaultAgentID string) *RouteResolver {
	return &RouteResolver{
		bindings:       bindings,
		defaultAgentID: defaultAgentID,
	}
}

// Resolve 根据入站消息的渠道和身份信息解析路由。
// 7 级优先级：peer > guild > team > account > channel_wildcard > default。
func (r *RouteResolver) Resolve(channel, senderID, chatID string) *ResolvedRoute {
	sessionKey := utils.BuildSessionKey(channel, chatID)

	// Level 1: Peer 精确匹配
	for _, b := range r.bindings {
		if b.Peer != "" && b.Peer == senderID && matchChannel(b.Channel, channel) {
			return &ResolvedRoute{AgentID: b.Agent, SessionKey: sessionKey, MatchedBy: "binding.peer"}
		}
	}

	// Level 2: Guild 匹配
	for _, b := range r.bindings {
		if b.Guild != "" && b.Guild == chatID && matchChannel(b.Channel, channel) {
			return &ResolvedRoute{AgentID: b.Agent, SessionKey: sessionKey, MatchedBy: "binding.guild"}
		}
	}

	// Level 3: Team 匹配（使用 chatID 作为 team 标识）
	for _, b := range r.bindings {
		if b.Team != "" && b.Team == chatID && matchChannel(b.Channel, channel) {
			return &ResolvedRoute{AgentID: b.Agent, SessionKey: sessionKey, MatchedBy: "binding.team"}
		}
	}

	// Level 4: Account 匹配
	for _, b := range r.bindings {
		if b.Account != "" && b.Account == senderID && matchChannel(b.Channel, channel) {
			return &ResolvedRoute{AgentID: b.Agent, SessionKey: sessionKey, MatchedBy: "binding.account"}
		}
	}

	// Level 5: Channel 通配符匹配
	for _, b := range r.bindings {
		if b.Channel == "*" && b.Peer == "" && b.Guild == "" && b.Team == "" && b.Account == "" {
			return &ResolvedRoute{AgentID: b.Agent, SessionKey: sessionKey, MatchedBy: "binding.channel"}
		}
	}

	// Level 6: Channel 精确匹配（无其他条件）
	for _, b := range r.bindings {
		if b.Channel == channel && b.Peer == "" && b.Guild == "" && b.Team == "" && b.Account == "" {
			return &ResolvedRoute{AgentID: b.Agent, SessionKey: sessionKey, MatchedBy: "binding.channel_exact"}
		}
	}

	// Level 7: 默认
	return &ResolvedRoute{AgentID: r.defaultAgentID, SessionKey: sessionKey, MatchedBy: "default"}
}

// matchChannel 检查绑定的 channel 是否匹配。
func matchChannel(bindingChannel, actualChannel string) bool {
	return bindingChannel == "*" || bindingChannel == actualChannel
}
