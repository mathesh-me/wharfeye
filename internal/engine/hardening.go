package engine

import "github.com/mathesh-me/wharfeye/internal/models"

// HardeningAdvice provides specific remediation steps for a failed security check.
type HardeningAdvice struct {
	CheckID     string `json:"check_id"`
	CheckName   string `json:"check_name"`
	Severity    string `json:"severity"`
	RunFix      string `json:"run_fix"`
	DockerFix   string `json:"dockerfile_fix,omitempty"`
	Explanation string `json:"explanation"`
	Reference   string `json:"reference,omitempty"`
}

var hardeningMap = map[string]struct {
	RunFix      string
	DockerFix   string
	Explanation string
	Reference   string
}{
	"SEC-001": {
		RunFix:      "Add: --user 1000:1000",
		DockerFix:   "Add to Dockerfile: USER 1000:1000",
		Explanation: "Containers running as root (UID 0) have full host-level privileges if they escape the container. Create a non-root user in your Dockerfile and run as that user.",
		Reference:   "CIS Docker Benchmark v1.6.0 - 4.1 | https://www.cisecurity.org/benchmark/docker",
	},
	"SEC-002": {
		RunFix:      "Remove: --privileged",
		Explanation: "The --privileged flag disables all Linux security boundaries (namespaces, cgroups, seccomp, AppArmor). The container gets full access to host devices and kernel. Use --cap-add for specific capabilities instead.",
		Reference:   "CIS Docker Benchmark v1.6.0 - 5.4 | https://www.cisecurity.org/benchmark/docker",
	},
	"SEC-003": {
		RunFix:      "Replace: --cap-add=ALL with --cap-drop=ALL --cap-add=<needed_cap>",
		Explanation: "Linux capabilities grant fine-grained root powers. Adding ALL gives the container every capability (NET_RAW, SYS_ADMIN, etc). Drop all first, then add back only what your application needs. Common needs: NET_BIND_SERVICE (bind low ports), CHOWN, SETUID/SETGID.",
		Reference:   "CIS Docker Benchmark v1.6.0 - 5.3 | man 7 capabilities",
	},
	"SEC-004": {
		RunFix:      "Remove: --network=host. Use --network=bridge or a custom network",
		Explanation: "Host network mode bypasses Docker's network isolation. The container shares the host's network interfaces, IP, and port space. Use bridge networking with -p to expose only needed ports.",
		Reference:   "CIS Docker Benchmark v1.6.0 - 5.13 | NIST SP 800-190 Section 4.2",
	},
	"SEC-005": {
		RunFix:      "Remove: --pid=host",
		Explanation: "Host PID namespace lets the container see and send signals to all host processes, including other containers. This breaks process isolation. Remove unless you specifically need host-level process debugging.",
		Reference:   "CIS Docker Benchmark v1.6.0 - 5.15",
	},
	"SEC-006": {
		RunFix:      "Add: --read-only --tmpfs /tmp --tmpfs /run",
		DockerFix:   "Use: RUN mkdir -p /tmp && VOLUME /tmp",
		Explanation: "A writable root filesystem allows attackers to modify binaries or install malware inside the container. Make it read-only and mount specific writable paths with tmpfs for temporary files.",
		Reference:   "CIS Docker Benchmark v1.6.0 - 5.12 | NIST SP 800-190 Section 3.3.1",
	},
	"SEC-007": {
		RunFix:      "Add: --health-cmd 'CMD' --health-interval 30s --health-retries 3",
		DockerFix:   "Add: HEALTHCHECK --interval=30s --timeout=5s --retries=3 CMD wget -qO- http://localhost/ || exit 1",
		Explanation: "Health checks allow Docker to monitor container health and automatically restart unhealthy containers. Without them, a crashed process inside a running container goes undetected.",
		Reference:   "CIS Docker Benchmark v1.6.0 - 4.6 | https://docs.docker.com/reference/dockerfile/#healthcheck",
	},
	"SEC-008": {
		RunFix:      "Add: --memory=256m --cpus=1.0 (adjust to your workload)",
		Explanation: "Without resource limits, a single container can consume all host CPU and memory, causing denial of service for other containers and the host itself. Always set memory and CPU limits in production.",
		Reference:   "CIS Docker Benchmark v1.6.0 - 5.10, 5.11 | NIST SP 800-190 Section 3.3.4",
	},
	"SEC-009": {
		RunFix:      "Remove: -v /var/run/docker.sock:/var/run/docker.sock and other host system mounts",
		Explanation: "Mounting the Docker socket gives the container full control over the Docker daemon (can create privileged containers, access host filesystem). Mounting /etc, /proc, or /sys exposes host configuration. Only mount application-specific data directories.",
		Reference:   "CIS Docker Benchmark v1.6.0 - 5.5 | NIST SP 800-190 Section 3.3.2",
	},
	"SEC-010": {
		RunFix:      "Add: --security-opt=no-new-privileges:true --security-opt seccomp=default",
		Explanation: "Security profiles restrict what syscalls the container can make. AppArmor/SELinux provide mandatory access control. Seccomp filters dangerous syscalls. The no-new-privileges flag prevents privilege escalation via setuid binaries.",
		Reference:   "CIS Docker Benchmark v1.6.0 - 5.2, 5.21 | https://docs.docker.com/engine/security/seccomp/",
	},
	"SEC-011": {
		RunFix:      "Bind to localhost: -p 127.0.0.1:8080:8080 instead of -p 8080:8080",
		Explanation: "By default, Docker binds exposed ports to 0.0.0.0 (all interfaces), making the service reachable from any network. Bind to 127.0.0.1 for local-only access, or use a specific interface IP for controlled exposure.",
		Reference:   "CIS Docker Benchmark v1.6.0 - 5.13 | NIST SP 800-190 Section 4.2",
	},
	"SEC-012": {
		RunFix:      "Pin image version: use image:1.2.3@sha256:abc... instead of image:latest",
		DockerFix:   "Pin in Dockerfile: FROM image:1.2.3@sha256:abc...",
		Explanation: "The :latest tag is mutable - it can point to different images over time. This means builds are not reproducible and you may pull an image with known vulnerabilities. Use a specific version tag with a digest for supply chain security.",
		Reference:   "CIS Docker Benchmark v1.6.0 - 4.7 | NIST SP 800-190 Section 3.1.3",
	},
	"SEC-013": {
		RunFix:      "Add: --restart=unless-stopped (or on-failure:5 to limit restart attempts)",
		Explanation: "Without a restart policy, crashed containers stay stopped and must be manually restarted. Use 'unless-stopped' for services that should always run, or 'on-failure:5' to auto-restart with a retry limit.",
		Reference:   "https://docs.docker.com/reference/cli/docker/container/run/#restart",
	},
	"SEC-014": {
		RunFix:      "Replace excessive caps: --cap-drop=ALL --cap-add=<only_needed>",
		Explanation: "Capabilities like SYS_ADMIN (mount filesystems, load kernel modules), NET_ADMIN (modify routing tables), and SYS_PTRACE (debug processes) are high-risk. Drop all capabilities, then add back only what your application requires.",
		Reference:   "CIS Docker Benchmark v1.6.0 - 5.3 | man 7 capabilities",
	},
	"SEC-015": {
		RunFix:      "Add: --user 1000:1000 (or enable daemon-level userns-remap in /etc/docker/daemon.json)",
		DockerFix:   "Add to Dockerfile: RUN adduser -D appuser && USER appuser",
		Explanation: "Without user namespace remapping, UID 0 (root) inside the container maps to UID 0 on the host. If the container escapes, the attacker has host root access. Use --user to run as non-root, or enable userns-remap for UID translation.",
		Reference:   "CIS Docker Benchmark v1.6.0 - 2.8 | https://docs.docker.com/engine/security/userns-remap/",
	},
	"SEC-016": {
		RunFix:      "Remove: --ipc=host",
		Explanation: "Host IPC namespace shares the host's shared memory (/dev/shm), semaphores, and message queues with the container. This can be used to interfere with other processes or extract sensitive data from shared memory.",
		Reference:   "CIS Docker Benchmark v1.6.0 - 5.16",
	},
	"SEC-017": {
		RunFix:      "Add: --pids-limit=200 (adjust based on your application's process count)",
		Explanation: "Without a PID limit, a container can create unlimited processes (fork bomb), exhausting the host's process table and causing system-wide denial of service. Set a limit slightly above your application's normal process count.",
		Reference:   "CIS Docker Benchmark v1.6.0 - 5.28 | NIST SP 800-190 Section 3.3.4",
	},
	"SEC-018": {
		RunFix:      "Use Docker secrets (docker secret create) or --env-file with 0600 permissions instead of -e SECRET=value",
		DockerFix:   "Remove: ENV SECRET=value. Use runtime secrets or mounted secret files",
		Explanation: "Environment variables are visible in docker inspect output, /proc/*/environ inside the container, and often in logs. Use Docker secrets, mounted files, or a secrets manager (HashiCorp Vault, AWS Secrets Manager) for credentials.",
		Reference:   "CIS Docker Benchmark v1.6.0 - 5.19 | https://docs.docker.com/engine/swarm/secrets/",
	},
	"SEC-019": {
		RunFix:      "Remove: --security-opt seccomp=unconfined. Use default or custom profile",
		Explanation: "Seccomp (Secure Computing Mode) filters restrict which syscalls a container can make. The default Docker profile blocks ~44 dangerous syscalls. Disabling it allows the container to use all syscalls, increasing the kernel attack surface.",
		Reference:   "CIS Docker Benchmark v1.6.0 - 5.2 | https://docs.docker.com/engine/security/seccomp/",
	},
	"SEC-020": {
		RunFix:      "Map to high ports: -p 8080:80 instead of -p 80:80",
		Explanation: "Ports below 1024 are privileged on Linux and require root or CAP_NET_BIND_SERVICE. Map container ports to unprivileged host ports (1024+) to avoid requiring elevated permissions.",
		Reference:   "NIST SP 800-190 Section 4.2",
	},
}

// GetContainerHardening returns hardening advice for all failed checks in a report.
func GetContainerHardening(report *models.ContainerSecurityReport) []HardeningAdvice {
	var advice []HardeningAdvice
	for _, check := range report.Checks {
		if check.Passed {
			continue
		}
		h, ok := hardeningMap[check.ID]
		if !ok {
			continue
		}
		advice = append(advice, HardeningAdvice{
			CheckID:     check.ID,
			CheckName:   check.Name,
			Severity:    string(check.Severity),
			RunFix:      h.RunFix,
			DockerFix:   h.DockerFix,
			Explanation: h.Explanation,
			Reference:   h.Reference,
		})
	}
	return advice
}
