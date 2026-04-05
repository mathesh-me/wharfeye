const chartState = {
  cpu: null,
  mem: null,
  fleetCpu: null,
  fleetMem: null,
  fleetNet: null,
};

function app() {
  return {
    tab: 'dashboard',
    filter: '',
    snapshot: null,
    selectedContainer: null,
    detail: null,
    cpuHistory: {},
    memHistory: {},

    // Fleet-level history for dashboard charts
    fleetCpuHistory: [],
    fleetMemHistory: [],
    fleetNetRxHistory: [],
    fleetNetTxHistory: [],
    prevNetRx: null,
    prevNetTx: null,

    // Security
    securityReport: null,
    securityLoading: false,
    securityError: null,
    securityFilter: 'all',

    // Advisor
    advisorReport: null,
    advisorLoading: false,
    advisorError: null,
    advisorFilter: 'all',

    // Per-container drill-down
    selectedSecurityContainer: null,
    containerSecurity: null,
    selectedAdvisorContainer: null,
    containerRecommendations: null,

    ws: null,

    init() {
      this.connectWebSocket();
    },

    connectWebSocket() {
      const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
      this.ws = new WebSocket(`${proto}//${location.host}/ws`);

      this.ws.onmessage = (e) => {
        const snap = JSON.parse(e.data);
        this.snapshot = snap;
        this.updateHistory(snap);
        if (this.selectedContainer && chartState.cpu) {
          this.updateCharts();
        }
      };

      this.ws.onclose = () => {
        setTimeout(() => this.connectWebSocket(), 2000);
      };

      this.ws.onerror = () => {
        this.ws.close();
      };
    },

    updateHistory(snap) {
      const maxPoints = 60;
      if (!snap.stats) return;

      // Per-container history
      for (const [id, stats] of Object.entries(snap.stats)) {
        if (!this.cpuHistory[id]) this.cpuHistory[id] = [];
        if (!this.memHistory[id]) this.memHistory[id] = [];

        this.cpuHistory[id].push(stats.cpu.percent);
        this.memHistory[id].push(stats.memory.usage);

        if (this.cpuHistory[id].length > maxPoints) {
          this.cpuHistory[id] = this.cpuHistory[id].slice(-maxPoints);
        }
        if (this.memHistory[id].length > maxPoints) {
          this.memHistory[id] = this.memHistory[id].slice(-maxPoints);
        }
      }

      // Fleet-level aggregates
      let totalCpu = 0, totalMem = 0, totalRx = 0, totalTx = 0;
      for (const stats of Object.values(snap.stats)) {
        totalCpu += stats.cpu?.percent || 0;
        totalMem += stats.memory?.usage || 0;
        totalRx += stats.network_io?.rx_bytes || 0;
        totalTx += stats.network_io?.tx_bytes || 0;
      }
      this.fleetCpuHistory.push(totalCpu);
      this.fleetMemHistory.push(totalMem);

      // Network: compute delta (bytes/interval) since counters are cumulative
      if (this.prevNetRx !== null) {
        const dRx = Math.max(0, totalRx - this.prevNetRx);
        const dTx = Math.max(0, totalTx - this.prevNetTx);
        this.fleetNetRxHistory.push(dRx);
        this.fleetNetTxHistory.push(dTx);
      }
      this.prevNetRx = totalRx;
      this.prevNetTx = totalTx;

      if (this.fleetCpuHistory.length > maxPoints) {
        this.fleetCpuHistory = this.fleetCpuHistory.slice(-maxPoints);
        this.fleetMemHistory = this.fleetMemHistory.slice(-maxPoints);
        this.fleetNetRxHistory = this.fleetNetRxHistory.slice(-maxPoints);
        this.fleetNetTxHistory = this.fleetNetTxHistory.slice(-maxPoints);
      }

      // Update fleet charts if visible
      if (this.tab === 'dashboard' && !this.selectedContainer) {
        this.$nextTick(() => {
          this.ensureFleetCharts();
        });
      }
    },

    switchTab(t) {
      this.tab = t;
      this.selectedContainer = null;
      this.detail = null;
      this.selectedSecurityContainer = null;
      this.containerSecurity = null;
      this.selectedAdvisorContainer = null;
      this.containerRecommendations = null;

      if (t === 'dashboard') {
        this.$nextTick(() => this.ensureFleetCharts(true));
      }
      if (t === 'security' && !this.securityReport) {
        this.loadSecurity();
      }
      if (t === 'advisor' && !this.advisorReport) {
        this.loadAdvisor();
      }
    },

    filteredContainers() {
      if (!this.snapshot || !this.snapshot.containers) return [];
      if (!this.filter) return this.snapshot.containers;
      const f = this.filter.toLowerCase();
      return this.snapshot.containers.filter(c =>
        c.name.toLowerCase().includes(f) || c.image.toLowerCase().includes(f)
      );
    },

    // Security filtering
    filteredChecks(checks) {
      if (!checks) return [];
      const failed = checks.filter(c => !c.passed);
      if (this.securityFilter === 'all') return failed;
      return failed.filter(c => c.severity === this.securityFilter);
    },

    filteredSecurityContainers() {
      if (!this.securityReport?.containers) return [];
      if (this.securityFilter === 'all') return this.securityReport.containers;
      return this.securityReport.containers.filter(ctr =>
        (ctr.checks || []).some(c => !c.passed && c.severity === this.securityFilter)
      );
    },

    // Advisor filtering
    filteredRecommendations() {
      if (!this.advisorReport?.recommendations) return [];
      if (this.advisorFilter === 'all') return this.advisorReport.recommendations;
      return this.advisorReport.recommendations.filter(r => r.priority === this.advisorFilter);
    },

    getStats(id) {
      if (!this.snapshot || !this.snapshot.stats) return null;
      return this.snapshot.stats[id] || null;
    },

    async selectContainer(id) {
      this.selectedContainer = id;
      this.detail = null;

      try {
        const resp = await fetch(`/api/containers/${id}`);
        if (resp.ok) {
          this.detail = await resp.json();
          this.$nextTick(() => this.initCharts());
        }
      } catch (e) {
        console.error('Failed to load container detail:', e);
      }
    },

    initCharts() {
      if (chartState.cpu) { chartState.cpu.destroy(); chartState.cpu = null; }
      if (chartState.mem) { chartState.mem.destroy(); chartState.mem = null; }

      const cpuCanvas = document.getElementById('cpuChart');
      const memCanvas = document.getElementById('memChart');
      if (!cpuCanvas || !memCanvas) return;

      const labels = Array.from({ length: 60 }, (_, i) => '');

      chartState.cpu = new Chart(cpuCanvas, {
        type: 'line',
        data: {
          labels,
          datasets: [{
            label: 'CPU %',
            data: this.cpuHistory[this.selectedContainer] || [],
            borderColor: '#3b82f6',
            backgroundColor: 'rgba(59,130,246,0.1)',
            fill: true,
            tension: 0.3,
            pointRadius: 0,
          }]
        },
        options: chartOptions('CPU %', 0, 100)
      });

      chartState.mem = new Chart(memCanvas, {
        type: 'line',
        data: {
          labels,
          datasets: [{
            label: 'Memory',
            data: (this.memHistory[this.selectedContainer] || []).map(v => v / (1024 * 1024)),
            borderColor: '#8b5cf6',
            backgroundColor: 'rgba(139,92,246,0.1)',
            fill: true,
            tension: 0.3,
            pointRadius: 0,
          }]
        },
        options: chartOptions('MB')
      });
    },

    updateCharts() {
      if (!this.selectedContainer) return;

      if (chartState.cpu) {
        chartState.cpu.data.datasets[0].data = this.cpuHistory[this.selectedContainer] || [];
        chartState.cpu.update('none');
      }
      if (chartState.mem) {
        chartState.mem.data.datasets[0].data = (this.memHistory[this.selectedContainer] || []).map(v => v / (1024 * 1024));
        chartState.mem.update('none');
      }
    },

    ensureFleetCharts(force = false) {
      const cpuEl = document.getElementById('fleetCpuChart');
      const memEl = document.getElementById('fleetMemChart');
      const netEl = document.getElementById('fleetNetChart');
      if (!cpuEl || !memEl || !netEl) return;

      const canvasesReady = [cpuEl, memEl, netEl].every(el => el.offsetWidth > 0 && el.offsetHeight > 0);
      if (!canvasesReady) {
        setTimeout(() => this.ensureFleetCharts(force), 500);
        return;
      }

      if (force || !chartState.fleetCpu || !chartState.fleetMem || !chartState.fleetNet) {
        this.initFleetCharts();
        return;
      }

      this.updateFleetCharts();
    },
    initFleetCharts() {
      if (chartState.fleetCpu) { chartState.fleetCpu.destroy(); chartState.fleetCpu = null; }
      if (chartState.fleetMem) { chartState.fleetMem.destroy(); chartState.fleetMem = null; }
      if (chartState.fleetNet) { chartState.fleetNet.destroy(); chartState.fleetNet = null; }

      const cpuEl = document.getElementById('fleetCpuChart');
      const memEl = document.getElementById('fleetMemChart');
      const netEl = document.getElementById('fleetNetChart');
      if (!cpuEl || !memEl || !netEl) return;

      const cpuLabels = Array.from({ length: Math.max(this.fleetCpuHistory.length, 1) }, () => '');
      const memLabels = Array.from({ length: Math.max(this.fleetMemHistory.length, 1) }, () => '');
      const netLabels = Array.from({ length: Math.max(this.fleetNetRxHistory.length, this.fleetNetTxHistory.length, 1) }, () => '');

      chartState.fleetCpu = new Chart(cpuEl, {
        type: 'line',
        data: {
          labels: cpuLabels,
          datasets: [{
            label: 'CPU %',
            data: [...this.fleetCpuHistory],
            borderColor: '#06b6d4',
            backgroundColor: 'rgba(6,182,212,0.15)',
            fill: true, tension: 0.3, pointRadius: 1, borderWidth: 2,
          }]
        },
        options: chartOptions('CPU %', 0)
      });

      chartState.fleetMem = new Chart(memEl, {
        type: 'line',
        data: {
          labels: memLabels,
          datasets: [{
            label: 'Memory (MB)',
            data: this.fleetMemHistory.map(v => v / (1024 * 1024)),
            borderColor: '#22c55e',
            backgroundColor: 'rgba(34,197,94,0.15)',
            fill: true, tension: 0.3, pointRadius: 1, borderWidth: 2,
          }]
        },
        options: chartOptions('MB')
      });

      chartState.fleetNet = new Chart(netEl, {
        type: 'line',
        data: {
          labels: netLabels,
          datasets: [
            {
              label: 'RX (KB/s)',
              data: this.fleetNetRxHistory.map(v => v / 1024),
              borderColor: '#a855f7',
              backgroundColor: 'rgba(168,85,247,0.1)',
              fill: true, tension: 0.3, pointRadius: 1, borderWidth: 2,
            },
            {
              label: 'TX (KB/s)',
              data: this.fleetNetTxHistory.map(v => v / 1024),
              borderColor: '#f97316',
              backgroundColor: 'rgba(249,115,22,0.1)',
              fill: true, tension: 0.3, pointRadius: 1, borderWidth: 2,
            }
          ]
        },
        options: {
          ...chartOptions('KB'),
          plugins: { legend: { display: true, labels: { color: '#94a3b8', boxWidth: 12 } } }
        }
      });
    },

    updateFleetCharts() {
      if (chartState.fleetCpu) {
        const cpuData = [...this.fleetCpuHistory];
        chartState.fleetCpu.data.labels = cpuData.map(() => '');
        chartState.fleetCpu.data.datasets[0].data = cpuData;
        chartState.fleetCpu.update();
      }
      if (chartState.fleetMem) {
        const memMB = this.fleetMemHistory.map(v => v / (1024 * 1024));
        chartState.fleetMem.data.labels = memMB.map(() => '');
        chartState.fleetMem.data.datasets[0].data = memMB;
        chartState.fleetMem.update();
      }
      if (chartState.fleetNet) {
        const rxKB = this.fleetNetRxHistory.map(v => v / 1024);
        const txKB = this.fleetNetTxHistory.map(v => v / 1024);
        const netLabels = Array.from({ length: Math.max(rxKB.length, txKB.length, 1) }, () => '');
        chartState.fleetNet.data.labels = netLabels;
        chartState.fleetNet.data.datasets[0].data = rxKB;
        chartState.fleetNet.data.datasets[1].data = txKB;
        chartState.fleetNet.update();
      }
    },

    async selectSecurityContainer(id, name) {
      this.selectedSecurityContainer = { id, name };
      this.containerSecurity = null;
      try {
        const resp = await fetch(`/api/security/${id}`);
        if (resp.ok) {
          this.containerSecurity = await resp.json();
        }
      } catch (e) {
        console.error('Failed to load container security:', e);
      }
    },

    backToSecurityList() {
      this.selectedSecurityContainer = null;
      this.containerSecurity = null;
    },

    async selectAdvisorContainer(id, name) {
      this.selectedAdvisorContainer = { id, name };
      this.containerRecommendations = null;
      try {
        const resp = await fetch(`/api/recommendations/${id}`);
        if (resp.ok) {
          this.containerRecommendations = await resp.json();
        }
      } catch (e) {
        console.error('Failed to load container recommendations:', e);
      }
    },

    backToAdvisorList() {
      this.selectedAdvisorContainer = null;
      this.containerRecommendations = null;
    },

    async loadSecurity() {
      this.securityLoading = true;
      this.securityError = null;
      try {
        const resp = await fetch('/api/security');
        if (!resp.ok) throw new Error(await resp.text());
        this.securityReport = await resp.json();
        this.$nextTick(() => this.initSecurityChart());
      } catch (e) {
        this.securityError = 'Failed to load security report: ' + e.message;
      }
      this.securityLoading = false;
    },

    initSecurityChart() {
      const el = document.getElementById('securityScoreChart');
      if (!el || !this.securityReport?.containers) return;

      // Destroy previous instance if any
      const existingChart = Chart.getChart(el);
      if (existingChart) existingChart.destroy();

      const containers = this.securityReport.containers;
      const labels = containers.map(c => c.container_name);
      const scores = containers.map(c => c.score);
      const colors = scores.map(s => s >= 80 ? '#22c55e' : s >= 50 ? '#eab308' : '#ef4444');
      const longestLabel = labels.reduce((max, label) => Math.max(max, label.length), 0);
      const yAxisWidth = Math.min(260, Math.max(140, longestLabel * 7 + 24));

      new Chart(el, {
        type: 'bar',
        data: {
          labels,
          datasets: [{
            label: 'Score',
            data: scores,
            backgroundColor: colors,
            borderRadius: 4,
          }]
        },
        options: {
          responsive: true,
          maintainAspectRatio: false,
          animation: false,
          indexAxis: 'y',
          scales: {
            x: {
              beginAtZero: true,
              max: 100,
              title: { display: true, text: 'Score', color: '#94a3b8' },
              ticks: { color: '#94a3b8' },
              grid: { color: 'rgba(148,163,184,0.1)' }
            },
            y: {
              afterFit(scale) {
                scale.width = Math.max(scale.width, yAxisWidth);
              },
              ticks: {
                color: '#94a3b8',
                font: { size: 11 },
                autoSkip: false,
                crossAlign: 'far',
                padding: 6,
              },
              grid: { display: false }
            }
          },
          plugins: { legend: { display: false } }
        }
      });
    },

    async loadAdvisor() {
      this.advisorLoading = true;
      this.advisorError = null;
      try {
        const resp = await fetch('/api/recommendations');
        if (!resp.ok) throw new Error(await resp.text());
        this.advisorReport = await resp.json();
      } catch (e) {
        this.advisorError = 'Failed to load recommendations: ' + e.message;
      }
      this.advisorLoading = false;
    },

    // Formatting helpers
    runtimeLabel() {
      const name = this.snapshot?.host?.runtime_name || '';
      const ver = this.snapshot?.host?.runtime_version || '';
      if (!name) return '';
      // Show runtime names without version clutter
      return name;
    },

    fmtPct(v) {
      if (v == null) return '0.0%';
      return v.toFixed(1) + '%';
    },

    fmtBytes(b) {
      if (b == null || b === 0) return '0 B';
      const units = ['B', 'KB', 'MB', 'GB', 'TB'];
      let i = 0;
      let v = b;
      while (v >= 1024 && i < units.length - 1) { v /= 1024; i++; }
      return v.toFixed(i > 0 ? 1 : 0) + ' ' + units[i];
    },

    fmtNetIO(stats) {
      if (!stats || !stats.network_io) return '0 B / 0 B';
      return this.fmtBytes(stats.network_io.rx_bytes) + ' / ' + this.fmtBytes(stats.network_io.tx_bytes);
    },

    scoreClass(score) {
      if (score >= 80) return 'score-good';
      if (score >= 50) return 'score-warn';
      return 'score-bad';
    }
  };
}

function chartOptions(yLabel, suggestedMin, suggestedMax) {
  return {
    responsive: true,
    maintainAspectRatio: false,
    animation: false,
    scales: {
      x: { display: false },
      y: {
        beginAtZero: true,
        suggestedMin: suggestedMin,
        suggestedMax: suggestedMax,
        title: { display: true, text: yLabel, color: '#94a3b8' },
        ticks: { color: '#94a3b8' },
        grid: { color: 'rgba(148,163,184,0.1)' }
      }
    },
    plugins: {
      legend: { display: false }
    }
  };
}
