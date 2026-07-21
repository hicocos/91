# Audio Library Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Scan supported audio files and provide an audio-only library and playback experience without changing video behavior.

**Architecture:** Reuse the existing catalog row, drive proxy, tags, counters, and deletion model with a persisted `media_type` discriminator. Add explicit media filters at public and worker boundaries, then expose dedicated audio API and React routes/components.

**Tech Stack:** Go 1.24, SQLite, chi, React 18, TypeScript 5.6, Vite 8, Node test runner, Docker Compose.

## Global Constraints

- Supported audio extensions: `.mp3`, `.m4a`, `.aac`, `.flac`, `.wav`, `.ogg`, `.oga`, `.opus`.
- Audio is direct-play only; do not add audio transcoding.
- Existing rows and existing public video behavior default to `video`.
- Audio must not enter home, video list, shorts, video recommendations, thumbnail, teaser, or video-transcode queues.
- Preserve all unrelated dirty-worktree changes; do not commit or push.

---

### Task 1: Persisted media type and scanner configuration

**Files:**
- Modify: `backend/internal/config/config.go`
- Modify: `backend/internal/config/config_test.go`
- Modify: `backend/config.example.yaml`
- Modify: `backend/internal/catalog/schema.sql`
- Modify: `backend/internal/catalog/catalog.go`
- Create: `backend/internal/catalog/media_type_test.go`

**Interfaces:**
- Produces: `config.Scanner.AudioExtensions []string`, `catalog.MediaTypeVideo`, `catalog.MediaTypeAudio`, and `catalog.Video.MediaType`.
- Existing `catalog.UpsertVideo` persists normalized media type and treats empty as video.

- [ ] Add failing config tests asserting omitted `audio_extensions` receives all eight defaults and a custom list is preserved.
- [ ] Run `go test ./internal/config -run 'AudioExtensions' -count=1`; expect failure because `AudioExtensions` does not exist.
- [ ] Add `AudioExtensions []string yaml:"audio_extensions"`, default constants, and example YAML.
- [ ] Re-run the focused config tests; expect PASS.
- [ ] Add failing catalog tests that an empty media type round-trips as `video`, explicit `audio` round-trips as `audio`, and a database created before the column exists migrates successfully.
- [ ] Run `go test ./internal/catalog -run 'MediaType' -count=1`; expect compile or assertion failure.
- [ ] Add `media_type TEXT NOT NULL DEFAULT 'video'`, migration fallback, `Video.MediaType`, insert/update/select/scan support, and normalization limited to `video|audio`.
- [ ] Re-run focused catalog tests; expect PASS.

### Task 2: Scanner classification and video-worker isolation

**Files:**
- Modify: `backend/internal/scanner/scanner.go`
- Modify: `backend/internal/scanner/scanner_test.go`
- Modify: `backend/cmd/server/drives.go`
- Modify: `backend/internal/catalog/catalog.go`
- Modify: `backend/internal/catalog/video_management_test.go`

**Interfaces:**
- Consumes: `config.Scanner.VideoExtensions`, `config.Scanner.AudioExtensions`, and catalog media constants.
- Produces: `scanner.New(..., videoExts, audioExts, ...)`; audio records use `preview_status='skipped'`, `thumbnail_status='skipped'`, and no quality.

- [ ] Add a failing scanner test with one `.mp4`, one `.flac`, and one `.txt`, asserting two records with distinct media types and no callback for audio video-generation work.
- [ ] Run `go test ./internal/scanner -run 'Audio' -count=1`; expect failure because audio is ignored.
- [ ] Split scanner extension maps, classify candidates, persist media type, and invoke `OnNewVideo` only for video.
- [ ] Re-run focused scanner tests; expect PASS.
- [ ] Add failing catalog tests proving thumbnail, preview, and transcode candidate queries exclude audio.
- [ ] Run the focused catalog tests; expect audio to appear incorrectly.
- [ ] Add `media_type='video'` predicates to every video-only generation candidate/count query and pass both extension lists from server scan setup.
- [ ] Re-run focused tests; expect PASS.

### Task 3: Catalog listing separation

**Files:**
- Modify: `backend/internal/catalog/catalog.go`
- Create: `backend/internal/catalog/media_listing_test.go`
- Modify: `backend/internal/api/home_recommendations.go`
- Modify: `backend/internal/api/shorts_feed.go`

**Interfaces:**
- Produces: `catalog.ListParams.MediaType string`; empty defaults to video for public listing compatibility.
- All recommendation and short-feed SQL is video-only.

- [ ] Add failing tests seeding one video and one audio, then assert default `ListVideos` returns only video and `MediaTypeAudio` returns only audio with correct totals/search/sort.
- [ ] Run `go test ./internal/catalog -run 'MediaListing' -count=1`; expect mixed results.
- [ ] Normalize `ListParams.MediaType`, append the media predicate to list and total SQL, and constrain tag/count/recommendation pools where they feed video UI.
- [ ] Re-run focused tests; expect PASS.
- [ ] Add or extend short/home tests proving audio never enters those feeds, run focused API tests, and add explicit media predicates to custom feed SQL until GREEN.

