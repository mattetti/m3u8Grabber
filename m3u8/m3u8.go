package m3u8

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/cookiejar"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var Debug = false
var Logger = log.New(os.Stdout, "", log.Ldate|log.Ltime|log.Lshortfile)

var kvSplitByEqRegex = regexp.MustCompile(`([a-zA-Z0-9_-]+)=("[^"]+"|[^",]+)`)

// CustomM3u8Processor is an interface that allows for the caller to customize
// the processing of the file. For instance to handle DRM content.
type CustomM3u8Processor interface {
	// ProcessKey gets called after the ExtXKey value is extracted
	ProcessKey(f *M3u8File, key string, _logger *log.Logger) error
}

// CustomProcessor allows the caller to set their own processor.
var CustomProcessor CustomM3u8Processor

type M3u8File struct {
	Url string
	// urls of the all the segments
	Segments []string
	// ExtXKey is the m3u8 entry that describes the encryption
	ExtXKey string
	// GlobalKey is the global encryption key optionally mentioned in the m3u8
	// file
	GlobalKey []byte
	// IV is the AES IV is specified
	IV []byte
	// Renditions in case the file has different versions
	Renditions []Rendition

	// Apple HLS supports two encryption methods:
	// 		AES-128 It encrypts the whole segment with the Advanced Encryption Standard (AES) using a 128 bit key, Cipher Block Chaining (CBC) and PKCS7 padding.The CBC will be restarted with each segment using the Initialization Vector (IV) provided.
	// 		SAMPLE-AES It encrypts each individual media sample (e.g., video, audio, etc.) by its own with AES. The specific encryption and packaging depends on the media format, e.g., H.264, AAC, etc. SAMPLE-AES allows fine grained encryption modes, e.g., just encrypt I frames, just encrypt 1 out of 10 samples, etc. This could decrease the complexity of the decryption process. Several advantages result out of this approach as fewer CPU cycles are needed and for example mobile devices need less power consumption, higher resolutions can be effectively decrypted, etc.
	CryptoMethod string

	// ClosedCaptions - The value can be either a quoted-string or an
	// enumerated-string with the value NONE.  If the value is a quoted-string,
	// it MUST match the value of the GROUP-ID attribute of an EXT-X-MEDIA tag
	// elsewhere in the Playlist whose TYPE attribute is CLOSED-CAPTIONS, and
	// indicates the set of closed-caption Renditions that can be used when
	// playing the presentation.

	SubtitleStreams []SubtitleStream

	Audiostreams []Audiostream
}

// HasDefaultExtAudioStream returns true + the stream if the m3u8 file has an
// external audio stream set to default. This is used when the audio content
// isn't available in the main video file.
func (m *M3u8File) HasDefaultExtAudioStream() (bool, *Audiostream) {
	if m == nil {
		return false, nil
	}
	for _, s := range m.Audiostreams {
		if s.Default && s.URI != "" {
			return true, &s
		}
	}
	return false, nil
}

// Audiostream represents an audio track example:
// GROUP-ID="audio-aacl-64",NAME="Audio
// Français",LANGUAGE="fr",AUTOSELECT=YES,DEFAULT=YES,CHANNELS="2",URI="something-audio_fre=64000.m3u8"
type Audiostream struct {
	GroupID  string
	Name     string
	URI      string
	Lang     string
	Default  bool
	Channels int
}

// example: #EXT-X-MEDIA:TYPE=SUBTITLES,SUBFORMAT=WebVTT,GROUP-ID="textstream",NAME="Sous-Titres Français",LANGUAGE="fr",AUTOSELECT=YES,DEFAULT=YES,URI
type SubtitleStream struct {
	GroupID   string
	Name      string
	URI       string
	Lang      string
	Default   bool
	Subformat string
}

type M3u8Seg struct {
	Url      string
	Position int
	Response *http.Response
	// Download retries before giving up
	Retries int
}

// Process fetches the m3u8 file and processes it.
func (f *M3u8File) Process() error {
	return f.getSegments("", "")
}

