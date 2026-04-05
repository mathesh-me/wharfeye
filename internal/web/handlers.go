package web

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/mathesh-me/wharfeye/internal/engine"
	"github.com/mathesh-me/wharfeye/internal/models"
)

func (s *Server) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	snap := s.engine.Collector.Latest()
	if snap == nil {
		http.Error(w, `{"error":"no data yet"}`, http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, snap)
}

func (s *Server) handleContainerDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, `{"error":"missing container id"}`, http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	detail, err := s.client.InspectContainer(ctx, id)
	if err != nil {
		http.Error(w, `{"error":"container not found"}`, http.StatusNotFound)
		return
	}

	// Include live stats if available
	snap := s.engine.Collector.Latest()
	var stats any
	if snap != nil {
		if s, ok := snap.Stats[id]; ok {
			stats = s
		}
	}

	resp := struct {
		Detail any `json:"detail"`
		Stats  any `json:"stats,omitempty"`
	}{
		Detail: detail,
		Stats:  stats,
	}
	writeJSON(w, resp)
}

func (s *Server) handleSecurity(w http.ResponseWriter, r *http.Request) {
	if s.scanner == nil {
		http.Error(w, `{"error":"scanner not available"}`, http.StatusServiceUnavailable)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	report, err := s.scanner.ScanFleet(ctx)
	if err != nil {
		slog.Error("security scan failed", "error", err)
		http.Error(w, `{"error":"scan failed"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, report)
}

func (s *Server) handleRecommendations(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	report, err := s.advisor.Analyze(ctx)
	if err != nil {
		slog.Error("advisor analysis failed", "error", err)
		http.Error(w, `{"error":"analysis failed"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, report)
}

func (s *Server) handleContainerSecurity(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, `{"error":"missing container id"}`, http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// Find container by ID
	containers, err := s.client.ListContainers(ctx)
	if err != nil {
		http.Error(w, `{"error":"listing containers"}`, http.StatusInternalServerError)
		return
	}

	var target *models.Container
	for i, c := range containers {
		if c.ID == id || c.Name == id {
			target = &containers[i]
			break
		}
	}
	if target == nil {
		http.Error(w, `{"error":"container not found"}`, http.StatusNotFound)
		return
	}

	report, err := s.scanner.ScanContainer(ctx, *target)
	if err != nil {
		http.Error(w, `{"error":"scan failed"}`, http.StatusInternalServerError)
		return
	}

	hardening := engine.GetContainerHardening(report)
	resp := struct {
		Report    any `json:"report"`
		Hardening any `json:"hardening"`
	}{report, hardening}
	writeJSON(w, resp)
}

func (s *Server) handleContainerRecommendations(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, `{"error":"missing container id"}`, http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	report, err := s.advisor.Analyze(ctx)
	if err != nil {
		http.Error(w, `{"error":"analysis failed"}`, http.StatusInternalServerError)
		return
	}

	// Filter to this container
	var filtered []models.Recommendation
	for _, rec := range report.Recommendations {
		if rec.ContainerID == id || rec.ContainerName == id {
			filtered = append(filtered, rec)
		}
	}

	resp := struct {
		ContainerID     string                   `json:"container_id"`
		Recommendations []models.Recommendation  `json:"recommendations"`
	}{id, filtered}
	writeJSON(w, resp)
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("websocket upgrade failed", "error", err)
		return
	}

	s.addClient(conn)
	slog.Debug("websocket client connected")

	// Send current snapshot immediately
	if snap := s.engine.Collector.Latest(); snap != nil {
		if err := conn.WriteJSON(snap); err != nil {
			s.removeClient(conn)
			conn.Close()
			return
		}
	}

	// Read loop (handles client disconnect)
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			s.removeClient(conn)
			conn.Close()
			return
		}
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		slog.Error("encoding json response", "error", err)
	}
}
