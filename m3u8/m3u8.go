package m3u8

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
)

var Debug = false

type M3u8File struct {
	Url string
	// urls of the all the segments
	Segments []string
}

func (f *M3u8File) DownloadToFile(tmpFile, httpProxy, socksProxy string) error {
	err := f.getSegments(httpProxy, socksProxy)
	if err != nil {
		return err
	}
	return f.dlSegments(tmpFile, httpProxy, socksProxy)
}

func (f *M3u8File) getSegments(httpProxy, socksProxy string) error {
	transport, err := customTransport(httpProxy, socksProxy)
	if err != nil {
		return err
	}
	client := &http.Client{Transport: transport}
	response, err := client.Get(f.Url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Couldn't download url: %s\n", f.Url)
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

func (f *M3u8File) dlSegments(destination, httpProxy, socksProxy string) error {
	if f.Segments == nil || len(f.Segments) < 1 {
		log.Println("No segments to download")
		return nil
	}
	out, err := os.Create(destination)
	if err != nil {
		return err
	}
	defer out.Close()

	totalSegments := len(f.Segments)
	log.Println(fmt.Sprintf("downloading %d segments", totalSegments))

	client := &http.Client{} //Transport: transport}
	// TODO: concurent downloads
	var resp *http.Response
	for i, url := range f.Segments {
		resp, err = downloadUrl(client, url, 5, httpProxy, socksProxy)
		if err != nil {
			break
		}
		defer resp.Body.Close()

		if Debug {
			log.Println(fmt.Sprintf("downloaded %d of %d", (i + 1), totalSegments))
		} else {
			fmt.Fprint(os.Stdout, ".")
		}

		_, err = io.Copy(out, resp.Body)
		if err != nil {
			break
		}
	}
	if err != nil {
		return err
	}
	if !Debug {
		fmt.Fprint(os.Stdout, "\n")
	}
	return err
}
