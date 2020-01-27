package m3u8

import (
	"strconv"
	"strings"
)

const (
	altStreamMarker   = "#EXT-X-STREAM-INF"
	subsStreamMarker  = "#EXT-X-MEDIA:TYPE=SUBTITLES"
	audioStreamMarker = "#EXT-X-MEDIA:TYPE=AUDIO"
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
	// Audio - The value is a quoted-string. It MUST match the value of the
	// GROUP-ID attribute of an EXT-X-MEDIA tag elsewhere in the Master Playlist
	// whose TYPE attribute is AUDIO. It indicates the set of audio Renditions
	// that SHOULD be used when playing the presentation. (optional)
	Audio string
	// Video - The value is a quoted-string.  It MUST match the value of the
	// GROUP-ID attribute of an EXT-X-MEDIA tag elsewhere in the Master Playlist
	// whose TYPE attribute is VIDEO.  It indicates the set of video Renditions
	// that SHOULD be used when playing the presentation. (optional)
	Video string

	// FrameRate - The value is a decimal-floating-point describing the maximum
	// frame rate for all the video in the Variant Stream, rounded to 3 decimal
	// places. The FRAME-RATE attribute is OPTIONAL but is recommended if the
	// Variant Stream includes video.  The FRAME-RATE attribute SHOULD be
	// included if any video in a Variant Stream exceeds 30 frames per second.
	FrameRate float64
}

func ExtractRendition(l string) Rendition {
	alt := Rendition{}
	if !strings.HasPrefix(l, altStreamMarker) {
		return alt
	}
	for k, v := range decodeEqParamLine(l[len(altStreamMarker):]) {
		switch k {
		case "AUDIO":
			alt.Audio = v
		case "VIDEO":
			alt.Video = v
		case "URI":
			alt.URL = v
		case "PROGRAM-ID":
			alt.ProgramID, _ = strconv.Atoi(v)
		case "BANDWIDTH":
			alt.Bandwidth, _ = strconv.Atoi(v)
		case "RESOLUTION":
			alt.Resolution = v
		case "CODECS":
			alt.Codecs = splitAndTrimCommaList(v)
		case "FRAME-RATE":
			alt.FrameRate, _ = strconv.ParseFloat(v, 32)
		case "CLOSED-CAPTIONS":
			if v == "NONE" {
				continue
			}
			alt.ClosedCaptions = splitAndTrimCommaList(v)
		}
	}

	return alt
}
