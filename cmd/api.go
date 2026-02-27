package main

// api.go 提供前端所需的所有 HTTP API 端点：
//   GET  /api/health  — 前端健康检查
//   POST /api/rpc     — RPC 统一调用
//   GET  /api/events  — SSE 事件流

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/linkxzhou/SimpleClaw/agent"
	"github.com/linkxzhou/SimpleClaw/bus"
	"github.com/linkxzhou/SimpleClaw/channels"
	"github.com/linkxzhou/SimpleClaw/config"
	"github.com/linkxzhou/SimpleClaw/cron"
	"github.com/linkxzhou/SimpleClaw/health"
	"github.com/linkxzhou/SimpleClaw/memory"
)

// APIServer 统一管理前端 API。
type APIServer struct {
	agent      *agent.Agent
	sessionMgr *memory.SessionManager
	cronSvc    *cron.Service
	channelMgr *channels.Manager
	bus        *bus.MessageBus
	cfg        *config.Config
	logger     *slog.Logger
	startTime  time.Time

	// SSE 连接管理
	sseClients   map[string]map[chan SSEEvent]struct{} // sessionKey -> set of channels
	sseMu        sync.Mutex

	// chat.send 运行上下文管理（用于 abort）
	runContexts map[string]context.CancelFunc // runID -> cancel
	runMu       sync.Mutex
}

// SSEEvent 表示一条 SSE 消息。
type SSEEvent struct {
	RunID        string      `json:"runId"`
	State        string      `json:"state"` // delta, final, error, aborted
	Message      interface{} `json:"message,omitempty"`
	ErrorMessage string      `json:"errorMessage,omitempty"`
}

// NewAPIServer 创建 APIServer 实例。
func NewAPIServer(
	agentInstance *agent.Agent,
	sessionMgr *memory.SessionManager,
	cronSvc *cron.Service,
	channelMgr *channels.Manager,
	msgBus *bus.MessageBus,
	cfg *config.Config,
	logger *slog.Logger,
) *APIServer {
	return &APIServer{
		agent:       agentInstance,
		sessionMgr:  sessionMgr,
		cronSvc:     cronSvc,
		channelMgr:  channelMgr,
		bus:         msgBus,
		cfg:         cfg,
		logger:      logger,
		startTime:   time.Now(),
		sseClients:  make(map[string]map[chan SSEEvent]struct{}),
		runContexts: make(map[string]context.CancelFunc),
	}
}

// RegisterRoutes 将 API 路由注册到 health.Server。
func (s *APIServer) RegisterRoutes(srv *health.Server) {
	srv.HandleFunc("/api/health", s.withCORS(s.handleHealth))
	srv.HandleFunc("/api/rpc", s.withCORS(s.handleRPC))
	srv.HandleFunc("/api/events", s.withCORS(s.handleEvents))

	// 尝试加载 frontend/dist 静态文件，提供 SPA 服务
	s.registerFrontend(srv)
}

// registerFrontend 查找并注册前端静态文件服务。
// 查找顺序：可执行文件同级 frontend/dist → 工作目录 frontend/dist。
func (s *APIServer) registerFrontend(srv *health.Server) {
	distDir := s.findFrontendDist()
	if distDir == "" {
		s.logger.Info("frontend/dist not found, skipping static file serving")
		return
	}

	s.logger.Info("serving frontend", "dist", distDir)

	fsys := os.DirFS(distDir)
	fileServer := http.FileServer(http.FS(fsys))

	srv.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		s.setCORS(w)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// 尝试提供静态文件；如果不存在则回退到 index.html（SPA）
		path := r.URL.Path
		if path == "/" {
			path = "/index.html"
		}

		// 检查文件是否存在
		f, err := fsys.Open(path[1:]) // 去掉前导 /
		if err == nil {
			f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}

		// 文件不存在：如果是 API 路径，返回 404；否则回退到 index.html（SPA 路由）
		if len(path) > 4 && path[:5] == "/api/" {
			http.NotFound(w, r)
			return
		}
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}

