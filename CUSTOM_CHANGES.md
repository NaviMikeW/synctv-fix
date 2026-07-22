# Custom changes

## Phase 1: mobile Emby playback compatibility

- MP4/M4V direct play is limited to H.264 with AAC/MP3.
- MOV direct play is limited to H.264 with AAC/MP3.
- WebM video direct play is disabled for the shared room profile.
- H.264 High 10 (Hi10P) is excluded from direct play.
- Other or uncertain video codecs request H.264 + AAC HLS from Emby.
- Sync, chat, Bilibili and other existing features remain intact.

## Phase 1.2: Emby dashboard playback sessions

- Active SyncTV rooms report playback start, progress, pause, resume, seek, rate changes and stop events to Emby.
- Emby shows one playback session per SyncTV room, using the Emby account bound by the movie creator.
- Progress is recalibrated every 10 seconds and immediately after player interactions.
- Sessions stop when the room has no viewers, the current item changes, or the room leaves the server cache.
- Emby credentials remain on the SyncTV backend and are never sent to the browser for session reporting.

## Build metadata

- Docker builds inject `VERSION=v0.9.15`, preventing SyncTV's implicit `dev` defaults.
- The custom build identity is recorded in the Git commit and OCI image label instead of using a `-fix` program-version suffix that SyncTV would treat as an older prerelease.

## Planned phase 2

- Per-user quality, audio and subtitle selection.
- Per-user Emby playback sessions when viewers use different sources or tracks.
