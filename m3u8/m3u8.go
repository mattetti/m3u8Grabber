package m3u8

import (
  "net/http"
  "fmt"
  "os"
  "io/ioutil"
  "strings"
  "log"
  "io"
  "os/exec"
  "time"
  "net"
  "github.com/mattetti/m3u8GRabber/m3u8Utils"
  "errors"
)

var Debug = false

// timeout of 30 seconds
func timeoutTransport() (*http.Transport){
  return &http.Transport{ ResponseHeaderTimeout: 5*time.Second,
		Dial: func(netw, addr string) (net.Conn, error) {
			deadline := time.Now().Add(30 * time.Second)
			c, err := net.DialTimeout(netw, addr, time.Second)
			if err != nil {
				return nil, err
			}
			c.SetDeadline(deadline)
			return c, nil
		},
  }
}

// Extracts the segments of a TS m3u8 file.
func SegmentsForUrl(url string) (*[]string, error) {

  transport := timeoutTransport()
  client := &http.Client{Transport: transport}

  response, err := client.Get(url)
  if err != nil {
    fmt.Fprintf(os.Stderr, "Couldn't download m3u8 url: %s\n", url)
    os.Exit(0)
  }
  defer response.Body.Close()

  contents, err := ioutil.ReadAll(response.Body)
  m3u8Utils.ErrorCheck(err)

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

func downloadUrl(client *http.Client, url *string, retries int) (resp *http.Response, err error) {
  client.Transport = timeoutTransport()
  resp, err = client.Get(*url)
  if err != nil {
    if (retries-1 == 0) {
      return resp, errors.New(*url + "failed to download")
    } else {
      return downloadUrl(client, url, retries-1)
    }
  }
  return resp, err
}

// Download a list of segments and put them together.
func DownloadSegments(segmentUrls *[]string, destination string) (err error) {

  out, err := os.Create(destination)
  defer out.Close()
  m3u8Utils.ErrorCheck(err)

  totalSegments := len(*segmentUrls)
  log.Println(fmt.Sprintf("downloading %d segments", totalSegments))

  client := &http.Client{} //Transport: transport}
  
  // TODO: concurent downloads
  for i, url := range *segmentUrls {

    resp, err := downloadUrl(client, &url, 5)
    if err != nil {
      log.Fatal(err)
      break
    } 

    if Debug {
      log.Println(fmt.Sprintf("downloaded %d of %d", (i+1), totalSegments))
    } else {
      fmt.Fprint(os.Stdout, ".")
    }

    _, err = io.Copy(out, resp.Body)
    m3u8Utils.ErrorCheck(err)
    resp.Body.Close()
  }

  if !Debug { fmt.Fprint(os.Stdout, "\n") }
  return err
}


// Converts a mp4/aac TS file into a MKV file
func TsToMkv(inTsPath string, outMkvPath string) (err error){

  // Look for ffmpeg
  cmd := exec.Command("which", "ffmpeg")
  buf, err := cmd.Output()
  if err != nil {
    log.Fatal("ffmpeg wasn't found on your system, it is required to convert to mkv.")
    os.Exit(1)
  }
  ffmpegPath := strings.Trim(string(buf), "\n")

  // ffmpeg flags
  // -y overwrites without asking
  cmd = exec.Command(ffmpegPath, "-y", "-i", inTsPath, "-vcodec", "copy", "-acodec", "copy", outMkvPath)

  // Pipe out the cmd output in debug mode
  if Debug {
    stdout, err := cmd.StdoutPipe()
    m3u8Utils.ErrorCheck(err)
    stderr, err := cmd.StderrPipe()
    m3u8Utils.ErrorCheck(err)
    go io.Copy(os.Stdout, stdout) 
    go io.Copy(os.Stderr, stderr) 
  }

  err = cmd.Start()
  m3u8Utils.ErrorCheck(err)
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