// findFrontendDist 查找 frontend/dist 目录。
func (s *APIServer) findFrontendDist() string {
	// 1. 可执行文件同级目录
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Join(filepath.Dir(exe), "frontend", "dist")
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
	}
	// 2. 当前工作目录
	if cwd, err := os.Getwd(); err == nil {
		dir := filepath.Join(cwd, "frontend", "dist")
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
	}
	return ""
}

// withCORS 为 handler 统一注入 CORS 头并处理 OPTIONS 预检请求。
func (s *APIServer) withCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.setCORS(w)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

// ============ HTTP 处理器 ============

// handleHealth 前端健康检查。
func (s *APIServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":      true,
		"uptime":  int64(time.Since(s.startTime).Seconds()),
		"version": "0.1.0",
	})
}

// rpcRequest RPC 请求格式。
type rpcRequest struct {
	Method string                 `json:"method"`
	Params map[string]interface{} `json:"params"`
}

// handleRPC 统一 RPC 入口。
func (s *APIServer) handleRPC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.rpcError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req rpcRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.rpcError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.Params == nil {
		req.Params = make(map[string]interface{})
	}

	result, err := s.dispatchRPC(r.Context(), req.Method, req.Params)
	if err != nil {
		s.rpcError(w, http.StatusOK, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"result":  result,
	})
}

