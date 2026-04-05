package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mathesh-me/wharfeye/internal/engine"
	"github.com/mathesh-me/wharfeye/internal/models"
	"github.com/mathesh-me/wharfeye/internal/tui/components"
	"github.com/mathesh-me/wharfeye/internal/tui/views"
)

// Tab represents a navigation tab.
type Tab int

const (
	TabDashboard Tab = iota
	TabSecurity
	TabAdvisor
	TabDetail
)

// Model is the main bubbletea model for the TUI.
type Model struct {
	engine     *engine.Engine
	scanner    *engine.Scanner
	subID      string
	snapshotCh <-chan engine.Snapshot

	// State
	snapshot       *engine.Snapshot
	cpuHistory     map[string][]float64
	securityReport *models.FleetSecurityReport
	advisorReport  *models.AdvisorReport
	advisor        *engine.Advisor
	selected       int
	activeTab      Tab
	filter         string
	filtering      bool
	scanning       bool
	width          int
	height         int
	ready          bool
	err            error

	cancel context.CancelFunc
}

// Messages
type snapshotMsg engine.Snapshot
type securityScanDoneMsg *models.FleetSecurityReport
type securityScanErrMsg error
type advisorDoneMsg *models.AdvisorReport
type errMsg error

// NewModel creates a new TUI model wired to the engine.
func NewModel(eng *engine.Engine, scanner *engine.Scanner) Model {
	advisor := engine.NewAdvisor(eng.Client, eng.Collector)
	return Model{
		engine:     eng,
		scanner:    scanner,
		advisor:    advisor,
		cpuHistory: make(map[string][]float64),
		activeTab:  TabDashboard,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.startEngine(),
		tea.WindowSize(),
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		return m, nil

	case snapshotMsg:
		snap := engine.Snapshot(msg)
		m.snapshot = &snap
		m.updateCPUHistory(&snap)
		return m, m.waitForSnapshot()

	case securityScanDoneMsg:
		m.securityReport = msg
		m.scanning = false
		m.activeTab = TabSecurity
		m.selected = 0
		return m, nil

	case securityScanErrMsg:
		m.err = msg
		m.scanning = false
		return m, nil

	case advisorDoneMsg:
		m.advisorReport = msg
		m.activeTab = TabAdvisor
		m.selected = 0
		return m, nil

	case errMsg:
		m.err = msg
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.filtering {
		switch msg.String() {
		case "esc":
			m.filtering = false
			m.filter = ""
			m.selected = 0
			return m, nil
		case "enter":
			m.filtering = false
			m.selected = 0
			return m, nil
		case "backspace":
			if len(m.filter) > 0 {
				m.filter = m.filter[:len(m.filter)-1]
			}
			m.selected = 0
			return m, nil
		default:
			if len(msg.String()) == 1 {
				m.filter += msg.String()
				m.selected = 0
			}
			return m, nil
		}
	}

	switch msg.String() {
	case "q", "ctrl+c":
		if m.cancel != nil {
			m.cancel()
		}
		return m, tea.Quit

	case "1":
		m.activeTab = TabDashboard
		m.selected = 0
		return m, nil

	case "2":
		m.activeTab = TabSecurity
		m.selected = 0
		return m, nil

	case "3":
		m.activeTab = TabAdvisor
		m.selected = 0
		if m.advisorReport == nil {
			return m, m.runAdvisorAnalysis()
		}
		return m, nil

	case "enter":
		if m.activeTab == TabDashboard {
			m.activeTab = TabDetail
		}
		return m, nil

	case "esc", "backspace":
		if m.activeTab != TabDashboard {
			m.activeTab = TabDashboard
		}
		return m, nil

	case "up", "k":
		if m.selected > 0 {
			m.selected--
		}
		return m, nil

	case "down", "j":
		maxIdx := m.maxSelected() - 1
		if m.selected < maxIdx {
			m.selected++
		}
		return m, nil

	case "/":
		m.filtering = true
		m.filter = ""
		return m, nil

	case "s":
		if !m.scanning && m.scanner != nil {
			m.scanning = true
			return m, m.runSecurityScan()
		}
		return m, nil

	case "r":
		return m, nil
	}

	return m, nil
}

func (m Model) View() string {
	if !m.ready {
		return "Initializing..."
	}

	if m.err != nil {
		return fmt.Sprintf("Error: %v\n\nPress q to quit.", m.err)
	}

	var content string

	switch m.activeTab {
	case TabDashboard:
		content = views.DashboardView(m.snapshot, m.cpuHistory,
			m.selected, m.filter, m.width, m.height-2)

	case TabDetail:
		if m.snapshot != nil {
			ctr, stats := views.ContainerDetailFromSnapshot(m.snapshot, m.filter, m.selected)
			hist := m.cpuHistory[ctr.ID]
			content = views.DetailView(ctr, stats, hist, m.width, m.height-2)
		} else {
			content = "No data available"
		}

	case TabSecurity:
		if m.scanning {
			content = lipgloss.NewStyle().
				Width(m.width).
				Align(lipgloss.Center).
				Padding(2, 0).
				Render("Scanning containers...")
		} else {
			content = views.SecurityView(m.securityReport, m.selected, m.width, m.height-2)
		}

	case TabAdvisor:
		content = views.AdvisorView(m.advisorReport, m.selected, m.width, m.height-2)
	}

	// Filter input bar
	if m.filtering {
		filterBar := lipgloss.NewStyle().
			Foreground(lipgloss.Color("39")).
			Padding(0, 1).
			Render(fmt.Sprintf("Filter: %s█", m.filter))
		content += "\n" + filterBar
	}

	// Status bar
	host := engine.HostSummary{}
	if m.snapshot != nil {
		host = m.snapshot.Host
	}
	statusBar := components.StatusBar(host, m.width, int(m.activeTab))

	contentLines := strings.Count(content, "\n") + 1
	padding := m.height - contentLines - 1
	if padding > 0 {
		content += strings.Repeat("\n", padding)
	}

	return content + statusBar
}

func (m *Model) startEngine() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		m.cancel = cancel

		subID, ch := m.engine.Collector.Subscribe()
		m.subID = subID
		m.snapshotCh = ch

		go func() {
			if err := m.engine.Start(ctx); err != nil && ctx.Err() == nil {
				// Engine failed
			}
		}()

		select {
		case snap := <-ch:
			return snapshotMsg(snap)
		case <-ctx.Done():
			return nil
		}
	}
}

