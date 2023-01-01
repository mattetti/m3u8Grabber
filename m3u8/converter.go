package m3u8

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ConcatMediaList adds together all the files listed in the passed text file
// and saves the output in the outPath. The listPath file is deleted
// automatically.
func ConcatMediaList(listPath, outPath string) error {
	ffmpegPath, err := FfmpegPath()
	if err != nil {
		Logger.Fatal("ffmpeg wasn't found on your system, it is required to concatenate the video files.\n" +
			"Temp file left on your hardrive:\n" + listPath)
		os.Exit(1)
	}

	//ffmpeg -f concat -i filelist.txt -c copy outPath
	cmd := exec.Command(ffmpegPath, "-f", "concat", "-i", listPath, "-codec", "copy", outPath)

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
		err = os.Remove(listPath)
		if err != nil {
			Logger.Println("Couldn't delete temp file: " + listPath + "\n Please delete manually.\n")
		}
	}

	return err
}

// AdtsToAac reencodes a ADTS audio file into a clean aac
func AdtsToAac(path string) error {
	ffmpegPath, err := FfmpegPath()
	if err != nil {
		Logger.Fatal("ffmpeg wasn't found on your system, it is required to convert the adts file.\n" +
			"Temp file left on your hardrive:\n" + path)
		os.Exit(1)
	}

	outPath := path + ".aac"

	// ffmpeg -i input.aac -c copy -bsf:a aac_adtstoasc output.aac
	cmd := exec.Command(ffmpegPath, "-i", path, "-codec", "copy", "-bsf:a", "aac_adtstoasc", outPath)

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
		return fmt.Errorf("ffmpeg command failed")
	}

	err = os.Remove(path)
	if err != nil {
		Logger.Println("Couldn't delete temp file: " + path + "\n Please delete manually.\n")
		return err
	}
	return os.Rename(outPath, path)
}

// TsToMp4 converts a mp4/aac TS file into a MKV file using ffmeg.
func TsToMp4(inTsPath []string, outMp4Path string, subFile string) error {
	Logger.Println("converting to mp4")
	return TsToMkv(inTsPath, outMp4Path, subFile)
}

// TsToMkv converts a mp4/aac TS file into a MKV file using ffmeg.
func TsToMkv(inTsPaths []string, outMkvPath string, subFile string) (err error) {
	ffmpegPath, err := FfmpegPath()
	if err != nil {
		Logger.Fatalf("ffmpeg wasn't found on your system, it is required to convert video files.\n"+
			"Temp file(s) left on your hardrive: %+v\n", inTsPaths)
		os.Exit(1)
	}

	// -y overwrites without asking
	args := []string{"-y"}

	// add all the inTSPaths to the args
	for _, path := range inTsPaths {
		args = append(args, "-i", path)
	}

	if subFile != "" {
		// check if the file does exist
		if fileAlreadyExists(subFile) {
			// if it exists, add the subtitle to the ffmpeg command
			args = append(args,
				"-i", subFile,
				"-c:s", "mov_text")
		}
	}

	// add the rest of the args
	args = append(args,
		"-vcodec", "copy",
		"-acodec", "copy",
		"-bsf:a", "aac_adtstoasc")

	args = append(args, outMkvPath)

	cmd := exec.Command(ffmpegPath, args...)

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
		for _, path := range inTsPaths {
			err = os.Remove(path)
			if err != nil {
				Logger.Println("Couldn't delete temp file: " + path + "\n Please delete manually.\n")
			}
		}
		if subFile != "" {
			err = os.Remove(subFile)
			if err != nil {
				Logger.Println("Couldn't delete temp subfile: " + subFile + "\n Please delete manually.\n")
			}
		}
	}

	return err
}

func FfmpegPath() (string, error) {
	// Look for ffmpeg
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("where", "ffmpeg")
	} else {
		cmd = exec.Command("which", "ffmpeg")
	}
	buf, err := cmd.Output()
	return strings.Trim(strings.Trim(string(buf), "\r\n"), "\n"), err
}

// SubToSrt converts a sub file into a srt if ffmpeg supports the input format.
func SubToSrt(inSubPath string) (err error) {

	outSubPath := inSubPath[:len(inSubPath)-len(filepath.Ext(inSubPath))]
	outSubPath += ".srt"

	ffmpegPath, err := FfmpegPath()
	if err != nil {
		Logger.Fatal("ffmpeg wasn't found on your system, it is required to convert video files.\n" +
			"Temp file left on your hardrive:\n" + inSubPath)
		os.Exit(1)
	}

	// ffmpeg flags
	// -y overwrites without asking
	//cmd = exec.Command(ffmpegPath, "-y", "-i", inSubPath, outSubPath)
	cmd := exec.Command(ffmpegPath, "-y", "-i", inSubPath, outSubPath)

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
	}
	return err
}
