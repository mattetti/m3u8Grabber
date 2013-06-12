package main

import (
	"flag"
	"fmt"
	"github.com/mattetti/m3u8GRabber/m3u8"
	"github.com/mattetti/m3u8GRabber/m3u8Utils"
	"log"
	"os"
)

// Flags
var m3u8Url = flag.String("m3u8", "", "Url of the m3u8 file to download.")
var outputFileName = flag.String("output", "downloaded_video", "The name of the output file without the extension.")
var httpProxy = flag.String("http_proxy", "", "The url of the HTTP proxy to use")
var socksProxy = flag.String("socks_proxy", "", "<host>:<port> of the socks5 proxy to use")
var debug = flag.Bool("debug", false, "Enable debugging messages.")

func m3u8ArgCheck() {
	if *m3u8Url == "" {
		fmt.Fprint(os.Stderr, "You have to pass a m3u8 url file using the right flag.\n")
		os.Exit(0)
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
	m3u8Utils.ErrorCheck(err)

	// tmp and final files
	tmpTsFile := pathToUse + "/" + *outputFileName + ".ts"
	outputFilePath := pathToUse + "/" + *outputFileName + ".mkv"

	log.Println("Downloading " + outputFilePath)
	if m3u8Utils.FileAlreadyExists(outputFilePath) {
		log.Println(outputFilePath + " already exists, we won't redownload it.\n")
		log.Println("Delete the file if you want to redownload it.\n")
	} else {
		segmentUrls, _ := m3u8.SegmentsForUrl(*m3u8Url, httpProxy, socksProxy)
		m3u8.DownloadSegments(segmentUrls, tmpTsFile, httpProxy, socksProxy)
		m3u8.TsToMkv(tmpTsFile, outputFilePath)
		log.Println("Your file is available here: " + outputFilePath)
	}

}
