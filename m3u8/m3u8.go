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

type M3u8Seg struct {
	Url      string
	Position int
	Response *http.Response
	// Download retries before giving up
	Retries int
}

func (f *M3u8File) DownloadToFile(tmpFile, httpProxy, socksProxy string) error {
	err := f.getSegments(httpProxy, socksProxy)
	if err != nil {
		return err
	}
	return f.dlSegments(tmpFile, httpProxy, socksProxy)
}

func (f *M3u8File) getSegments(httpProxy, socksProxy string) error {
	//transport, err := customTransport(httpProxy, socksProxy)
	//if err != nil {
	//return err
	//}
	client := &http.Client{} //Transport: transport}
	response, err := client.Get(f.Url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Couldn't download url: %s - %s\n", f.Url, err)
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
	fmt.Printf("Downloading %d segments\n", totalSegments)

	client := &http.Client{} //Transport: transport}

	var resp *http.Response
	errChan := make(chan error)
	// TODO: sets a chan of chan to limit the amount
	// of concurrenct downloads
	ch := make(chan *M3u8Seg)
	dlQueue := make(chan *M3u8Seg, 3)
	var downloadsLeft int

	// enqueue the downloads on the dlQueue
	for i, url := range f.Segments {
		downloadsLeft++
		go func(dlUrl string, pos int) {
			dlQueue <- &M3u8Seg{Url: dlUrl, Position: pos, Retries: 5}
		}(url, i)
	}

	var procdSegments int
	for {
		select {
		// download queue buffered at 3 concurrent dls max
		case seg := <-dlQueue:
			go func(segToDl *M3u8Seg) {
				resp, err = downloadUrl(client, segToDl.Url, segToDl.Retries, httpProxy, socksProxy)
				if err != nil {
					if segToDl.Retries > 1 {
						fmt.Println("Retying downloading ", segToDl.Url)
						segToDl.Retries--
						dlQueue <- segToDl
					} else {
						downloadsLeft--
						errChan <- err
						return
					}
				}
				segToDl.Response = resp
				ch <- segToDl
			}(seg)
		case r, ok := <-ch:
			if !ok {
				// channel closed
				return nil
			}
			procdSegments++
			if Debug {
				log.Println(r.Response.Request.URL, r.Position)
			}
			//if Debug {
			log.Printf("%d/%d\n", procdSegments, totalSegments)
			//} else {
			/*				fmt.Fprint(os.Stdout, ".")*/
			/*			}*/
			if r.Response.StatusCode != 200 {
				fmt.Println(r.Response)
				continue
			}
			// TODO: copy to different files and put them together at the end
			// otherwise the segments won't be assembled in the right order.
			out, err := os.Create(fmt.Sprintf("%s._%d", destination, r.Position))
			if err != nil {
				errChan <- err
				continue
			}
			defer out.Close()
			defer r.Response.Body.Close()
			_, err = io.Copy(out, r.Response.Body)
			if err != nil {
				errChan <- err
				continue
			}
			downloadsLeft--
			if downloadsLeft < 1 {
				fmt.Printf("Assemble the %d ts files\n", totalSegments)
				out, err := os.OpenFile(destination, os.O_APPEND|os.O_WRONLY, 0774)
				defer out.Close()
				if err != nil {
					return err
				}
				files := make([]string, totalSegments)
				for i := 0; i < totalSegments; i++ {
					files[i] = fmt.Sprintf("%s._%d", destination, i)
				}
				fmt.Printf("Assembling %d ts segments\n", len(files))
				for _, file := range files {
					if file == "" {
						continue
					}
					in, err := os.OpenFile(file, os.O_RDONLY, 0666)
					if err != nil {
						return errors.New(fmt.Sprintf("Can't open %s because %s\n", file, err))
					}
					_, err = io.Copy(out, in)
					in.Close()
					if err != nil {
						return err
					}
					out.Sync()
					err = os.Remove(file)
					if err != nil {
						return err
					}
				}
				return nil
			}
		case err := <-errChan:
			// TODO: do a retry after the download is moved to a struct
			fmt.Println("failed ", err)
			return err
		}
	}
	return err
}
