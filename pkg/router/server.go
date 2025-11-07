package router

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
)

// Server HTTP API 服务器
// 提供路由查询接口（用于测试）
type Server struct {
	router *RouterAgent
	log    *logrus.Logger
	port   int
}

// NewServer 创建 HTTP 服务器
func NewServer(router *RouterAgent, port int, log *logrus.Logger) *Server {
	return &Server{
		router: router,
		port:   port,
		log:    log,
	}
}

// Start 启动 HTTP 服务器
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// 路由计算接口
	mux.HandleFunc("/route", s.handleRoute)

	// 健康检查接口
	mux.HandleFunc("/health", s.handleHealth)

	// 缓存统计接口
	mux.HandleFunc("/stats", s.handleStats)

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(shutdownCtx)
	}()

	s.log.WithField("port", s.port).Info("Starting HTTP API server")
	return server.ListenAndServe()
}

// handleRoute 处理路由查询请求
// GET /route?service=namespace/servicename
func (s *Server) handleRoute(w http.ResponseWriter, r *http.Request) {
	serviceName := r.URL.Query().Get("service")
	if serviceName == "" {
		http.Error(w, "missing service parameter", http.StatusBadRequest)
		return
	}

	startTime := time.Now()

	weights, err := s.router.ComputeRouting(r.Context(), serviceName)
	if err != nil {
		s.log.WithError(err).WithField("service", serviceName).Warn("Routing computation failed")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	duration := time.Since(startTime)

	response := map[string]interface{}{
		"service":      serviceName,
		"algorithm":    s.router.algorithm.Name(),
		"weights":      weights,
		"duration_ms":  duration.Milliseconds(),
		"duration_us":  duration.Microseconds(),
		"endpoints_count": len(weights),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)

	s.log.WithFields(logrus.Fields{
		"service":   serviceName,
		"endpoints": len(weights),
		"duration":  fmt.Sprintf("%dµs", duration.Microseconds()),
	}).Info("Routing computed successfully")
}

// handleHealth 健康检查
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
		"node":   s.router.nodeName,
	})
}

// handleStats 获取缓存统计
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	stats := s.router.GetCacheStats()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}
