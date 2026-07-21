#!/usr/bin/env python3
"""Patch SyncTV's Emby device profile for broad mobile-browser compatibility.

SyncTV currently creates one room-wide Emby URL, so the server cannot know whether
an individual viewer uses desktop Chrome, Android, or iOS Safari.  The conservative
phase-1 policy is therefore:

* Direct-play only MP4/M4V/MOV with H.264 video and AAC/MP3 audio.
* Transcode other video codecs to H.264 + AAC HLS.

This prevents MP4+HEVC files from being returned as original.mp4 to iOS Safari.
"""

from pathlib import Path

TARGET = Path("vendors/vendors/emby/user.go")


def replace_once(text: str, old: str, new: str, description: str) -> str:
    count = text.count(old)
    if count != 1:
        raise RuntimeError(
            f"Expected exactly one match for {description}, found {count}. "
            "The upstream source may have changed; review the patch before building."
        )
    return text.replace(old, new, 1)


def main() -> None:
    text = TARGET.read_text(encoding="utf-8")

    text = replace_once(
        text,
        'VideoCodec: "h264,hevc,vp9,av1",\n\t\t\t\t\tAudioCodec: "aac,mp3,mp2,opus,flac,vorbis",',
        'VideoCodec: "h264",\n\t\t\t\t\tAudioCodec: "aac,mp3",',
        "MP4 direct-play profile",
    )

    # SyncTV may otherwise ask Emby to produce HEVC/VP9 HLS.  Use the most widely
    # supported mobile-browser output instead.
    text = replace_once(
        text,
        'AudioCodec:          "aac,mp2,opus,flac",\n\t\t\t\t\tVideoCodec:          "hevc,h264,vp9",',
        'AudioCodec:          "aac",\n\t\t\t\t\tVideoCodec:          "h264",',
        "video HLS transcoding profile",
    )

    TARGET.write_text(text, encoding="utf-8")
    print(f"Patched {TARGET}")


if __name__ == "__main__":
    main()