// handleEvents SSE 事件流。
func (s *APIServer) handleEvents(w http.ResponseWriter, r *http.Request) {
	sessionKey := r.URL.Query().Get("sessionKey")
	if sessionKey == "" {
		http.Error(w, "missing sessionKey", http.StatusBadRequest)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := make(chan SSEEvent, 64)
	s.addSSEClient(sessionKey, ch)
	defer s.removeSSEClient(sessionKey, ch)

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(evt)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

// ============ RPC 分发 ============

// dispatchRPC 根据 method 分发到对应处理器。
func (s *APIServer) dispatchRPC(ctx context.Context, method string, params map[string]interface{}) (interface{}, error) {
	switch method {
	// Chat
	case "chat.send":
		return s.rpcChatSend(ctx, params)
	case "chat.history":
		return s.rpcChatHistory(params)
	case "chat.abort":
		return s.rpcChatAbort(params)

	// Sessions
	case "sessions.list":
		return s.rpcSessionsList(params)

	// Provider
	case "provider.list":
		return s.rpcProviderList()
	case "provider.getDefault":
		return s.rpcProviderGetDefault()
	case "provider.save":
		return s.rpcProviderSave(params)
	case "provider.delete":
		return s.rpcProviderDelete(params)
	case "provider.setDefault":
		return s.rpcProviderSetDefault(params)
	case "provider.validateKey":
		return s.rpcProviderValidateKey(params)

	// Channels
	case "channels.status":
		return s.rpcChannelsStatus()
	case "channels.add":
		return s.rpcChannelsAdd(params)
	case "channels.delete":
		return s.rpcChannelsDelete(params)
	case "channels.connect":
		return s.rpcChannelsConnect(params)
	case "channels.disconnect":
		return s.rpcChannelsDisconnect(params)

	// Cron
	case "cron.list":
		return s.rpcCronList()
	case "cron.create":
		return s.rpcCronCreate(params)
	case "cron.update":
		return s.rpcCronUpdate(params)
	case "cron.delete":
		return s.rpcCronDelete(params)
	case "cron.toggle":
		return s.rpcCronToggle(params)
	case "cron.trigger":
		return s.rpcCronTrigger(ctx, params)

	// Skills
	case "skills.status":
		return s.rpcSkillsStatus()
	case "skills.update":
		return s.rpcSkillsUpdate(params)
	case "skills.search":
		return s.rpcSkillsSearch(params)
	case "skills.install":
		return s.rpcSkillsInstall(params)
	case "skills.uninstall":
		return s.rpcSkillsUninstall(params)

	default:
		return nil, fmt.Errorf("unknown method: %s", method)
	}
}

// ============ Chat RPC ============

func (s *APIServer) rpcChatSend(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	sessionKey, _ := params["sessionKey"].(string)
	message, _ := params["message"].(string)
	if sessionKey == "" || message == "" {
		return nil, fmt.Errorf("sessionKey and message are required")
	}

	runID := fmt.Sprintf("run_%d", time.Now().UnixNano())

	// 创建可取消的上下文
	runCtx, cancel := context.WithCancel(ctx)
	s.runMu.Lock()
	s.runContexts[runID] = cancel
	s.runMu.Unlock()

	// 异步执行 Agent
	go func() {
		defer func() {
			s.runMu.Lock()
			delete(s.runContexts, runID)
			s.runMu.Unlock()
			cancel()
		}()

		// 通过消息总线发送消息，Agent 主循环会消费
		// 但 chat.send 需要直接调用 ProcessDirect 并通过 SSE 推送结果
		response, err := s.agent.ProcessDirect(runCtx, message)

		if runCtx.Err() != nil {
			// 被中止
			s.broadcastSSE(sessionKey, SSEEvent{
				RunID: runID,
				State: "aborted",
			})
			return
		}

		if err != nil {
			s.broadcastSSE(sessionKey, SSEEvent{
				RunID:        runID,
				State:        "error",
				ErrorMessage: err.Error(),
			})
			return
		}

		// 保存到 session
		if s.sessionMgr != nil {
			sess := s.sessionMgr.GetOrCreate(sessionKey)
			sess.AddMessage("user", message, nil)
			sess.AddMessage("assistant", response, nil)
			_ = s.sessionMgr.Save(sess)
		}

		// 推送最终结果
		s.broadcastSSE(sessionKey, SSEEvent{
			RunID: runID,
			State: "final",
			Message: map[string]interface{}{
				"role":    "assistant",
				"content": response,
			},
		})
	}()

	return map[string]interface{}{"runId": runID}, nil
}

func (s *APIServer) rpcChatHistory(params map[string]interface{}) (interface{}, error) {
	sessionKey, _ := params["sessionKey"].(string)
	if sessionKey == "" {
		return nil, fmt.Errorf("sessionKey is required")
	}

	limit := 200
	if l, ok := params["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}

	if s.sessionMgr == nil {
		return map[string]interface{}{"messages": []interface{}{}}, nil
	}

	sess := s.sessionMgr.GetOrCreate(sessionKey)
	messages := sess.GetHistory(limit)
	return map[string]interface{}{
		"messages": messages,
	}, nil
}

func (s *APIServer) rpcChatAbort(params map[string]interface{}) (interface{}, error) {
	sessionKey, _ := params["sessionKey"].(string)
	if sessionKey == "" {
		return nil, fmt.Errorf("sessionKey is required")
	}

	// 取消所有与该 session 相关的运行中任务
	s.runMu.Lock()
	for runID, cancel := range s.runContexts {
		// 简单起见，取消所有运行中的任务
		// 未来可以通过 runID 与 sessionKey 的映射精确控制
		cancel()
		delete(s.runContexts, runID)
	}
	s.runMu.Unlock()

	return nil, nil
}

// ============ Sessions RPC ============

func (s *APIServer) rpcSessionsList(params map[string]interface{}) (interface{}, error) {
	if s.sessionMgr == nil {
		return map[string]interface{}{"sessions": []interface{}{}}, nil
	}

	limit := 50
	if l, ok := params["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}

	sessions := s.sessionMgr.ListSessions()
	if len(sessions) > limit {
		sessions = sessions[:limit]
	}

	result := make([]map[string]interface{}, len(sessions))
	for i, info := range sessions {
		result[i] = map[string]interface{}{
			"key":           info.Key,
			"label":         info.Key,
			"displayName":   info.Key,
			"thinkingLevel": "normal",
			"model":         s.cfg.Agents.Defaults.Model,
		}
	}
	return map[string]interface{}{"sessions": result}, nil
}

// ============ Provider RPC ============

func (s *APIServer) rpcProviderList() (interface{}, error) {
	provs := []map[string]interface{}{}
	keys := s.cfg.GetAPIKeys()

	vendors := []struct {
		id, name, typ string
		key           string
	}{
		{"openrouter", "OpenRouter", "openrouter", s.cfg.Providers.OpenRouter.APIKey},
		{"anthropic", "Anthropic", "anthropic", s.cfg.Providers.Anthropic.APIKey},
		{"openai", "OpenAI", "openai", s.cfg.Providers.OpenAI.APIKey},
		{"gemini", "Gemini", "gemini", s.cfg.Providers.Gemini.APIKey},
		{"zhipu", "智谱", "zhipu", s.cfg.Providers.Zhipu.APIKey},
		{"groq", "Groq", "groq", s.cfg.Providers.Groq.APIKey},
		{"deepseek", "DeepSeek", "deepseek", s.cfg.Providers.DeepSeek.APIKey},
		{"vllm", "vLLM", "vllm", s.cfg.Providers.VLLM.APIKey},
	}

	for _, v := range vendors {
		hasKey := keys[v.id] != ""
		masked := ""
		if v.key != "" && len(v.key) > 8 {
			masked = v.key[:4] + "..." + v.key[len(v.key)-4:]
		} else if v.key != "" {
			masked = "****"
		}
		provs = append(provs, map[string]interface{}{
			"id":        v.id,
			"name":      v.name,
			"type":      v.typ,
			"enabled":   hasKey,
			"hasKey":    hasKey,
			"keyMasked": masked,
		})
	}

	return map[string]interface{}{"providers": provs}, nil
}

func (s *APIServer) rpcProviderGetDefault() (interface{}, error) {
	// 根据模型名确定默认 vendor
	model := s.cfg.Agents.Defaults.Model
	vendor := "openai"
	for _, v := range []string{"anthropic", "openai", "deepseek", "zhipu", "groq", "gemini", "openrouter", "moonshot", "qwen", "ollama", "vllm"} {
		if len(model) > len(v)+1 && model[:len(v)+1] == v+"/" {
			vendor = v
			break
		}
	}
	return map[string]interface{}{"id": vendor}, nil
}

func (s *APIServer) rpcProviderSave(params map[string]interface{}) (interface{}, error) {
	// Provider 配置修改需要持久化到 config.json
	// 目前返回成功，实际持久化需要 config 包支持写回
	s.logger.Info("provider.save called", "params", params)
	return nil, nil
}

func (s *APIServer) rpcProviderDelete(params map[string]interface{}) (interface{}, error) {
	id, _ := params["id"].(string)
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}
	s.logger.Info("provider.delete called", "id", id)
	return nil, nil
}

func (s *APIServer) rpcProviderSetDefault(params map[string]interface{}) (interface{}, error) {
	id, _ := params["id"].(string)
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}
	s.logger.Info("provider.setDefault called", "id", id)
	return nil, nil
}

func (s *APIServer) rpcProviderValidateKey(params map[string]interface{}) (interface{}, error) {
	apiKey, _ := params["apiKey"].(string)
	if apiKey == "" {
		return map[string]interface{}{"valid": false}, nil
	}
	// 简单验证：key 不为空且长度 > 10
	valid := len(apiKey) > 10
	return map[string]interface{}{"valid": valid}, nil
}

// ============ Channels RPC ============

func (s *APIServer) rpcChannelsStatus() (interface{}, error) {
	status := s.channelMgr.GetStatus()
	result := []map[string]interface{}{}

	for name, st := range status {
		statusStr := "disconnected"
		if st.Running {
			statusStr = "connected"
		}
		result = append(result, map[string]interface{}{
			"id":        name,
			"type":      name,
			"name":      name,
			"status":    statusStr,
			"connected": st.Running,
			"error":     "",
		})
	}
	return map[string]interface{}{"channels": result}, nil
}

func (s *APIServer) rpcChannelsAdd(params map[string]interface{}) (interface{}, error) {
	chType, _ := params["type"].(string)
	name, _ := params["name"].(string)
	if chType == "" || name == "" {
		return nil, fmt.Errorf("type and name are required")
	}
	s.logger.Info("channels.add called", "type", chType, "name", name)
	return nil, nil
}

func (s *APIServer) rpcChannelsDelete(params map[string]interface{}) (interface{}, error) {
	id, _ := params["id"].(string)
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}
	s.logger.Info("channels.delete called", "id", id)
	return nil, nil
}

func (s *APIServer) rpcChannelsConnect(params map[string]interface{}) (interface{}, error) {
	id, _ := params["id"].(string)
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}
	ch, ok := s.channelMgr.GetChannel(id)
	if !ok {
		return nil, fmt.Errorf("channel not found: %s", id)
	}
	go func() {
		if err := ch.Start(context.Background()); err != nil {
			s.logger.Error("channel start failed", "id", id, "error", err)
		}
	}()
	return nil, nil
}