func (f *M3u8File) getSegments(httpProxy, socksProxy string) error {
	jar, err := cookiejar.New(nil)
	if err != nil {
		panic(err)
	}

	client := &http.Client{
		Jar: jar,
	}

	response, err := client.Get(f.Url)
	if err != nil {
		Logger.Printf("Couldn't download url: %s - %s\n", f.Url, err)
		return err
	}
	defer response.Body.Close()

	contents, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return err
	}

	m3u8content := string(contents)
	m3u8Lines := strings.Split(strings.TrimSpace(m3u8content), "\n")

	if !strings.HasPrefix(m3u8Lines[0], "#EXTM3U") {
		return errors.New(f.Url + " is not a valid m3u8 file (missing #EXTM3U header)")
	}

	var segmentUrls []string
	var l string
	for i := 0; i < len(m3u8Lines); i++ {
		l = m3u8Lines[i]
		if strings.HasPrefix(l, "#EXT-X-KEY:") {
			Logger.Println("This m3u8 contains encrypted data:", l[11:])
			f.ExtXKey = l
		}
		// this isn't a standard m3u8 file, we have multiple variants of the
		// stream.
		if strings.HasPrefix(l, altStreamMarker) {
			if len(f.Renditions) < 1 {
				f.Renditions = []Rendition{}
			}
			rendition := ExtractRendition(l)
			i++
			rendition.URL = strings.TrimRight(strings.TrimRight(m3u8Lines[i], "\r\n"), "\r")
			f.Renditions = append(f.Renditions, rendition)
		}
		if strings.HasPrefix(l, audioStreamMarker) {
			stream := Audiostream{}
			for k, v := range decodeEqParamLine(l[len(audioStreamMarker):]) {
				switch k {
				case "GROUP-ID":
					stream.GroupID = v
				case "NAME":
					stream.Name = v
				case "LANGUAGE":
					stream.Lang = v
				case "DEFAULT":
					if strings.ToUpper(v) == "YES" {
						stream.Default = true
					}
				case "URI":
					stream.URI = makeURLAbsolute(v, f.Url)
				case "CHANNELS":
					chans, _ := strconv.ParseUint(v, 10, 32)
					stream.Channels = int(chans)
				}
			}

			if stream.Default && stream.URI != "" {
				Logger.Printf("Found default audio stream: %s (%s-%s) at %s\n", stream.GroupID, stream.Name, stream.Lang, stream.URI)
			} else if Debug {
				Logger.Printf("\nAudio stream: %s (%s-%s), default: %t at %s\n", stream.GroupID, stream.Name, stream.Lang, stream.Default, stream.URI)
			}
			f.Audiostreams = append(f.Audiostreams, stream)
		}
		// subtitle
		if strings.HasPrefix(l, subsStreamMarker) {

			stream := SubtitleStream{}
			for k, v := range decodeEqParamLine(l[len(subsStreamMarker):]) {
				switch k {
				case "GROUP-ID":
					stream.GroupID = v
				case "NAME":
					stream.Name = v
				case "LANGUAGE":
					stream.Lang = v
				case "DEFAULT":
					if strings.ToUpper(v) == "YES" {
						stream.Default = true
					}
				case "URI":
					stream.URI = makeURLAbsolute(v, f.Url)
				case "SUBFORMAT":
					stream.Subformat = v
				}
			}
			f.SubtitleStreams = append(f.SubtitleStreams, stream)
		}

		// #EXT-X-MAP:URI="something_v720.mp4",BYTERANGE="19779@0"
		if strings.HasPrefix(l, "#EXT-X-MAP:") {
			mapParams := decodeEqParamLine(l[11:])
			uri := makeURLAbsolute(mapParams["URI"], f.Url)
			segmentUrls = append(segmentUrls, uri)
		}

		// #EXT-X-BYTERANGE:1638290@2995376

	}

	// crypto
	if f.ExtXKey != "" {
		idx := strings.IndexByte(f.ExtXKey, ':')
		subLines := strings.Split(f.ExtXKey[idx+1:], ",")
		for _, sl := range subLines {
			slC := strings.Split(sl, "=")
			if len(slC) < 2 {
				continue
			}
			switch slC[0] {
			case "METHOD":
				f.CryptoMethod = strings.ToLower(slC[1])
			default:
				// Logger.Println(slC)
			}
		}

		if CustomProcessor != nil {
			if err := CustomProcessor.ProcessKey(f, f.ExtXKey, Logger); err != nil {
				return err
			}
		} else {

			if f.CryptoMethod == "SAMPLE-AES" {
				Logger.Print("SAMPLE-AES encryption not yet supported")
				return fmt.Errorf("stream is SAMPLE-AES encrypted, this is not yet supported")
			}

			if f.CryptoMethod != "SAMPLE-AES" {
				// aes-128
				// See https://developer.apple.com/library/content/technotes/tn2288/_index.html#//apple_ref/doc/uid/DTS40012238-CH1-ENCRYPT
				// See https://www.theoplayer.com/blog/content-protection-for-hls-with-aes-128-encryption
				idx = strings.Index(f.ExtXKey, "URI=")
				start := idx + 5
				if idx > 0 && len(f.ExtXKey) > start {
					idx = strings.IndexByte(f.ExtXKey[start:], '"')
					uri := f.ExtXKey[start : start+idx]
					if Debug {
						Logger.Println("Encryption key available from:", uri)
					}
					if len(uri) > 0 {
						if strings.Index(uri, "skd://") == 0 {
							f.GlobalKey = []byte("1a0770070728b80aeeb0902129f52878")
						} else {
							req, err := http.NewRequest("GET", uri, nil)
							if err != nil {
								return fmt.Errorf("could not create request for %s, err: %v", uri, err)
							}
							req.Header.Set("Accept", "text/plain")
							req.Header.Set("Origin", f.Url)
							req.Header.Set("Referer", f.Url)
							resp, err := client.Do(req)
							if err != nil {
								Logger.Printf("Failed to download the encryption key - %v\n", err)
								return err
							}
							if resp.StatusCode < 200 || resp.StatusCode > 299 {
								Logger.Printf("Failed to properly download the encryption key from %s - Status code: %d\n", uri, resp.StatusCode)
								return fmt.Errorf("encryption key response code: %d", resp.StatusCode)
							}
							f.GlobalKey, err = ioutil.ReadAll(resp.Body)
							resp.Body.Close()
							if err != nil {
								Logger.Printf("Failed to read the encryption key from source - %v\n", err)
								return err
							}
							if Debug {
								Logger.Printf("Encryption key: %v\n", f.GlobalKey)
							}
						}
					}
				}
			}
		}
		// TODO: support IV
		// TODO: support multiple keys, one per entry
	}

	// if we have multiple versions, pick the highest
	// TODO: make that a param/option
	if len(f.Renditions) > 0 {
		Logger.Printf("Found %d renditions, picking the highest resolution\n", len(f.Renditions))
		// sort the renditions so we get the biggest first
		sort.Slice(f.Renditions, func(i, j int) bool {
			return f.Renditions[i].Bandwidth > f.Renditions[j].Bandwidth
		})
		f.Renditions[0].URL = makeURLAbsolute(f.Renditions[0].URL, f.Url)
		Logger.Printf("Chosen rendition: %+v\n", f.Renditions[0])
		f.Renditions[0].URL = makeURLAbsolute(f.Renditions[0].URL, f.Url)
		
		// Preserve the audio and subtitle streams that were already parsed from the master playlist
		preservedAudioStreams := f.Audiostreams
		preservedSubtitleStreams := f.SubtitleStreams
		
		nf := &M3u8File{Url: f.Renditions[0].URL,
			SubtitleStreams: f.Renditions[0].SubtitleStreams,
		}
		if err := nf.getSegments(httpProxy, socksProxy); err != nil {
			return err
		}
		f.ExtXKey = nf.ExtXKey
		f.Url = nf.Url
		f.Segments = nf.Segments
		f.GlobalKey = nf.GlobalKey
		f.IV = nf.IV
		
		// Restore the preserved streams - these were parsed from the master playlist
		// and should not be lost when processing a rendition
		f.Audiostreams = preservedAudioStreams
		f.SubtitleStreams = preservedSubtitleStreams
		
		if Debug {
			Logger.Printf("Preserved %d audio streams and %d subtitle streams from master playlist\n", 
				len(f.Audiostreams), len(f.SubtitleStreams))
		}
		
		// Also append any subtitle streams that might be in the rendition itself
		f.SubtitleStreams = append(f.SubtitleStreams, nf.SubtitleStreams...)
		return nil
	}

	if len(segmentUrls) == 0 {
		for _, line := range m3u8Lines {
			// trim each line
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") {
				// deal with relative paths
				line = makeURLAbsolute(line, f.Url)
				segmentUrls = append(segmentUrls, line)
			}
		}
	}
	f.Segments = segmentUrls
	return nil
}

func splitAndTrimCommaList(str string) []string {
	list := strings.Split(str, ",")
	for i, item := range list {
		list[i] = strings.TrimSpace(item)
	}
	return list
}

// split a line of arguments with key=value format
func decodeEqParamLine(line string) map[string]string {
	out := make(map[string]string)
	for _, kv := range kvSplitByEqRegex.FindAllStringSubmatch(line, -1) {
		k, v := kv[1], kv[2]
		out[k] = strings.Trim(v, ` "`)
	}
	return out
}

func makeURLAbsolute(uri, refURL string) string {
	if !strings.HasPrefix(uri, "http") {
		if lastQuestionMark := strings.LastIndex(refURL, "?"); lastQuestionMark > 0 {
			refURL = refURL[:lastQuestionMark]
		}
		lastSlash := strings.LastIndex(refURL, "/")
		uri = refURL[:lastSlash+1] + uri
	}
	return uri
}
