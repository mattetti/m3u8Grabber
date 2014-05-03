package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/mattetti/m3u8Grabber/m3u8"
	"github.com/mattetti/m3u8Grabber/server"
)

// Flags
var (
	m3u8Url        = flag.String("m3u8", "", "Url of the m3u8 file to download.")
	outputFileName = flag.String("output", "downloaded_video", "The name of the output file without the extension.")
	httpProxy      = flag.String("http_proxy", "", "The url of the HTTP proxy to use.")
	socksProxy     = flag.String("socks_proxy", "", "<host>:<port> of the socks5 proxy to use.")
	debug          = flag.Bool("debug", false, "Enable debugging messages.")
	serverPort     = flag.Int("server_port", 13535, "The port to run the http server on.")
	serverMode     = flag.Bool("server", false, "Enable running a local web server (not working yet).")
	dlRootDir      = flag.String("root_dir", "/tmp", "The root dir to download files to")
)

func m3u8ArgCheck() {
	if *m3u8Url == "" && !*serverMode {
		fmt.Fprint(os.Stderr, "You have to pass a m3u8 url file using the right flag or enable the server mode.\n")
		os.Exit(2)
	}
}

func main() {

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s \n", os.Args[0])
		flag.PrintDefaults()
	}

	flag.Parse()
	m3u8ArgCheck()
	m3u8.Debug = *debug

	// Working dir
	pathToUse, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	if *m3u8Url != "" {
		err = m3u8.DownloadM3u8ContentWithRetries(*m3u8Url, pathToUse, *outputFileName, *httpProxy, *socksProxy, 0)
		if err != nil {
			log.Printf("Error downloading %s, error: %s\n", *m3u8Url, err)
			os.Exit(2)
		}
	}

	// server mode
	if *serverMode {
		server.Start(*serverPort, *dlRootDir, *httpProxy, *socksProxy)
	}
}
