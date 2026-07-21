# Audio Library Design

## Goal

Extend the existing media library so mounted drives scan and expose audio files through an audio-only list and playback page, without allowing audio records into video home, listing, short-feed, recommendation, preview, thumbnail, or video-transcode flows.

## Supported formats

The default scanner accepts `.mp3`, `.m4a`, `.aac`, `.flac`, `.wav`, `.ogg`, `.oga`, and `.opus` in addition to the existing video formats. Audio is served directly through the existing authenticated Range-capable stream proxy. No audio transcoding or duplicate playback copy is created in this release; unsupported browser codecs produce a clear playback error.

## Data model

Keep the existing `videos` table as the shared media table to preserve drive ownership, stable IDs, deduplication, tags, views, likes, deletion tombstones, and stream authorization. Add `media_type TEXT NOT NULL DEFAULT 'video'` and expose `MediaType` on `catalog.Video`. Existing rows migrate to `video`.

Scanner extension classification is authoritative: configured video extensions produce `video`, configured audio extensions produce `audio`. Audio rows set `quality` empty and video-only generation states to `skipped` so they cannot enter thumbnail, teaser, or transcode queues.

`ListParams.MediaType` controls catalog listing. Public video callers explicitly use `video`; the new audio API uses `audio`. Video detail, subtitle, short feed, home recommendations, and related-video pools reject or exclude audio. Audio detail and related-audio pools reject or exclude video.

## API

Authenticated routes:

- `GET /api/audios`: paginated audio list with `q`, `tag`, `sort`, `page`, and `size`.
- `GET /api/audio/{id}`: audio-only detail response.
- `POST /api/audio/{id}/view`: audio-only view counter.
- Existing `/p/stream/{videoID}` remains the authorized byte source for both media types because it resolves a catalog ID before touching a drive.

Audio DTOs expose ID, `/audio/{id}` href, title, author, duration, size, extension/format, source label, views, publish date, tags, and `audioSrc`. Video DTOs and routes remain backward compatible.

## Frontend

Add an `AudioLines` navigation item and routes `/audio` and `/audio/:id`.

The audio list is a compact, responsive track table rather than a video card grid. Each row has a stable square format mark, title and author, format, duration, size, view count, and a play affordance. Search, sort, tags, pagination, loading, empty, and retry states match existing application conventions while all copy says audio rather than video.

The audio detail page uses a dedicated `<audio>` element and custom surrounding surface. It displays the title, author, source/format metadata, tags, views, and same-media recommendations. Playback errors distinguish unsupported browser format from a generic load failure. It does not render a fake video viewport, subtitles, or video poster/preview UI.

## Error handling and compatibility

- Wrong-media detail routes return 404.
- Hidden or detached-drive media stays inaccessible.
- Stream responses preserve upstream Range behavior and MIME metadata; the handler adds a media-type fallback MIME only when an upstream/local response omitted it.
- An audio decode failure leaves the library record intact and tells the user the current browser may not support that format.
- Existing custom `video_extensions` remain honored; audio extensions are configured separately as `audio_extensions` and receive defaults when omitted.

## Verification

Use Go TDD for config defaults, schema migration, scanner classification, worker exclusions, catalog filtering, API route isolation, and audio stream authorization. Use Node source-contract tests plus TypeScript build for frontend navigation, data client, list, detail player, responsive CSS, and copy. Finish with all backend tests, all frontend tests, production build, Docker Compose rebuild, health check, and an authenticated end-to-end check using a generated audio fixture on a local mounted drive where feasible.
