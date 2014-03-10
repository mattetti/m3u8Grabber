package m3u8

import (
	"errors"
	"log"
	"os"
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
