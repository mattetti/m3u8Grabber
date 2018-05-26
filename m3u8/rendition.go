package m3u8

import (
	"strconv"
	"strings"
)

const (
	altStreamMarker = "#EXT-X-STREAM-INF"
)

// Rendition is an alternative version of a stream.
// Each member of the Group MUST be an alternative rendition of the same content
// See https://tools.ietf.org/html/draft-pantos-http-live-streaming-16#page-21
type Rendition struct {
	ProgramID int

	// Bandwidth of the rendition
	// The value is a decimal-integer of bits per second.  It represents the
	// peak segment bit rate of the Variant Stream.
	//
	// If all the Media Segments in a Variant Stream have already been
	// created, the BANDWIDTH value MUST be the largest sum of peak segment
	// bit rates that is produced by any playable combination of Renditions.
	// (For a Variant Stream with a single Media Playlist, this is just the
	// peak segment bit rate of that Media Playlist.)  An inaccurate value
	// can cause playback stalls or prevent clients from playing the
	// variant.
	//
	// If the Master Playlist is to be made available before all Media
	// Segments in the presentation have been encoded, the BANDWIDTH value
	// SHOULD be the BANDWIDTH value of a representative period of similar
	// content, encoded using the same settings.
	Bandwidth int

	// The value is a decimal-resolution describing the optimal pixel resolution
	// at which to display all the video in the Variant Stream.
	Resolution string

	// The value is a quoted-string containing a comma-separated list of
	// formats, where each format specifies a media sample type that is present
	// in one or more Renditions specified by the Variant Stream. Valid format
	// identifiers are those in the ISO Base Media File Format Name Space
	// defined by The 'Codecs' and 'Profiles' Parameters for "Bucket" Media
	// Types [RFC6381].
	Codecs []string

	// The value can be either a quoted-string or an enumerated-string with the
	// value NONE.  If the value is a quoted-string, it MUST match the value of
	// the GROUP-ID attribute of an EXT-X-MEDIA tag elsewhere in the Playlist
	// whose TYPE attribute is CLOSED-CAPTIONS, and indicates the set of
	// closed-caption Renditions that can be used when playing the presentation.
	ClosedCaptions []string
	URL            string
}

func ExtractRendition(l string) Rendition {
	alt := Rendition{}
	if !strings.HasPrefix(l, altStreamMarker) {
		return alt
	}
	data := l[len(altStreamMarker)+1:]
	idx := -1
	for {
		// find the next end of key
		idx = strings.IndexByte(data, '=')
		if idx < 0 {
			break
		}
		key := data[:idx]
		data = data[idx+1:]
		var value string
		// check if we have a quoted string
		if data[0] == '"' {
			idx = strings.IndexByte(data[1:], '"')
			if idx < 0 {
				break
			}
			idx++ // because we looked up starting at index 1
			value = data[1:idx]

			data = data[idx+1:]
			if len(data) > 1 && data[0] == ',' {
				data = data[1:]
			}
		} else {
			idx = strings.IndexByte(data, ',')
			if idx < 0 {
				idx = len(data)
			}
			value = data[:idx]

			if idx < len(data) {
				data = data[idx+1:]
			}
		}
		switch key {
		case "PROGRAM-ID":
			alt.ProgramID, _ = strconv.Atoi(value)
		case "BANDWIDTH":
			alt.Bandwidth, _ = strconv.Atoi(value)
		case "RESOLUTION":
			alt.Resolution = value
		case "CODECS":
			alt.Codecs = splitAndTrimCommaList(value)
		case "CLOSED-CAPTIONS":
			if value == "NONE" {
				continue
			}
			alt.ClosedCaptions = splitAndTrimCommaList(value)
		}
	}

	return alt
}
