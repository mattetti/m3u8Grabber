package m3u8

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/hailiang/gosocks"
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

// TsToMkv converts a mp4/aac TS file into a MKV file using ffmeg.
func TsToMkv(inTsPath, outMkvPath string) (err error) {

	// Look for ffmpeg
	cmd := exec.Command("which", "ffmpeg")
	buf, err := cmd.Output()
	if err != nil {
		log.Fatal("ffmpeg wasn't found on your system, it is required to convert to mkv.\n" +
			"Temp file left on your hardrive:\n" + inTsPath)
		os.Exit(1)
	}
	ffmpegPath := strings.Trim(string(buf), "\n")

	// ffmpeg flags
	// -y overwrites without asking
	cmd = exec.Command(ffmpegPath, "-y", "-i", inTsPath, "-vcodec", "copy", "-acodec", "copy", outMkvPath)

	// Pipe out the cmd output in debug mode
	if Debug {
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return err
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			return err
		}
		go io.Copy(os.Stdout, stdout)
		go io.Copy(os.Stderr, stderr)
	}

	err = cmd.Start()
	if err != nil {
		return err
	}
	cmd.Wait()

	state := cmd.ProcessState
	if !state.Success() {
		log.Fatal("Something went wrong when trying to use ffmpeg")
	} else {
		err = os.Remove(inTsPath)
		if err != nil {
			log.Println("Couldn't delete temp file: " + inTsPath + "\n Please delete manually.\n")
		}
	}

	return err
}

// downloadUrl is a wrapper allowing to download content by setting up optional proxies and supporting retries.
func downloadUrl(client *http.Client, url string, retries int, httpProxy, socksProxy string) (resp *http.Response, err error) {
	client.Transport, err = customTransport(httpProxy, socksProxy)
	if err != nil {
		return nil, err
	}
	resp, err = client.Get(url)
	// Handle retries
	if err != nil {
		if retries-1 == 0 {
			return nil, errors.New(url + " failed to download")
		} else {
			return downloadUrl(client, url, retries-1, httpProxy, socksProxy)
		}
	}
	return resp, err
}

// customTransport lets users use custom http or socks proxy.
// If none of the proxy settings were passed, a normal transport is used
// with some default timeout values.
func customTransport(httpProxy, socksProxy string) (*http.Transport, error) {
	var transport *http.Transport
	var err error
	// http proxy transport
	if httpProxy != "" {
		url, err := url.Parse(httpProxy)
		if err != nil {
			return nil, err
		}
		transport = &http.Transport{Proxy: http.ProxyURL(url)}
		// socks proxy transport
	} else if socksProxy != "" {
		dialSocksProxy := socks.DialSocksProxy(socks.SOCKS5, socksProxy)
		transport = &http.Transport{Dial: dialSocksProxy}
	} else {
		// timeout transport
		transport = &http.Transport{ResponseHeaderTimeout: 5 * time.Second,
			Dial: func(netw, addr string) (net.Conn, error) {
				deadline := time.Now().Add(45 * time.Second)
				c, err := net.DialTimeout(netw, addr, time.Second)
				if err != nil {
					return nil, err
				}
				c.SetDeadline(deadline)
				return c, nil
			},
		}
	}

	return transport, err
}
