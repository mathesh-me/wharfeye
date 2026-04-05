# Security Checks

20 runtime misconfiguration checks based on CIS Docker Benchmark v1.6.0 and NIST SP 800-190.

| ID | Check | Severity | What Triggers It |
|----|-------|----------|-----------------|
| SEC-001 | Running as root | HIGH | No `--user` flag or user is root/0 |
| SEC-002 | Privileged mode | CRITICAL | `--privileged` flag |
| SEC-003 | All capabilities | CRITICAL | `--cap-add=ALL` |
| SEC-004 | Host network | HIGH | `--network=host` |
| SEC-005 | Host PID namespace | HIGH | `--pid=host` |
| SEC-006 | Writable root filesystem | MEDIUM | No `--read-only` flag |
| SEC-007 | No health check | LOW | No `HEALTHCHECK` or `--health-cmd` |
| SEC-008 | No resource limits | MEDIUM | No `--memory` or `--cpus`/`--cpu-quota` |
| SEC-009 | Sensitive mount | HIGH | Docker socket or `/etc` mounted |
| SEC-010 | No security options | MEDIUM | No AppArmor/Seccomp profile |
| SEC-011 | Exposed ports on 0.0.0.0 | MEDIUM | `-p 0.0.0.0:port:port` |
| SEC-012 | Using :latest tag | LOW | Image tag is `:latest` or untagged |
| SEC-013 | No restart policy | LOW | No `--restart` flag |
| SEC-014 | Excessive capabilities | MEDIUM | Specific dangerous caps (SYS_ADMIN, NET_RAW, etc.) |
| SEC-015 | No user namespace remap | LOW | Running as root without userns |
| SEC-016 | Host IPC namespace | HIGH | `--ipc=host` |
| SEC-017 | No PID limit | MEDIUM | No `--pids-limit` set |
| SEC-018 | Secrets in environment | HIGH | Env vars containing PASSWORD, SECRET, TOKEN, etc. |
| SEC-019 | Seccomp disabled | HIGH | `--security-opt seccomp=unconfined` |
| SEC-020 | Privileged port mapping | LOW | Host ports below 1024 mapped |

## References

- CIS Docker Benchmark v1.6.0: https://www.cisecurity.org/benchmark/docker
- NIST SP 800-190 (Container Security): https://csrc.nist.gov/pubs/sp/800/190/final
- Docker Security Best Practices: https://docs.docker.com/engine/security/
