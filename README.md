# Sleepcast

Self-hosted web app that plays audio extracted from any YouTube URL.
Paste a URL, hit play — audio-only stream is downloaded and served via an
HTML5 player with skip, sleep timer, and auto-cleanup.

No auth, no Google Cloud project, no sign-in. Just a URL.

## Run

```sh
go run .
```

Open <http://localhost:5005>, paste a YouTube URL, hit **Play**.

## Features

- Accepts any YouTube URL shape: `watch?v=`, `youtu.be/`, `/shorts/`, `/embed/`,
  `/live/`, or a raw 11-char video ID.
- Lowest-bitrate m4a (~48 kbps AAC) → small files, instant playback.
- Player: play/pause, ±15s, ±30s, scrubber.
- Sleep timer: off / 10 / 20 / 30 / 45 / 60 min.
- `navigator.mediaSession` → lock-screen controls on mobile.
- Auto-clean: file deleted when playback ends, plus a 7-day TTL sweep.
- Cached: playing the same URL twice skips the download.

## Config (all optional)

| Env var | Default |
|---|---|
| `SLEEPCAST_ADDR` | `:5005` |
| `SLEEPCAST_DATA_DIR` | `./data` |
| `SLEEPCAST_TTL_HOURS` | `168` (7 days) |

## Layout

```
internal/
  config/   env-var config
  storage/  SQLite + media dir helpers
  youtube/  URL parsing, audio download (kkdai/youtube/v2), job tracker
  cleanup/  on-finish purge, TTL sweep, startup reconciliation
  server/   HTTP handlers, routing
web/        index.html, app.js, style.css
```

## Caveats

- `kkdai/youtube/v2` occasionally breaks when YouTube rotates its player JS
  cipher. If downloads start failing, `go get -u github.com/kkdai/youtube/v2`.
- Age-restricted videos cannot be fetched anonymously.
