package m3u8

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/hailiang/gosocks"
)

var (
	TimeoutDuration = 12 * time.Minute
	MaxRetries      = 3
)

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
		}
		return downloadUrl(client, url, retries-1, httpProxy, socksProxy)

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

func CleanPath(path string) string {
	path = strings.Replace(path, "?", "", -1)
	// On windows we don't want to remove C:\
	if strings.Index(path, ":") > -1 {
		path = path[:2] + strings.Replace(path[2:], ":", "", -1)
	}
	path = strings.Replace(path, ",", "", -1)
	//"!"#$%&'()*,:;<=>?[]^`{|}" string with "!"#$%&'()*,,;<=>?[]^`{|}".
	return strings.TrimSpace(path)
}
