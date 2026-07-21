# Storage Provider Onboarding Refactor Implementation Plan

> **For agentic workers:** Implement continuously with strict RED-GREEN-REFACTOR. Do not commit, push, revert, format, or overwrite unrelated pre-existing changes.

**Goal:** Replace project one's fragile cloud-drive onboarding path with a registry-backed, probe-before-save workflow and complete OneDrive, Google Drive, WebDAV, and S3-compatible integrations while preserving the multi-mounted-drive video product model.

**Architecture:** Keep the existing `drives.Drive` data-plane contract used by scanning, playback, preview, transcoding, and crawler migration. Add a provider control-plane registry and manifests, secure OAuth flow storage, connection probing, atomic save/attach orchestration, and an S3 driver. Existing non-target providers remain compatible through legacy registration.

**Tech Stack:** Go 1.23, SQLite, chi, React/TypeScript/Vite, Docker Compose.

## Global Constraints

- Work in `/www/91` in place because the repository contains user-owned uncommitted work; preserve every unrelated change.
- Do not commit or push.
- OneDrive, Google Drive, WebDAV, and S3 are the full new-framework providers.
- Existing providers and persisted drives remain compatible.
- New/edit configuration is persisted only after successful connection probing; failed edit keeps the old runtime instance and database configuration.
- OAuth uses hashed, session-bound, provider-bound, encrypted, expiring, one-time state.
- OAuth is primary for OneDrive/Google Drive; manual refresh token remains available.
- OneDrive supports personal and SharePoint drive contexts without requiring the third-party OpenList renew endpoint.
- Google supports My Drive and Shared Drive.
- S3 supports AWS, MinIO, and R2-style endpoints, optional endpoint/session token, path style, bucket, region, and root prefix.
- Private and insecure endpoints are disabled by default and enabled only by explicit server environment variables.
- Sensitive credentials are never returned in plaintext by the new edit API.
- Preserve project one's simultaneous multi-drive scan/playback model; do not introduce a unique active account.
- Build, deploy with Docker Compose, and verify the live service on `127.0.0.1:9191`.

---

### Task 1: Provider registry and manifest

**Files:** Create focused files under `backend/internal/storageproviders/`; modify server composition and admin routes; add Go tests.

- [ ] Write failing registry/manifest tests proving all visible storage kinds have one descriptor and the four target providers expose correct fields, auth methods, root modes, and upload capability.
- [ ] Implement descriptor, registry, manifest JSON, legacy descriptors, and `GET /admin/api/storage/providers`.
- [ ] Replace target-provider support/metadata switches with registry lookups while preserving legacy behavior.
- [ ] Run focused and full backend tests.

### Task 2: Probe-before-save lifecycle and credential redaction

**Files:** Provider orchestration/API/catalog tests and frontend API/types.

- [ ] Write failing tests for create success, create probe failure with no row, edit probe failure preserving old row/runtime, and sensitive-field redaction/merge.
- [ ] Implement normalized account input, temporary driver build/probe, short-lived session/provider/config-bound one-time probe tokens, then transactional save/attach.
- [ ] Make legacy `/admin/api/drives` follow probe-before-persist semantics or keep it as a safe compatibility wrapper.
- [ ] Add field-level token update persistence so refresh callbacks cannot overwrite unrelated drive fields.
- [ ] Run focused and full backend tests.

### Task 3: Secure OAuth onboarding

**Files:** Catalog schema/repository, OAuth service, OneDrive/Google auth routes, server wiring, frontend popup helper and tests.

- [ ] Write failing OAuth flow tests for hash-at-rest, session/provider binding, TTL, concurrent flows, one-time consumption, and encrypted pending config.
- [ ] Implement OneDrive and Google start/callback endpoints with fixed service-derived callback URLs and nonce-bound popup success pages.
- [ ] Implement frontend OAuth popup validation for exact origin, popup source, provider, and nonce.
- [ ] Preserve manual refresh-token onboarding.
- [ ] Run focused backend/frontend tests.

