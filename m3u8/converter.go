package m3u8

import (
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
)

// TsToMkv converts a mp4/aac TS file into a MKV file using ffmeg.
func TsToMkv(inTsPath, outMkvPath string) (err error) {

	// Look for ffmpeg
	cmd := exec.Command("which", "ffmpeg")
	buf, err := cmd.Output()
	if err != nil {
		log.Fatal("ffmpeg wasn't found on your system, it is required to convert to mkv.\n" +
			"Temp file left on your hardrive:\n" + inTsPath)
		os.Exit(1)
	}
	ffmpegPath := strings.Trim(string(buf), "\n")

	// ffmpeg flags
	// -y overwrites without asking
	cmd = exec.Command(ffmpegPath, "-y", "-i", inTsPath, "-vcodec", "copy", "-acodec", "copy", outMkvPath)

	// Pipe out the cmd output in debug mode
	if Debug {
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return err
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			return err
		}
		go io.Copy(os.Stdout, stdout)
		go io.Copy(os.Stderr, stderr)
	}

	err = cmd.Start()
	if err != nil {
		return err
	}
	cmd.Wait()

	state := cmd.ProcessState
	if !state.Success() {
		log.Fatal("Something went wrong when trying to use ffmpeg")
	} else {
		err = os.Remove(inTsPath)
		if err != nil {
			log.Println("Couldn't delete temp file: " + inTsPath + "\n Please delete manually.\n")
		}
	}

	return err
}