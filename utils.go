package main

import (
  "os"
  "fmt"
  "log"
)

func m3u8ArgCheck(){
  if *m3u8Url == "" { 
    fmt.Fprint(os.Stderr, "You have to pass a m3u8 url file using the right flag.\n")
    os.Exit(0)
  }
}

func errorCheck (err error) {
  if err != nil {
    log.Fatal( err )
    os.Exit(1)
  }
}

func fileAlreadyExists(path string) bool {
  _, err := os.Stat(path)
 return !os.IsNotExist(err)
}
