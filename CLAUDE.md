# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

m3u8Grabber is a Go command-line tool that downloads MPEG Transport Stream videos from m3u8 playlist files and converts them to mp4. It supports AES-encrypted playlists and parallel segment downloading for improved performance.

## Build and Development Commands

```bash
# Build the application
go build .

# Run tests
go test ./...

# Run specific test verbosely
go test -v ./m3u8

# Format code
gofmt -w .

# Download a video
./m3u8Grabber --m3u8="<url>" --output="<filename>"

# Run in server mode
./m3u8Grabber --server --server_port=13535
```

## Architecture

### Core Components

1. **main.go**: Entry point handling CLI flags and initialization. Supports both direct download mode and server mode.

2. **m3u8/** package: Core functionality
   - `m3u8.go`: Main M3u8File struct and parsing logic for playlists, including support for multiple renditions, audio streams, and subtitles
   - `downloader.go`: HTTP client with proxy support (HTTP/SOCKS5) and retry logic
   - `worker.go`: Parallel segment downloading using goroutines
   - `converter.go`: FFmpeg integration for TS to MP4 conversion and stream muxing
   - `crypto.go`: AES decryption support for encrypted segments
   - `rendition.go`: Multi-rendition/quality selection logic

3. **server/** package: HTTP server mode for receiving download requests via API

### Key Data Structures

- `M3u8File`: Main structure containing segments, encryption keys, renditions, audio streams, and subtitle streams
- `Rendition`: Different quality/bitrate versions of the same content
- `Audiostream`: External audio tracks with language and channel info
- `SubtitleStream`: WebVTT subtitle tracks

### External Dependencies

- **ffmpeg**: Required for TS to MP4 conversion and stream muxing (must be installed separately)
- **go-astisub**: Subtitle handling
- **go-astits**: MPEG-TS parsing
- **h12.io/socks**: SOCKS5 proxy support

### Processing Flow

1. Parse m3u8 playlist to extract segments, encryption info, and alternate streams
2. Download segments in parallel using worker pool
3. Decrypt segments if encrypted (AES-128 or SAMPLE-AES)
4. Handle multiple audio streams and subtitles if present
5. Concatenate segments and convert to MP4 using ffmpeg

## Important Implementation Details

- Parallel downloading significantly improves performance (10x faster than ffmpeg for large files)
- Supports custom processors via `CustomM3u8Processor` interface for DRM handling
- Global timeout of 12 minutes per download with 3 retries per segment
- Automatic URL resolution for relative segment paths
- Support for both sequential and explicit IV in AES encryption