func (s *APIServer) rpcChannelsDisconnect(params map[string]interface{}) (interface{}, error) {
	id, _ := params["id"].(string)
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}
	ch, ok := s.channelMgr.GetChannel(id)
	if !ok {
		return nil, fmt.Errorf("channel not found: %s", id)
	}
	if err := ch.Stop(); err != nil {
		return nil, fmt.Errorf("stop channel: %w", err)
	}
	return nil, nil
}

// ============ Cron RPC ============

func (s *APIServer) rpcCronList() (interface{}, error) {
	jobs := s.cronSvc.ListJobs(true)
	result := make([]map[string]interface{}, len(jobs))
	for i, j := range jobs {
		result[i] = map[string]interface{}{
			"id":      j.ID,
			"name":    j.Name,
			"message": j.Payload.Message,
			"schedule": map[string]interface{}{
				"kind":    j.Schedule.Kind,
				"atMs":    j.Schedule.AtMs,
				"everyMs": j.Schedule.EveryMs,
				"expr":    j.Schedule.Expr,
			},
			"target": map[string]interface{}{
				"channelType": j.Payload.Channel,
				"channelId":   j.Payload.To,
			},
			"enabled":   j.Enabled,
			"createdAt": time.UnixMilli(j.CreatedAtMs).Format(time.RFC3339),
			"updatedAt": time.UnixMilli(j.UpdatedAtMs).Format(time.RFC3339),
			"lastRun": map[string]interface{}{
				"time":    time.UnixMilli(j.State.LastRunAtMs).Format(time.RFC3339),
				"success": j.State.LastStatus == "ok",
			},
			"nextRun": time.UnixMilli(j.State.NextRunAtMs).Format(time.RFC3339),
		}
	}
	return map[string]interface{}{"jobs": result}, nil
}

