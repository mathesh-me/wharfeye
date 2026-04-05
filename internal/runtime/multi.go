package runtime

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/mathesh-me/wharfeye/internal/models"
)

// MultiClient aggregates containers from multiple runtime backends.
// It implements the Client interface so all downstream consumers
// (Engine, Scanner, Advisor, Web, TUI) work transparently.
type MultiClient struct {
	clients []Client
	infos   []*RuntimeInfo

	mu      sync.RWMutex
	routing map[string]Client // containerID -> client
}

// NewMultiClient creates a client that merges results from all given clients.
func NewMultiClient(clients []Client, infos []*RuntimeInfo) *MultiClient {
	return &MultiClient{
		clients: clients,
		infos:   infos,
		routing: make(map[string]Client),
	}
}

func (m *MultiClient) Info(ctx context.Context) (*RuntimeInfo, error) {
	if len(m.infos) == 0 {
		return nil, fmt.Errorf("no runtimes available")
	}
	// Return composite info with deduplicated names
	seen := make(map[string]bool)
	var names []string
	for _, info := range m.infos {
		if !seen[info.Name] {
			seen[info.Name] = true
			names = append(names, info.Name)
		}
	}
	return &RuntimeInfo{
		Name:    strings.Join(names, " + "),
		Version: m.infos[0].Version,
		OS:      m.infos[0].OS,
		Arch:    m.infos[0].Arch,
	}, nil
}

func (m *MultiClient) ListContainers(ctx context.Context) ([]models.Container, error) {
	var all []models.Container
	m.mu.Lock()
	defer m.mu.Unlock()
	// Clear old routing
	m.routing = make(map[string]Client)

	for _, c := range m.clients {
		containers, err := c.ListContainers(ctx)
		if err != nil {
			continue // skip unavailable runtimes
		}
		for _, ctr := range containers {
			m.routing[ctr.ID] = c
		}
		all = append(all, containers...)
	}
	return all, nil
}

func (m *MultiClient) InspectContainer(ctx context.Context, id string) (*models.ContainerDetail, error) {
	if c := m.routeClient(id); c != nil {
		return c.InspectContainer(ctx, id)
	}
	// Fallback: try each client, recovering from panics in poorly-handling clients
	for _, c := range m.clients {
		detail, err := m.safeInspect(ctx, c, id)
		if err == nil {
			return detail, nil
		}
	}
	return nil, fmt.Errorf("container %s not found in any runtime", id)
}

// safeInspect wraps InspectContainer with panic recovery for resilience.
func (m *MultiClient) safeInspect(ctx context.Context, c Client, id string) (detail *models.ContainerDetail, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("inspect panicked: %v", r)
		}
	}()
	return c.InspectContainer(ctx, id)
}

func (m *MultiClient) ContainerStats(ctx context.Context, id string) (*models.ContainerStats, error) {
	if c := m.routeClient(id); c != nil {
		return c.ContainerStats(ctx, id)
	}
	for _, c := range m.clients {
		stats, err := c.ContainerStats(ctx, id)
		if err == nil {
			return stats, nil
		}
	}
	return nil, fmt.Errorf("stats for container %s not found", id)
}

func (m *MultiClient) StreamStats(ctx context.Context, id string) (<-chan *models.ContainerStats, error) {
	if c := m.routeClient(id); c != nil {
		return c.StreamStats(ctx, id)
	}
	for _, c := range m.clients {
		ch, err := c.StreamStats(ctx, id)
		if err == nil {
			return ch, nil
		}
	}
	return nil, fmt.Errorf("stream for container %s not found", id)
}

func (m *MultiClient) ListImages(ctx context.Context) ([]models.Image, error) {
	var all []models.Image
	for _, c := range m.clients {
		images, err := c.ListImages(ctx)
		if err == nil {
			all = append(all, images...)
		}
	}
	return all, nil
}

func (m *MultiClient) InspectImage(ctx context.Context, id string) (*models.ImageDetail, error) {
	for _, c := range m.clients {
		detail, err := c.InspectImage(ctx, id)
		if err == nil {
			return detail, nil
		}
	}
	return nil, fmt.Errorf("image %s not found in any runtime", id)
}

func (m *MultiClient) ListVolumes(ctx context.Context) ([]models.Volume, error) {
	var all []models.Volume
	for _, c := range m.clients {
		volumes, err := c.ListVolumes(ctx)
		if err == nil {
			all = append(all, volumes...)
		}
	}
	return all, nil
}

func (m *MultiClient) StopContainer(ctx context.Context, id string) error {
	if c := m.routeClient(id); c != nil {
		return c.StopContainer(ctx, id)
	}
	return fmt.Errorf("container %s not found in any runtime", id)
}

func (m *MultiClient) RestartContainer(ctx context.Context, id string) error {
	if c := m.routeClient(id); c != nil {
		return c.RestartContainer(ctx, id)
	}
	return fmt.Errorf("container %s not found in any runtime", id)
}

func (m *MultiClient) routeClient(id string) Client {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.routing[id]
}

// AutoDetectAll probes all available runtimes and returns a MultiClient
// that aggregates containers from all of them. Also returns info for each
// runtime for display purposes.
func AutoDetectAll(ctx context.Context) (Client, []*RuntimeInfo, error) {
	found := DetectAll()
	if len(found) == 0 {
		return nil, nil, fmt.Errorf("no container runtime found")
	}

	var clients []Client
	var infos []*RuntimeInfo

	for _, r := range found {
		c, err := NewClient(r.Name, r.Socket)
		if err != nil {
			continue
		}
		info, err := c.Info(ctx)
		if err != nil {
			continue
		}
		clients = append(clients, c)
		infos = append(infos, info)
	}

	if len(clients) == 0 {
		return nil, nil, fmt.Errorf("no container runtime could be connected")
	}

	// Single runtime - return directly, no wrapping overhead
	if len(clients) == 1 {
		return clients[0], infos, nil
	}

	return NewMultiClient(clients, infos), infos, nil
}