### Task 4: OneDrive and Google Drive correctness

**Files:** Existing provider drivers/descriptors/tests.

- [ ] Write failing tests for direct Microsoft token refresh, SharePoint site/drive base selection, Shared Drive list/upload/delete parameters, and shortcut identity/delete behavior.
- [ ] Remove the default third-party OneDrive renew dependency for new configurations and use configured OAuth client/token endpoint.
- [ ] Add stable SharePoint drive ID context to all OneDrive operations.
- [ ] Persist and apply Google Shared Drive ID consistently.
- [ ] Keep shortcut ID separate from target playback ID.
- [ ] Replace stale-snapshot token updates with field-level updates.
- [ ] Run complete OneDrive/Google tests and backend suite.

### Task 5: WebDAV hardening

**Files:** WebDAV driver/descriptor/network policy/tests.

- [ ] Write failing tests for public/private/insecure endpoint policy, redirect revalidation, anonymous auth, and read probe behavior.
- [ ] Implement endpoint policy with explicit `ALLOW_PRIVATE_STORAGE_ENDPOINTS` and `ALLOW_INSECURE_STORAGE_ENDPOINTS` escapes.
- [ ] Implement read-only onboarding probe and transfer timeout behavior without leaking credentials across origins; retain the project's existing WebDAV upload path unchanged.
- [ ] Run focused and backend tests.

### Task 6: S3-compatible provider

**Files:** `backend/internal/drives/s3/`, descriptor, dependencies/vendor, proxy/crawler integration, tests.

- [ ] Write failing adapter tests using an injectable HTTP endpoint for ListObjectsV2 pagination/delimiter, HeadObject, presigned GET, DeleteObject, prefix normalization, root-boundary failures, and path-style/virtual-host behavior.
- [ ] Implement S3 driver and provider descriptor with endpoint, region, bucket, AK/SK, optional session token, force path style, and root prefix.
- [ ] Ensure object key is fileID and ETag is not treated as reliable MD5.
- [ ] Add S3 to runtime registry, playback, deletion, and directory browsing; explicitly exclude it from crawler upload targets.
- [ ] Vendor dependencies and run focused/full backend tests and vet.

### Task 7: Manifest-driven admin UI

**Files:** `src/admin/drive/*`, `src/admin/DrivesPage.tsx`, `src/admin/api.ts`, crawler target UI, styles/icons, frontend tests.

- [ ] Write failing behavioral/source contract tests for provider manifest loading, S3 fields, OAuth/manual modes, test-and-save behavior, sensitive configured placeholders, error retention, and S3 exclusion from upload targets.
- [ ] Refactor DriveForm to use provider manifests for the four target providers while retaining custom QR flows for legacy providers.
- [ ] Add OneDrive/Google OAuth popup onboarding and manual advanced fallback.
- [ ] Add S3 and improved WebDAV forms with clear validation/errors.
- [ ] Replace hardcoded crawler upload whitelist with backend capability/account readiness data.
- [ ] Run frontend lint, tests, and build.

### Task 8: Integration, compatibility, deployment, and verification

- [ ] Add migration/compatibility tests for existing encrypted OneDrive/Google/WebDAV rows.
- [ ] Run `go test ./...`, `go vet ./...`, `npm run lint`, `npm test`, and `npm run build`.
- [ ] Independently review the task-owned diff for security and logic defects; fix blocking findings and rerun covering tests.
- [ ] Back up `/www/91/data` before deployment.
- [ ] Run `docker compose build` and recreate `video-site-91`.
- [ ] Verify compose health, container logs, `http://127.0.0.1:9191/healthz`, authenticated-route behavior, provider manifest response where feasible, and deployed frontend assets containing S3/OAuth onboarding strings.
- [ ] Confirm no commit or push occurred and report pre-existing unrelated changes separately.