func (s *APIServer) rpcCronCreate(params map[string]interface{}) (interface{}, error) {
	name, _ := params["name"].(string)
	message, _ := params["message"].(string)
	if name == "" || message == "" {
		return nil, fmt.Errorf("name and message are required")
	}

	// 解析调度计划
	sched := cron.Schedule{Kind: cron.ScheduleEvery, EveryMs: 3600000} // 默认每小时
	if schedStr, ok := params["schedule"].(string); ok && schedStr != "" {
		sched = cron.Schedule{Kind: cron.ScheduleCron, Expr: schedStr}
	}

	// 解析投递目标
	deliver := false
	channel := ""
	to := ""
	if target, ok := params["target"].(map[string]interface{}); ok {
		channel, _ = target["channelType"].(string)
		to, _ = target["channelId"].(string)
		if channel != "" && to != "" {
			deliver = true
		}
	}

	job := s.cronSvc.AddJob(name, sched, message, deliver, channel, to, false)
	return map[string]interface{}{"id": job.ID}, nil
}

func (s *APIServer) rpcCronUpdate(params map[string]interface{}) (interface{}, error) {
	id, _ := params["id"].(string)
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}

	// 简单实现：如果有 enabled 字段则更新启用状态
	if enabled, ok := params["enabled"].(bool); ok {
		job := s.cronSvc.EnableJob(id, enabled)
		if job == nil {
			return nil, fmt.Errorf("job not found: %s", id)
		}
	}

	return nil, nil
}

