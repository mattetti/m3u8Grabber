package m3u8

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
)

var Debug = false
var Logger = log.New(os.Stdout, "", log.Ldate|log.Ltime|log.Lshortfile)

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
}

type M3u8Seg struct {
	Url      string
	Position int
	Response *http.Response
	// Download retries before giving up
	Retries int
}

func (f *M3u8File) getSegments(httpProxy, socksProxy string) error {
	client := &http.Client{}
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

	if m3u8Lines[0] != "#EXTM3U" {
		return errors.New(f.Url + "is not a valid m3u8 file")
	}
	var l string
	for i := 0; i < len(m3u8Lines); i++ {
		l = m3u8Lines[i]
		if strings.HasPrefix(l, "#EXT-X-KEY:") {
			Logger.Println("This m3u8 contains encrypted data:", l[11:])
			f.ExtXKey = l
		}
		// this isn't a normal m3u8 file, we have multiple variations
		if strings.HasPrefix(l, altStreamMarker) {
			if len(f.Renditions) < 1 {
				f.Renditions = []Rendition{}
			}
			rendition := ExtractRendition(l)
			i++
			rendition.URL = m3u8Lines[i]
			f.Renditions = append(f.Renditions, rendition)
		}
	}

	// crypto
	if f.ExtXKey != "" {
		// See https://developer.apple.com/library/content/technotes/tn2288/_index.html#//apple_ref/doc/uid/DTS40012238-CH1-ENCRYPT
		// See https://www.theoplayer.com/blog/content-protection-for-hls-with-aes-128-encryption
		idx := strings.Index(f.ExtXKey, "URI=")
		start := idx + 5
		if idx > 0 && len(f.ExtXKey) > start {
			idx = strings.IndexByte(f.ExtXKey[start:], '"')
			uri := f.ExtXKey[start : start+idx]
			if Debug {
				Logger.Println("Encryption key available from:", uri)
			}
			if len(uri) > 0 {
				resp, err := downloadUrl(&http.Client{}, uri, 3, "", "")
				if err != nil {
					Logger.Printf("Failed to download the encryption key - %v\n", err)
					return err
				}
				if resp.StatusCode < 200 || resp.StatusCode > 299 {
					Logger.Printf("Failed to properly download the encryption key from %s - Status code: %d\n", uri, resp.StatusCode)
					return fmt.Errorf("Encryption key response code: %d", resp.StatusCode)
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
		Logger.Printf("Chosen rendition: %+v\n", f.Renditions[0])
		nf := &M3u8File{Url: f.Renditions[0].URL}
		if err := nf.getSegments(httpProxy, socksProxy); err != nil {
			return err
		}
		f.ExtXKey = nf.ExtXKey
		f.Url = nf.Url
		f.Segments = nf.Segments
		f.GlobalKey = nf.GlobalKey
		f.IV = nf.IV
		return nil
	}

	url, err := url.Parse(f.Url)
	if err != nil {
		return err
	}

	var segmentUrls []string
	for _, line := range m3u8Lines {
		// trim each line (working on a copu)
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			if !strings.HasPrefix(line, "http") {
				line = fmt.Sprintf("%s://%s/%s", url.Scheme, url.Host, line)
			}
			segmentUrls = append(segmentUrls, line)
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