func (m *Model) waitForSnapshot() tea.Cmd {
	ch := m.snapshotCh
	return func() tea.Msg {
		snap, ok := <-ch
		if !ok {
			return nil
		}
		return snapshotMsg(snap)
	}
}

func (m *Model) runSecurityScan() tea.Cmd {
	scanner := m.scanner
	return func() tea.Msg {
		report, err := scanner.ScanFleet(context.Background())
		if err != nil {
			return securityScanErrMsg(err)
		}
		return securityScanDoneMsg(report)
	}
}

func (m *Model) runAdvisorAnalysis() tea.Cmd {
	advisor := m.advisor
	return func() tea.Msg {
		report, err := advisor.Analyze(context.Background())
		if err != nil {
			return errMsg(err)
		}
		return advisorDoneMsg(report)
	}
}

func (m *Model) updateCPUHistory(snapshot *engine.Snapshot) {
	const maxHistory = 30

	for _, ctr := range snapshot.Containers {
		if s, ok := snapshot.Stats[ctr.ID]; ok {
			hist := m.cpuHistory[ctr.ID]
			hist = append(hist, s.CPU.Percent)
			if len(hist) > maxHistory {
				hist = hist[len(hist)-maxHistory:]
			}
			m.cpuHistory[ctr.ID] = hist
		}
	}
}

func (m *Model) maxSelected() int {
	if m.snapshot == nil {
		return 0
	}

	switch m.activeTab {
	case TabSecurity:
		if m.securityReport != nil {
			return len(m.securityReport.Containers)
		}
		return 0
	case TabAdvisor:
		if m.advisorReport != nil {
			return len(m.advisorReport.Recommendations)
		}
		return 0
	default:
		return views.FilteredContainerCount(m.snapshot, m.filter)
	}
}
