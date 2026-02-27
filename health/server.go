// Package health 提供 HTTP 健康检查服务。
// /health — 存活探针（始终 200）
// /ready — 就绪探针（检查依赖状态）

package health

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync/atomic"
	"time"
)

// Server 健康检查 HTTP 服务器。
type Server struct {
	addr      string
	startTime time.Time
	ready     atomic.Bool
	server    *http.Server
	logger    *slog.Logger
	extra     map[string]http.HandlerFunc // 外部注册的路由
}

// NewServer 创建健康检查服务器。
func NewServer(host string, port int, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{
		addr:      fmt.Sprintf("%s:%d", host, port),
		startTime: time.Now(),
		logger:    logger,
	}
}

// SetReady 设置就绪状态。
func (s *Server) SetReady(ready bool) {
	s.ready.Store(ready)
}

// HandleFunc 注册外部 HTTP 路由，必须在 Start 之前调用。
func (s *Server) HandleFunc(pattern string, handler http.HandlerFunc) {
	if s.extra == nil {
		s.extra = make(map[string]http.HandlerFunc)
	}
	s.extra[pattern] = handler
}

// Handle 注册外部 HTTP Handler（如 http.FileServer），必须在 Start 之前调用。
func (s *Server) Handle(pattern string, handler http.Handler) {
	s.HandleFunc(pattern, handler.ServeHTTP)
}

// Start 启动健康检查服务器。
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.healthHandler)
	mux.HandleFunc("/ready", s.readyHandler)

	// 注册外部路由
	for pattern, handler := range s.extra {
		mux.HandleFunc(pattern, handler)
	}

	s.server = &http.Server{
		Addr:    s.addr,
		Handler: mux,
		BaseContext: func(_ net.Listener) context.Context {
			return ctx
		},
	}

	s.logger.Info("health server starting", "addr", s.addr)

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.server.Shutdown(shutdownCtx)
	}()

	if err := s.server.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

// healthHandler 存活探针 — 始终返回 200。
func (s *Server) healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"uptime": time.Since(s.startTime).String(),
	})
}

// readyHandler 就绪探针 — 根据 ready 状态返回 200 或 503。
func (s *Server) readyHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if s.ready.Load() {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "ready",
			"uptime": time.Since(s.startTime).String(),
		})
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "not_ready",
			"uptime": time.Since(s.startTime).String(),
		})
	}
}
