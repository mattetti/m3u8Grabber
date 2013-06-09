# m3u8Grabber

This command line tool is designed to download a MPEG video Transport Stream
defined in a m3u8 file into a mkv file.

## Requirements

OS: unix based
Libraries: ffmpeg needs to be installed and available.

## Usage

$ m3u8Grabber --m3u8="http://someCompatibleM3U8.url" --output="coolStuff"

## Status

This is a very alpha piece of code, it was only tested against 1 m3u8
format and probably won't work for you. You've been warned.