func (s *APIServer) rpcCronDelete(params map[string]interface{}) (interface{}, error) {
	id, _ := params["id"].(string)
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}
	if !s.cronSvc.RemoveJob(id) {
		return nil, fmt.Errorf("job not found: %s", id)
	}
	return nil, nil
}

func (s *APIServer) rpcCronToggle(params map[string]interface{}) (interface{}, error) {
	id, _ := params["id"].(string)
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}
	enabled, _ := params["enabled"].(bool)
	job := s.cronSvc.EnableJob(id, enabled)
	if job == nil {
		return nil, fmt.Errorf("job not found: %s", id)
	}
	return nil, nil
}

func (s *APIServer) rpcCronTrigger(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	id, _ := params["id"].(string)
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}
	if !s.cronSvc.RunJob(ctx, id, true) {
		return nil, fmt.Errorf("job not found: %s", id)
	}
	return nil, nil
}

// ============ Skills RPC ============

// 技能系统：返回内置工具列表 + workspace 中的用户技能。
func (s *APIServer) rpcSkillsStatus() (interface{}, error) {
	skills := []map[string]interface{}{
		{"id": "skill_read_file", "slug": "read-file", "name": "Read File", "description": "读取文件内容", "enabled": true, "icon": "📄", "version": "1.0.0", "author": "SimpleClaw", "isCore": true, "isBundled": true, "configurable": false},
		{"id": "skill_write_file", "slug": "write-file", "name": "Write File", "description": "写入文件内容", "enabled": true, "icon": "✏️", "version": "1.0.0", "author": "SimpleClaw", "isCore": true, "isBundled": true, "configurable": false},
		{"id": "skill_edit_file", "slug": "edit-file", "name": "Edit File", "description": "编辑文件内容", "enabled": true, "icon": "📝", "version": "1.0.0", "author": "SimpleClaw", "isCore": true, "isBundled": true, "configurable": false},
		{"id": "skill_list_dir", "slug": "list-dir", "name": "List Directory", "description": "列出目录内容", "enabled": true, "icon": "📁", "version": "1.0.0", "author": "SimpleClaw", "isCore": true, "isBundled": true, "configurable": false},
		{"id": "skill_exec", "slug": "shell-exec", "name": "Shell Execute", "description": "执行 Shell 命令", "enabled": true, "icon": "🖥️", "version": "1.0.0", "author": "SimpleClaw", "isCore": true, "isBundled": true, "configurable": false},
		{"id": "skill_web_search", "slug": "web-search", "name": "Web Search", "description": "搜索网页获取实时信息", "enabled": true, "icon": "🔍", "version": "1.0.0", "author": "SimpleClaw", "isCore": true, "isBundled": true, "configurable": true},
		{"id": "skill_web_fetch", "slug": "web-fetch", "name": "Web Fetch", "description": "抓取网页内容", "enabled": true, "icon": "🌐", "version": "1.0.0", "author": "SimpleClaw", "isCore": true, "isBundled": true, "configurable": false},
		{"id": "skill_go_run", "slug": "go-run", "name": "Go Run", "description": "执行 Go 脚本", "enabled": true, "icon": "🐹", "version": "1.0.0", "author": "SimpleClaw", "isCore": true, "isBundled": true, "configurable": false},
		{"id": "skill_message", "slug": "message", "name": "Message", "description": "发送消息到渠道", "enabled": true, "icon": "💬", "version": "1.0.0", "author": "SimpleClaw", "isCore": true, "isBundled": true, "configurable": false},
		{"id": "skill_spawn", "slug": "spawn", "name": "Spawn Agent", "description": "创建子 Agent 执行任务", "enabled": true, "icon": "🤖", "version": "1.0.0", "author": "SimpleClaw", "isCore": true, "isBundled": true, "configurable": false},
	}

	// 加载 workspace 中的用户技能
	workspace := s.cfg.WorkspacePath()
	loader := agent.NewSkillsLoader(workspace, "")
	for _, skill := range loader.ListSkills(false) {
		meta := loader.GetSkillMetadata(skill.Name)
		desc := skill.Name
		if meta != nil && meta.Description != "" {
			desc = meta.Description
		}
		skills = append(skills, map[string]interface{}{
			"id":           "skill_" + skill.Name,
			"slug":         skill.Name,
			"name":         skill.Name,
			"description":  desc,
			"enabled":      true,
			"icon":         "🧩",
			"version":      "1.0.0",
			"author":       skill.Source,
			"isCore":       false,
			"isBundled":    false,
			"configurable": false,
		})
	}

	return map[string]interface{}{"skills": skills}, nil
}

