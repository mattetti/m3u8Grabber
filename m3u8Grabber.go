package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/mattetti/m3u8GRabber/m3u8"
	"github.com/mattetti/m3u8GRabber/m3u8Utils"
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

func downloadM3u8Content(url *string, destFolder string, outputFilename, httpProxy, socksProxy *string) error {
	// tmp and final files
	tmpTsFile := destFolder + "/" + *outputFileName + ".ts"
	outputFilePath := destFolder + "/" + *outputFileName + ".mkv"

	log.Println("Downloading " + outputFilePath)
	if m3u8Utils.FileAlreadyExists(outputFilePath) {
		log.Println(outputFilePath + " already exists, we won't redownload it.\n")
		log.Println("Delete the file if you want to redownload it.\n")
	} else {
		segmentUrls, _ := m3u8.SegmentsForUrl(*url, httpProxy, socksProxy)
		err := m3u8.DownloadSegments(segmentUrls, tmpTsFile, httpProxy, socksProxy)
		if err != nil {
			return err
		}
		err = m3u8.TsToMkv(tmpTsFile, outputFilePath)
		if err != nil {
			return err
		}
		log.Println("Your file is available here: " + outputFilePath)
	}
	return nil
}

func downloadM3u8ContentWithRetries(url *string, destFolder string, outputFilename, httpProxy, socksProxy *string, retry int) error {
	var err error
	if retry < 3 {
		err = downloadM3u8Content(url, destFolder, outputFilename, httpProxy, socksProxy)
		if err != nil {
			log.Printf("ERROR: %s\n", err)
			err = downloadM3u8ContentWithRetries(url, destFolder, outputFilename, httpProxy, socksProxy, retry+1)
		}
	} else {
		return errors.New("Too many retries")
	}
	return err
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
	m3u8Utils.ErrorCheck(err)

	if *m3u8Url != "" {
		err = downloadM3u8ContentWithRetries(m3u8Url, pathToUse, outputFileName, httpProxy, socksProxy, 0)
		if err != nil {
			log.Printf("Error downloading %s, error: %s\n", m3u8Url, err)
		}
	}

	// server mode
	if *serverMode {
		m3u8Utils.StartServer(*serverPort, *dlRootDir)
	}
}
