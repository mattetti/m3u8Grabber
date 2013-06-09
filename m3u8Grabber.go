package main

import (
  "net/http"
  "io/ioutil"
  "flag"
  "log"
  "os"
  "strings"
  "fmt"
  "io"
  "os/exec"
)

// Flags
var m3u8Url = flag.String("m3u8", "", "Url of the m3u8 file to download.")
var outputFileName = flag.String("output", "downloaded_video", "The name of the output file without the extension.")
//var help = flag.Bool("help", false, "help")
var debug = flag.Bool("debug", false, "Enable debugging messages.")

// Extracts the segments from a m3u8 file
func m3u8Segments(url string) (*[]string, error) {

  response, err := http.Get(*m3u8Url)
  if err != nil {
    fmt.Fprintf(os.Stderr, "Couldn't download m3u8 url: %s\n", url)
    os.Exit(0)
  }
  defer response.Body.Close()

  contents, err := ioutil.ReadAll(response.Body)
  errorCheck(err)

  m3u8content := string(contents)
  m3u8Lines := strings.Split(strings.TrimSpace(m3u8content), "\n")

  if m3u8Lines[0] != "#EXTM3U" { 
    log.Fatal("not a valid m3u8 file")
    os.Exit(0)
  }

  var segmentUrls []string
  for i, value := range m3u8Lines {
    // trim each line
    m3u8Lines[i] = strings.TrimSpace(value)
    if m3u8Lines[i] != "" && !strings.HasPrefix(m3u8Lines[i], "#") { 
      segmentUrls = append(segmentUrls, m3u8Lines[i]) 
    }
  }

  return &segmentUrls, err
}

// Converts a mp4/aac TS file into a MKV file
func tsToMkv(inTsPath string, outMkvPath string) (err error){

  // Look for ffmpeg
  cmd := exec.Command("which", "ffmpeg")
  buf, err := cmd.Output()
  errorCheck(err)
  if len(buf) == 0 {
    log.Fatal("ffmpeg wasn't found on your system, it is required to convert to mkv.")
    os.Exit(0)
  }
  ffmpegPath := strings.Trim(string(buf), "\n")

  // ffmpeg flags
  // -y overwrites without asking
  cmd = exec.Command(ffmpegPath, "-y", "-i", inTsPath, "-vcodec", "copy", "-acodec", "copy", outMkvPath)

  // Pipe out the cmd output in debug mode
  if *debug {
    stdout, err := cmd.StdoutPipe()
    errorCheck(err)
    stderr, err := cmd.StderrPipe()
    errorCheck(err)
    go io.Copy(os.Stdout, stdout) 
    go io.Copy(os.Stderr, stderr) 
  }

  err = cmd.Start()
  errorCheck(err)
  cmd.Wait()

  state := cmd.ProcessState
  if !state.Success() {
    log.Fatal("Something went wrong when trying to use ffmpeg")
  } else {
    err = os.Remove(inTsPath)
    if err != nil{
      log.Println("Couldn't delete temp file: " + inTsPath + "\n Please delete manually.\n")
    }
  }

  return err
}

func downloadSegments(segmentUrls *[]string, destination string) (err error) {

  out, err := os.Create(destination)
  defer out.Close()
  errorCheck(err)

  totalSegments := len(*segmentUrls)
  log.Println(fmt.Sprintf("downloading %d segments", totalSegments))

  // TODO: concurent downloads
  for i, url := range *segmentUrls {
    resp, err := http.Get(url)
    defer resp.Body.Close()

    if *debug {
      log.Println(fmt.Sprintf("downlading %d of %d", (i+1), totalSegments))
    } else {
      fmt.Fprint(os.Stdout, ".")
    }

    if err != nil {
      log.Fatal(err)
    } else {
      _, err := io.Copy(out, resp.Body)
      errorCheck(err)
    }
  }

  return err
}

func main(){

  flag.Usage = func() {
    fmt.Fprintf(os.Stderr, "Usage: %s \n", os.Args[0])
    flag.PrintDefaults()
  }

  flag.Parse()
  m3u8ArgCheck()

  // Working dir
  pathToUse, err := os.Getwd()
  errorCheck(err)

  // tmp and final files
  tmpTsFile := pathToUse + "/" + *outputFileName + ".ts"
  outputFilePath := pathToUse + "/" + *outputFileName + ".mkv"

  log.Println("Downloading " + outputFilePath)
  if fileAlreadyExists(outputFilePath){
    log.Println(outputFilePath + " already exists, we won't redownload it.\n")
    log.Println("Delete the file if you want to redownload it.\n")
  } else {
    segmentUrls, _ := m3u8Segments(*m3u8Url)
    downloadSegments(segmentUrls, tmpTsFile)
    tsToMkv(tmpTsFile, outputFilePath)
    log.Println("Your file is available here: " + outputFilePath)
  }

}