func (s *APIServer) rpcSkillsUpdate(params map[string]interface{}) (interface{}, error) {
	id, _ := params["id"].(string)
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}
	s.logger.Info("skills.update called", "id", id, "enabled", params["enabled"])
	return nil, nil
}

func (s *APIServer) rpcSkillsSearch(params map[string]interface{}) (interface{}, error) {
	// 市场搜索功能暂为存根
	return map[string]interface{}{"skills": []interface{}{}}, nil
}

func (s *APIServer) rpcSkillsInstall(params map[string]interface{}) (interface{}, error) {
	slug, _ := params["slug"].(string)
	if slug == "" {
		return nil, fmt.Errorf("slug is required")
	}
	s.logger.Info("skills.install called", "slug", slug)
	return nil, nil
}

func (s *APIServer) rpcSkillsUninstall(params map[string]interface{}) (interface{}, error) {
	slug, _ := params["slug"].(string)
	if slug == "" {
		return nil, fmt.Errorf("slug is required")
	}
	s.logger.Info("skills.uninstall called", "slug", slug)
	return nil, nil
}

// ============ SSE 管理 ============

func (s *APIServer) addSSEClient(sessionKey string, ch chan SSEEvent) {
	s.sseMu.Lock()
	defer s.sseMu.Unlock()
	if s.sseClients[sessionKey] == nil {
		s.sseClients[sessionKey] = make(map[chan SSEEvent]struct{})
	}
	s.sseClients[sessionKey][ch] = struct{}{}
}

func (s *APIServer) removeSSEClient(sessionKey string, ch chan SSEEvent) {
	s.sseMu.Lock()
	defer s.sseMu.Unlock()
	if clients, ok := s.sseClients[sessionKey]; ok {
		delete(clients, ch)
		if len(clients) == 0 {
			delete(s.sseClients, sessionKey)
		}
	}
	close(ch)
}

func (s *APIServer) broadcastSSE(sessionKey string, evt SSEEvent) {
	s.sseMu.Lock()
	clients := s.sseClients[sessionKey]
	s.sseMu.Unlock()

	for ch := range clients {
		select {
		case ch <- evt:
		default:
			// 客户端缓冲区满，跳过
		}
	}
}

// ============ 辅助方法 ============

func (s *APIServer) setCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
	w.Header().Set("Access-Control-Max-Age", "86400")
}

func (s *APIServer) rpcError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": false,
		"error":   msg,
	})
}