### Task 4: Dedicated audio API and stream MIME fallback

**Files:**
- Create: `backend/internal/api/audio.go`
- Create: `backend/internal/api/audio_test.go`
- Modify: `backend/internal/api/api.go`
- Modify: `backend/internal/proxy/proxy.go`
- Modify: `backend/internal/proxy/proxy_test.go`

**Interfaces:**
- Produces: `AudioDTO`, `AudioDetailDTO`, `GET /api/audios`, `GET /api/audio/{id}`, `POST /api/audio/{id}/view`.
- Uses existing `/p/stream/{videoID}` for `audioSrc`.

- [ ] Add failing API tests for paginated audio listing, audio detail, wrong-media 404 in both directions, audio-only view recording, same-media recommendations, and stream authorization.
- [ ] Run `go test ./internal/api -run 'Audio|WrongMedia' -count=1`; expect missing routes/types.
- [ ] Implement audio handlers/mappers in `audio.go`, register routes, and gate existing video detail/subtitle/view endpoints on video media type.
- [ ] Re-run focused API tests; expect PASS.
- [ ] Add a failing proxy test for preserving Range and supplying an audio MIME fallback only when upstream omitted Content-Type.
- [ ] Run `go test ./internal/proxy -run 'AudioContentType' -count=1`; expect missing fallback.
- [ ] Extend stream serving with an optional fallback content type derived from extension, without overwriting upstream Content-Type, then re-run focused tests.

### Task 5: Audio frontend data contract and navigation

**Files:**
- Modify: `src/types.ts`
- Create: `src/data/audios.ts`
- Modify: `src/components/MainNav.tsx`
- Modify: `src/App.tsx`
- Create: `tests/audioNavigation.test.ts`
- Create: `tests/audioApi.test.ts`

**Interfaces:**
- Produces: `AudioItem`, `AudioDetail`, `fetchAudios`, `fetchAudioDetail`, `recordAudioView`, routes `/audio` and `/audio/:id`.

- [ ] Add source-contract tests asserting the `AudioLines` navigation item, lazy routes, encoded API paths, response validation, and audio type fields.
- [ ] Run `node --import tsx --test tests/audioNavigation.test.ts tests/audioApi.test.ts`; expect failures.
- [ ] Add typed audio contracts/client, navigation, and lazy route declarations.
- [ ] Re-run focused tests; expect PASS.

### Task 6: Audio list and playback page

**Files:**
- Create: `src/pages/AudioLibraryPage.tsx`
- Create: `src/pages/AudioDetailPage.tsx`
- Create: `src/components/AudioTrackList.tsx`
- Create: `src/components/AudioPlayer.tsx`
- Create: `src/styles/audio.css`
- Modify: `src/main.tsx`
- Create: `tests/audioPages.test.ts`
- Create: `tests/audioResponsive.test.ts`

**Interfaces:**
- Consumes: audio data client/types from Task 5.
- Produces: searchable/sortable/paginated track list and dedicated semantic audio detail player.

- [ ] Add failing source-contract tests for `<audio preload="metadata">`, unsupported-format error copy, first-play view recording, search/sort/pagination states, audio-specific empty/error copy, recommendation links, and mobile CSS constraints.
- [ ] Run `node --import tsx --test tests/audioPages.test.ts tests/audioResponsive.test.ts`; expect missing files.
- [ ] Implement a compact track list with stable format marks and metadata columns; reuse existing search, tag, sort, pagination, and empty-state components.
- [ ] Implement a dedicated audio detail page/player with play/pause, seek, time, volume, browser-format error, metadata, tags, and audio recommendations.
- [ ] Add responsive CSS using existing theme tokens and import it from `main.tsx`.
- [ ] Re-run focused frontend tests and `npm run lint`; expect PASS.

### Task 7: Full verification and live deployment

**Files:**
- Inspect only all task-owned files and deployment output.

- [ ] Run `gofmt` on changed Go files.
- [ ] Run `go test ./...` from `backend`; require zero failures.
- [ ] Run `npm test`; require zero failures.
- [ ] Run `npm run lint` and `npm run build`; require exit 0.
- [ ] Run `git diff --check` and inspect scoped status without reverting unrelated changes.
- [ ] Run `docker compose up -d --build`; wait for `video-site-91` to report healthy.
- [ ] Generate a short MP3 fixture with FFmpeg in a mounted local-storage test directory or use an isolated temporary catalog integration when the production mount cannot safely be changed.
- [ ] Trigger/execute scanning, authenticate with an existing test session where available, and verify audio list/detail JSON, `Range: bytes=0-1023` returns 206, and the video list does not include the audio fixture.
- [ ] Remove only the generated fixture if it was added to production storage; keep the deployed feature and report exact commands/results.
