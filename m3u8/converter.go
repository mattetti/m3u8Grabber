package m3u8

import (
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// TsToMp4 converts a mp4/aac TS file into a MKV file using ffmeg.
func TsToMp4(inTsPath, outMp4Path string) error {
	Logger.Println("converting to mp4")
	return TsToMkv(inTsPath, outMp4Path)
}

// TsToMkv converts a mp4/aac TS file into a MKV file using ffmeg.
func TsToMkv(inTsPath, outMkvPath string) (err error) {

	// Look for ffmpeg
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("where", "ffmpeg")
	} else {
		cmd = exec.Command("which", "ffmpeg")
	}
	buf, err := cmd.Output()
	if err != nil {
		Logger.Fatal("ffmpeg wasn't found on your system, it is required to convert video files.\n" +
			"Temp file left on your hardrive:\n" + inTsPath)
		os.Exit(1)
	}
	ffmpegPath := strings.Trim(strings.Trim(string(buf), "\r\n"), "\n")

	// ffmpeg flags
	// -y overwrites without asking
	//cmd = exec.Command(ffmpegPath, "-y", "-i", inTsPath, "-vcodec", "copy", "-acodec", "copy", outMkvPath)
	cmd = exec.Command(ffmpegPath, "-y", "-i", inTsPath, "-vcodec", "copy", "-acodec", "copy", "-bsf:a", "aac_adtstoasc", outMkvPath)

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

	if err := cmd.Start(); err != nil {
		return err
	}
	if err := cmd.Wait(); err != nil {
		Logger.Printf("ffmpeg Error: %v\n", err)
		Logger.Println("args", cmd.Args)
		return err
	}

	state := cmd.ProcessState
	if !state.Success() {
		Logger.Println("Error: something went wrong when trying to use ffmpeg")
	} else {
		err = os.Remove(inTsPath)
		if err != nil {
			Logger.Println("Couldn't delete temp file: " + inTsPath + "\n Please delete manually.\n")
		}
	}

	return err
}
