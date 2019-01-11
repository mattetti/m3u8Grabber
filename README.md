# m3u8Grabber

This command line tool is designed to download a MPEG video Transport Stream
defined in a m3u8 file into an mp4 file. Full hls protocol isn't implemented.

## Requirements

OS: unix, windows
Go: https://golang.org
Libraries: ffmpeg needs to be installed and available (for conversion).

## Usage

$ go build .
$ m3u8Grabber --m3u8="http://someCompatibleM3U8.url" --output="coolStuff"

The grabber can also be run as a server and receives downloads via HTTP requests (undocumented).

## Status

This was developed for very specific use cases and isn't well tested outside of those sources.
AES encrypted playlists with sequential IVs and a global key are supported.

Using this tool to download playlists is much faster for my use case than using ffmpeg directly (10x speed difference on a 200MB file). This is probably due to the fact that this code uses goroutines to process segments in parallel and does way less than ffmpeg in general.

Using ffmpeg to convert ts to mp4 is a bit sad and this dependency could be dropped in the future (PRs welcome).
