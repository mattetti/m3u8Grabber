package m3u8

import (
	"errors"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/hailiang/gosocks"
)

// downloadM3u8ContentWithRetries fetches a m3u8 and convert it to mkv.
// Downloads can fail a few times and will retried.
func DownloadM3u8ContentWithRetries(url, destFolder, outputFilename, httpProxy, socksProxy string, retry int) error {
	var err error

	if retry < 3 {
		err = DownloadM3u8Content(url, destFolder, outputFilename, httpProxy, socksProxy)
		if err != nil {
			log.Printf("ERROR: %s\n", err)
			err = DownloadM3u8ContentWithRetries(url, destFolder, outputFilename, httpProxy, socksProxy, retry+1)
		}
	} else {
		return errors.New("Too many retries")
	}
	return err
}

// DownloadM3u8Content fetches and convert a m3u8 into a mkv file.
func DownloadM3u8Content(url, destFolder, outputFilename, httpProxy, socksProxy string) error {
	// tmp and final files
	tmpTsFile := destFolder + "/" + outputFilename + ".ts"
	outputFilePath := destFolder + "/" + outputFilename + ".mkv"

	log.Println("Downloading to " + outputFilePath)
	if fileAlreadyExists(outputFilePath) {
		log.Println(outputFilePath + " already exists, we won't redownload it.\n")
		log.Println("Delete the file if you want to redownload it.\n")
	} else {
		m3f := &M3u8File{Url: url}
		err := m3f.DownloadToFile(tmpTsFile, httpProxy, socksProxy)
		if err != nil {
			return err
		}
		err = TsToMkv(tmpTsFile, outputFilePath)
		if err != nil {
			return err
		}
		log.Println("Your file is available here: " + outputFilePath)
	}
	return nil
}

func fileAlreadyExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
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
