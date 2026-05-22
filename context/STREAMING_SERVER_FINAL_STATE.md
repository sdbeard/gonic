# Streaming Server Final State

This document captures the long-term target shape for the simplified API-only
streaming server. It is intentionally product-oriented: the goal is to show the
component boundaries clearly enough to decompose the work into a roadmap.

The target server is explicitly for playing music, playlist-driven playback, and
future live/continuous broadcast. It removes the browser admin UI, Subsonic-first
controller surface, podcasts, jukebox, Last.fm, ListenBrainz, and scrobbling.

## Final-State Overview

Open the SVG image directly for the high-level component map:

![Streaming Server Final State](./streaming-server-final-state.svg)

## Top-Level Components

```mermaid
flowchart LR
    clients[API Clients\nplayers, automation, operators]
    listeners[Broadcast Listeners\nHTTP/HLS/future clients]

    api[Native API Server\nJSON control plane]
    middleware[Middleware\nCORS, gzip, auth, logging]

    catalog[Catalog Service\nartists, albums, tracks, search]
    scanner[Scanner Pipeline\nwalk files, read tags, update catalog]
    playlists[Playlist Service\nread, write, order tracks]
    streaming[Streaming Service\nraw file and transcoded playback]
    broadcast[Broadcast Service\ncontinuous playlist/live output]
    transcode[Transcode Service\nffmpeg profiles and cache]

    state[(JSONFile App State\nsettings, users, playlists, queues)]
    index[(Search/Catalog Index\nJSON first, Elasticsearch optional)]
    files[(Filesystem\nmusic, covers, cache)]

    clients --> api
    listeners --> broadcast
    api --> middleware
    middleware --> catalog
    middleware --> scanner
    middleware --> playlists
    middleware --> streaming
    middleware --> broadcast

    scanner --> files
    scanner --> catalog
    catalog --> index
    playlists --> state
    playlists --> catalog
    streaming --> catalog
    streaming --> files
    streaming --> transcode
    transcode --> files
    broadcast --> playlists
    broadcast --> streaming
    broadcast --> transcode
```

## API Surface

```mermaid
flowchart TB
    api[Native API Server]

    health[Health\nGET /api/health]
    scan[Scan\nPOST /api/scan\nGET /api/scan/status]
    library[Library\nGET /api/artists\nGET /api/albums\nGET /api/tracks\nGET /api/search]
    playlist[Playlists\nGET/POST /api/playlists\nGET/PUT/DELETE /api/playlists/:id]
    stream[Streaming\nGET /api/stream/:trackID]
    broadcast[Broadcast\nGET/POST /api/broadcasts\nPOST /api/broadcasts/:id/start\nGET /api/broadcasts/:id/stream]

    api --> health
    api --> scan
    api --> library
    api --> playlist
    api --> stream
    api --> broadcast
```

## Playback Flow

```mermaid
sequenceDiagram
    participant Client
    participant API
    participant Catalog
    participant Streaming
    participant Transcode
    participant Filesystem

    Client->>API: GET /api/stream/tr-123?profile=mp3
    API->>Streaming: stream track tr-123
    Streaming->>Catalog: locate track metadata and path
    Catalog-->>Streaming: file path, bitrate, mime, duration
    Streaming->>Transcode: apply profile when requested
    Transcode->>Filesystem: read source file, write/read cache
    Transcode-->>Streaming: audio bytes
    Streaming-->>Client: audio response
```

## Broadcast Flow

```mermaid
sequenceDiagram
    participant Operator
    participant API
    participant Broadcast
    participant Playlist
    participant Streaming
    participant Listener

    Operator->>API: POST /api/broadcasts/:id/start
    API->>Broadcast: start broadcast
    Broadcast->>Playlist: load queue
    loop continuous playback
        Broadcast->>Streaming: stream current track
        Streaming-->>Broadcast: encoded audio
        Listener->>Broadcast: GET /api/broadcasts/:id/stream
        Broadcast-->>Listener: continuous audio output
        Broadcast->>Playlist: advance queue
    end
```

## Storage Boundaries

```mermaid
flowchart LR
    app[Application Components]

    settings[Settings Store\nJSONFile]
    users[Auth/User Store\nJSONFile or token config]
    playlists[Playlist Store\nM3U and JSON metadata]
    catalog[Catalog Store\ntracks/albums/artists]
    search[Search Index\noptional Elasticsearch]
    cache[Transcode Cache\nfilesystem]
    music[Music Files\nfilesystem]

    app --> settings
    app --> users
    app --> playlists
    app --> catalog
    catalog --> search
    app --> cache
    app --> music
```

## Removed From Final State

```mermaid
flowchart TB
    removed[Removed Concerns]
    admin[Browser Admin UI]
    subsonic[Subsonic-first API Surface]
    podcasts[Podcasts]
    jukebox[Jukebox/mpv Control]
    lastfm[Last.fm]
    listenbrainz[ListenBrainz]
    scrobble[Scrobbling and play stats]
    external[External artist/album info cache]

    removed --> admin
    removed --> subsonic
    removed --> podcasts
    removed --> jukebox
    removed --> lastfm
    removed --> listenbrainz
    removed --> scrobble
    removed --> external
```

## Intended Package Direction

```text
cmd/gonic/
  main.go

api/
  openapi.yaml                 # eventual API contract

internal/
  pipeline/
  config/
  server/api/
  catalog/
  scanner/
  playlists/
  streaming/
  broadcast/
  transcode/
  storage/
    jsonfile/
    catalogindex/
  filesystem/
```

