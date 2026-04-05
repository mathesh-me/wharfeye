package web

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/mathesh-me/wharfeye/internal/engine"
	"github.com/mathesh-me/wharfeye/internal/runtime"
)

//go:embed static/*
var staticFiles embed.FS

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Server is the web dashboard HTTP server.
type Server struct {
	engine  *engine.Engine
	scanner *engine.Scanner
	advisor *engine.Advisor
	client  runtime.Client

	mu      sync.RWMutex
	clients map[*websocket.Conn]struct{}
}

// NewServer creates a new web server wired to the engine.
func NewServer(eng *engine.Engine, scanner *engine.Scanner, client runtime.Client) *Server {
	advisor := engine.NewAdvisor(client, eng.Collector)
	return &Server{
		engine:  eng,
		scanner: scanner,
		advisor: advisor,
		client:  client,
		clients: make(map[*websocket.Conn]struct{}),
	}
}

// Start launches the HTTP server and the engine collector.
func (s *Server) Start(ctx context.Context, addr string) error {
	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("GET /api/snapshot", s.handleSnapshot)
	mux.HandleFunc("GET /api/containers/{id}", s.handleContainerDetail)
	mux.HandleFunc("GET /api/security", s.handleSecurity)
	mux.HandleFunc("GET /api/recommendations", s.handleRecommendations)
	mux.HandleFunc("GET /api/security/{id}", s.handleContainerSecurity)
	mux.HandleFunc("GET /api/recommendations/{id}", s.handleContainerRecommendations)
	mux.HandleFunc("GET /ws", s.handleWebSocket)

	// Static files
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return fmt.Errorf("creating static fs: %w", err)
	}
	mux.Handle("/", http.FileServer(http.FS(staticFS)))

	// Start engine in background
	go func() {
		if err := s.engine.Start(ctx); err != nil && ctx.Err() == nil {
			slog.Error("engine stopped", "error", err)
		}
	}()

	// Start broadcasting snapshots to WebSocket clients
	go s.broadcastLoop(ctx)

	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("server shutdown", "error", err)
		}
	}()

	slog.Info("web dashboard starting", "addr", addr)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		return fmt.Errorf("http server: %w", err)
	}
	return nil
}

// broadcastLoop subscribes to engine snapshots and pushes them to all WebSocket clients.
func (s *Server) broadcastLoop(ctx context.Context) {
	subID, ch := s.engine.Collector.Subscribe()
	defer s.engine.Collector.Unsubscribe(subID)

	for {
		select {
		case <-ctx.Done():
			return
		case snap, ok := <-ch:
			if !ok {
				return
			}
			s.broadcast(snap)
		}
	}
}

// broadcast sends a snapshot to all connected WebSocket clients.
func (s *Server) broadcast(snap engine.Snapshot) {
	s.mu.RLock()
	clients := make([]*websocket.Conn, 0, len(s.clients))
	for c := range s.clients {
		clients = append(clients, c)
	}
	s.mu.RUnlock()

	for _, c := range clients {
		if err := c.WriteJSON(snap); err != nil {
			slog.Debug("websocket write error, removing client", "error", err)
			s.removeClient(c)
			c.Close()
		}
	}
}

func (s *Server) addClient(c *websocket.Conn) {
	s.mu.Lock()
	s.clients[c] = struct{}{}
	s.mu.Unlock()
}

func (s *Server) removeClient(c *websocket.Conn) {
	s.mu.Lock()
	delete(s.clients, c)
	s.mu.Unlock()
}
