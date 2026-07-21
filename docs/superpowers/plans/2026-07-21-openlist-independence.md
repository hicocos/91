# OpenList Independence Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove OpenList private runtime/API and OpenListTeam module dependencies from 91 while retaining vendor-neutral WebDAV support.

**Architecture:** OneDrive refreshes only against Microsoft OAuth. The Apache-2.0 WoPan client currently vendored as an external module moves behind an internal package boundary without behavior changes. A source contract test prevents the dependency from returning.

**Tech Stack:** Go 1.23, Node 20/TypeScript, Docker Compose.

## Global Constraints

- Preserve all pre-existing dirty-worktree changes.
- Do not copy AGPL-3.0 OpenList main-repository implementation.
- Keep standard WebDAV support vendor-neutral.
- Do not commit or push.
- Rebuild and recreate the live Docker Compose service, then verify `127.0.0.1:9191`.

---

### Task 1: OneDrive Microsoft-only refresh

**Files:**
- Modify: `backend/internal/drives/onedrive/driver_test.go`
- Modify: `backend/internal/drives/onedrive/driver.go`
- Modify: `backend/cmd/server/drives.go`
- Modify: `backend/internal/api/admin_storage.go`

**Interfaces:**
- `onedrive.Config` keeps `ClientID`, `ClientSecret`, `TenantID`, `OAuthURL`, and `APIBaseURL`; it removes `RenewAPIURL`.
- `Driver.refresh(context.Context) error` posts `grant_type=refresh_token` directly to Microsoft-compatible OAuth.

- [ ] Replace the legacy renew tests with tests asserting POST form data, optional secret, token persistence, missing-client rejection, and one replay after `InvalidAuthenticationToken`.
- [ ] Run `go test ./internal/drives/onedrive ./cmd/server ./internal/api` in the Go builder container and confirm RED because legacy fields/behavior still exist or Microsoft-only behavior is incomplete.
- [ ] Remove the legacy constant, fields, query protocol, server wiring, and stale credential retention; keep Microsoft direct refresh.
- [ ] Rerun the focused command and confirm GREEN.

### Task 2: Internalize the Apache-2.0 WoPan client

**Files:**
- Create: `backend/internal/drives/wopan/internal/client/*.go`
- Create: `backend/internal/drives/wopan/internal/client/LICENSE`
- Create: `backend/internal/drives/wopan/internal/client/README.md`
- Modify: `backend/internal/drives/wopan/driver.go`
- Modify: `backend/internal/drives/wopan/driver_test.go`
- Modify: `backend/go.mod`
- Modify: `backend/go.sum`
- Modify: `backend/vendor/modules.txt`
- Delete: `backend/vendor/github.com/OpenListTeam/wopan-sdk-go/**`

**Interfaces:**
- Internal package import path: `github.com/video-site/backend/internal/drives/wopan/internal/client`.
- Package name remains `wopan` to minimize adapter churn.

- [ ] Add `tests/openlistIndependence.test.ts` asserting no OpenList private API, OpenListTeam Go module/import, or vendor directory remains, while WebDAV stays supported.
- [ ] Run the focused Node contract and confirm RED against current sources.
- [ ] Copy the already-vendored Apache-2.0 client into the internal package, retain license/source notice, switch imports, and remove module/vendor declarations.
- [ ] Run WoPan package tests and the independence contract; confirm GREEN.

### Task 3: Vendor-neutral documentation and UI copy

**Files:**
- Modify: `README.md`
- Modify: `backend/README.md`
- Modify: `backend/config.example.yaml`
- Modify: `src/admin/drive/constants.ts`
- Modify: affected frontend source-contract tests.

**Interfaces:**
- WebDAV fields remain `base_url`, `username`, and `password`.
- OneDrive setup requires the project owner's Microsoft Entra OAuth client.

- [ ] Extend the independence test to reject OpenList-specific OneDrive/WebDAV setup copy while allowing historical attribution comments that are not dependencies.
- [ ] Run it RED.
- [ ] Replace setup copy and examples with Microsoft/vendor-neutral WebDAV wording.
- [ ] Run the contract GREEN.

### Task 4: Full verification and deployment

**Files:**
- No new production interfaces.

- [ ] Run backend `go test ./...` and `go vet ./...` in the Docker Go toolchain.
- [ ] Run `npm run lint`, `npm test`, and `npm run build`.
- [ ] Run OpenList dependency scans and `git diff --check` on task-owned files.
- [ ] Build `hicocos-91:local`, recreate the Compose service, wait for healthy status.
- [ ] Verify `/healthz`, homepage HTTP 200, container logs, image dependency scan, and SQLite integrity.
- [ ] Report exact evidence and list task-owned files; explicitly state no commit/push.
