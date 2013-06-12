package m3u8Utils

import (
	"log"
	"os"
)

func ErrorCheck(err error) {
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}
}

func FileAlreadyExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}
