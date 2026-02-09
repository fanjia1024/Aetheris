# Aetheris v1.0 Open Source Release Checklist

This checklist ensures that Aetheris v1.0 is fully ready for open source release, covering documentation, licensing, testing, examples, CI/CD, and community governance.

---

## 1. Core Functionality Verification
| Check Item | Goal | Verification Method | Status |
|------------|------|------------------|--------|
| Job Creation | Jobs can be created via Agent API or CLI | Run example jobs | ☐ |
| Job Execution | Jobs execute from start to finish successfully | Execute sample DAG/TaskGraph | ☐ |
| Event Stream Logging | All actions (planning, tool calls, steps, retries, failures, recovery) are recorded | Inspect JobStore events | ☐ |
| Job Replay | Job execution can be deterministically replayed | Use replay functionality | ☐ |
| Idempotency | Re-running a Job does not create side effects | Re-run same Job test | ☐ |
| Crash Recovery | Jobs can resume after interruption | Kill and restart Runner/Worker | ☐ |

---

## 2. Distributed Execution & Worker System
| Check Item | Goal | Verification Method | Status |
|------------|------|------------------|--------|
| Multi-Worker Support | Tasks execute across multiple workers | Deploy 2+ workers and run a Job | ☐ |
| Scheduler Retry | Failed tasks are automatically retried | Force a task failure | ☐ |
| Runner Checkpoint | Step-level checkpointing works | Stop Runner mid-task and resume | ☐ |

---

## 3. CLI & Agent API Functionality
| Check Item | Goal | Verification Method | Status |
|------------|------|------------------|--------|
| CLI Commands | `corag` subcommands (`agent create`, `jobs`, `trace`, `replay`, `cancel`, `workers`) work correctly | Run CLI examples | ☐ |
| Agent API | REST/gRPC endpoints operate correctly | Test API with Postman or scripts | ☐ |
| Job Cancellation | Jobs can be cancelled mid-execution | Test cancel API | ☐ |
| Event Query | Retrieve events by Job | API listEvents | ☐ |

---

## 4. Logging & Monitoring
| Check Item | Goal | Verification Method | Status |
|------------|------|------------------|--------|
| Trace UI | Shows task execution, DAG, TaskGraph | Open UI and view example Job | ☐ |
| Logs Completeness | Logs include execution, errors, retries | Inspect system logs | ☐ |
| Auditability | Each agent decision is traceable | Randomly check Job history | ☐ |

---

## 5. Documentation & Open Source Support
| Check Item | Goal | Verification Method | Status |
|------------|------|------------------|--------|
| README | Project description, architecture, examples, and license | Review README | ☐ |
| LICENSE | Apache 2.0 license included | File exists and correct | ☐ |
| CONTRIBUTING.md | Clear contribution guidelines | File exists | ☐ |
| CODE_OF_CONDUCT.md | Community behavior guidelines | File exists | ☐ |
| CHANGELOG.md | Records v1.0 release | File exists | ☐ |
| AGENTS.md | Agent usage examples | File exists | ☐ |

---

## 6. Examples & Testing
| Check Item | Goal | Verification Method | Status |
|------------|------|------------------|--------|
| Example Jobs | Examples run successfully | Execute jobs in `examples` directory | ☐ |
| Unit Tests | Core modules are covered | Run `go test ./...` | ☐ |
| Integration Tests | Multi-worker, retries, crash recovery tested | Implement and run integration tests | ☐ |

---

## 7. CI/CD & Security
| Check Item | Goal | Verification Method | Status |
|------------|------|------------------|--------|
| GitHub Actions | Build, test, and release workflows run | Check Actions logs | ☐ |
| SLSA3 / Provenance | Provenance generation available | Configure SLSA3 workflow | ☐ |
| Dependency Security | Go modules checked for vulnerabilities | Run `govulncheck` | ☐ |

---

## 8. Community & Release
| Check Item | Goal | Verification Method | Status |
|------------|------|------------------|--------|
| GitHub Release | v1.0 tag exists | Verify GitHub Releases | ☐ |
| Issue / PR Templates | Contributors know how to submit | Template files exist | ☐ |
| Discussions / Community Interaction | GitHub Discussions open | Check Discussions tab | ☐ |

---

> ✅ Mark each item as complete after verification.
> This checklist ensures Aetheris v1.0 is fully ready for production-grade open source release.

