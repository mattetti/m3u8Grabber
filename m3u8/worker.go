package m3u8

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var (
	TotalWorkers    = 4
	DlChan          = make(chan *WJob)
	segChan         = make(chan *WJob)
	TmpFolder, _    = ioutil.TempDir("", "m3u8")
	filenameCleaner = strings.NewReplacer("/", "-", "!", "", "?", "", ",", "")
)

type WJobType int

const (
	_ WJobType = iota
	ListDL
	FileDL
	CCDL
)

// LaunchWorkers starts download workers
func LaunchWorkers(wg *sync.WaitGroup, stop <-chan bool) {
	// we need to recreate the dlChan and the segChan in case we want to restart workers.
	DlChan = make(chan *WJob)
	segChan = make(chan *WJob)
	// the master worker downloads one full m3u8 at a time but
	// segments are downloaded concurrently
	masterW := &Worker{id: 0, wg: wg, master: true}
	go masterW.Work()

	for i := 1; i < TotalWorkers+1; i++ {
		w := &Worker{id: i, wg: wg, client: &http.Client{}}
		go w.Work()
	}
}

type WJob struct {
	Type          WJobType
	SkipConverter bool
	URL           string
	DestPath      string
	Filename      string
	Pos           int
	// Err gets populated if something goes wrong while processing the job
	Err error
	// Key is the AES segment key if available
	Key []byte
	IV  []byte
	wg  *sync.WaitGroup
}

type Worker struct {
	id     int
	wg     *sync.WaitGroup
	master bool
	client *http.Client
}

func (w *Worker) Work() {
	Logger.Printf("worker %d is ready for action\n", w.id)
	if w.master {
		w.wg.Add(1)
		for msg := range DlChan {
			w.dispatch(msg)
		}
		close(segChan)
		w.wg.Done()
	} else {
		for msg := range segChan {
			w.dispatch(msg)
		}
	}

	Logger.Printf("worker %d is out", w.id)
}

func (w *Worker) dispatch(job *WJob) {
	switch job.Type {
	case ListDL:
		w.downloadM3u8List(job)
	case FileDL:
		w.downloadM3u8Segment(job)
	case CCDL:
		w.downloadM3u8CC(job)
	default:
		Logger.Println("format not supported")
		return
	}

}

func (w *Worker) downloadM3u8List(j *WJob) {
	m3f := &M3u8File{Url: j.URL}
	m3f.getSegments("", "")
	j.Filename = CleanFilename(j.Filename)
	j.DestPath = CleanPath(j.DestPath)
	// Queue up the subs first
	for _, cc := range m3f.ClosedCaptions {
		// queue up the subtitles
		// FIXME: properly support multiple subtitles for a given source
		ccjob := &WJob{
			Type:          CCDL,
			URL:           cc,
			SkipConverter: true,
			DestPath:      j.DestPath,
			Filename:      j.Filename}
		segChan <- ccjob
	}
	//
	j.wg = &sync.WaitGroup{}
	for i, segURL := range m3f.Segments {
		j.wg.Add(1)
		segChan <- &WJob{
			Type:     FileDL,
			URL:      segURL,
			Pos:      i,
			wg:       j.wg,
			DestPath: j.DestPath,
			Filename: j.Filename,
		}
	}
	Logger.Printf("[%d] waiting for the segments to be downloaded", w.id)
	j.wg.Wait()
	if len(m3f.Segments) == 0 {
		j.Err = errors.New("invalid m3u8 file, no segments to download found")
		Logger.Printf("ERROR: %s", j.Err)
		return
	}
	// put the segments together
	Logger.Printf("All segments (%d) downloaded!\n", len(m3f.Segments))
	Logger.Printf("Rebuilding the file now, this step might take a little while.")

	// assemble
	tmpTsFile := j.DestPath + "/" + j.Filename + ".ts"
	if _, err := os.Stat(j.DestPath); err != nil {
		if os.IsNotExist(err) {
			// file does not exist
			if err := os.MkdirAll(j.DestPath, os.ModePerm); err != nil {
				Logger.Printf("Failed to create path to %s - %s\n", j.DestPath, err)
			}
		} else {
			Logger.Printf("Failed to create tmp ts file: %s - %s", tmpTsFile, err)
			return
		}
	}

	mp4Path := filepath.Join(j.DestPath, j.Filename) + ".mp4"
	out, err := os.Create(tmpTsFile)
	if err != nil {
		j.Err = fmt.Errorf("failed to create output ts file - %s - %s", tmpTsFile, err)
		Logger.Println(j.Err)
		return
	}
	if Debug {
		Logger.Printf("Reassembling %s\n", tmpTsFile)
	}

	var failed bool

	for i := 0; i < len(m3f.Segments); i++ {
		segmentFile := segmentTmpPath(j.Filename, i)
		if _, err := os.Stat(segmentFile); err != nil {
			Logger.Printf("skipping opening %s because %v\n", segmentFile, err)
			continue
		}

		// Decrypt in order if we have a global key
		if len(m3f.GlobalKey) > 0 {
			iv := m3f.IV
			if len(iv) == 0 {
				// An EXT-X-KEY tag that does not have an IV attribute indicates
				// that the Media Sequence Number is to be used as the IV when
				// decrypting a Media Segment, by putting its big-endian binary
				// representation into a 16-octet (128-bit) buffer and padding
				// (on the left) with zeros.
				buf := make([]byte, 4)
				binary.BigEndian.PutUint32(buf, uint32(i+1))
				iv = append(make([]byte, 12), buf...)
			}
			// create a temp file to decrypt to and then switch our input to
			// this decrypted file.
			decryptedFilePath := segmentFile + ".dec"
			tOut, err := os.Create(decryptedFilePath)
			if err != nil {
				Logger.Printf("Can't create %s because %s\n", decryptedFilePath, err)
				failed = true
				break
			}

			in, err := os.Open(segmentFile)
			if err != nil {
				Logger.Printf("Can't open %s because %s\n", segmentFile, err)
				failed = true
				break
			}

			if Debug {
				Logger.Printf("Decrypting segment %d\n", i)
			}

			err = aesDecrypt(in, tOut, m3f.GlobalKey, iv)
			if Debug {
				Logger.Printf("Segment %d decrypted, error: %v\n", i, err)
			}
			tOut.Sync()
			tOut.Close()
			in.Close()
			err = os.Remove(segmentFile)
			if err != nil {
				Logger.Println("failed to remove encrypted", segmentFile, err)
			}
			// rename so the decrypted file replaces the original segment
			os.Rename(decryptedFilePath, segmentFile)
		}

		// TODO: more robust check if twe are dealing with an audio segment
		if strings.ToLower(filepath.Ext(m3f.Segments[0])) == ".aac" {
			// we can't append ADTS files together, we have to convert the audio to
			// aac first.
			// We used to do that on the assembled file but that doesn't work with audio only m3u8 since you can't simply concatenate the audio adts files.
			if Debug {
				Logger.Printf("Converting segment %d to AAC\n", i)
			}
			if err := AdtsToAac(segmentFile); err != nil {
				Logger.Printf(err.Error())
				break
			}
		}

		in, err := os.Open(segmentFile)
		if err != nil {
			Logger.Println(err)
			failed = true
			break
		}
		_, err = io.Copy(out, in)

		in.Close()
		if err != nil {
			Logger.Println(err)
			failed = true
			break
		}
		out.Sync()
		err = os.Remove(segmentFile)
		if err != nil {
			Logger.Println("failed to remove", segmentFile, err)
		}
	}
	out.Close()
	if failed {
		return
	}

	if j.SkipConverter {
		Logger.Printf("Content available at %s\n", tmpTsFile)
		return
	}

	Logger.Printf("Preparing to convert to %s\n", mp4Path)
	if err := TsToMp4(tmpTsFile, mp4Path); err != nil {
		j.Err = fmt.Errorf("mp4 conversion error error - %v", err)
		Logger.Println(j.Err)
		return
	}
	Logger.Printf("Episode available at %s\n", mp4Path)
}

func (w *Worker) downloadM3u8CC(j *WJob) {
	Logger.Printf("Downloading subtitles from %s\n", j.URL)
	m3f := &M3u8File{Url: j.URL}
	if err := m3f.getSegments("", ""); err != nil {
		Logger.Printf("Failed to download CC: %v\n", err)
	}
	if len(m3f.Segments) < 1 {
		Logger.Printf("Empty sub playlist")
		return
	}
	subFile := j.DestPath + "/" + j.Filename + filepath.Ext(m3f.Segments[0])
	if _, err := os.Stat(j.DestPath); err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(j.DestPath, os.ModePerm); err != nil {
				Logger.Printf("Failed to create path to %s - %s\n", j.DestPath, err)
			}
		} else {
			Logger.Printf("Failed to create the sub file: %s - %s", subFile, err)
			return
		}
	}
	out, err := os.Create(subFile)
	if err != nil {
		Logger.Printf("Failed to create the sub file: %s - %s", subFile, err)
		return
	}
	for _, segURL := range m3f.Segments {
		res, err := http.Get(segURL)
		if err != nil {
			Logger.Printf("Failed to get subtitle part %s, %v\n", segURL, err)
			return
		}
		_, err = io.Copy(out, res.Body)
		if err != nil {
			Logger.Printf("Failed to append to the subtitle file, %v\n", err)
		}
		res.Body.Close()
	}
	Logger.Printf("Sub file available at %s\n", subFile)
	// convert to srt
	if err := SubToSrt(subFile); err != nil {
		Logger.Printf("Failed to convert the subtitles - %v\n", err)
	}
}

// downloadM3u8Segment downloads one segment of a m3u8 file
func (w *Worker) downloadM3u8Segment(j *WJob) {
	defer func() {
		if j.wg != nil {
			j.wg.Done()
		}
	}()

	Logger.Printf("[%d] - %s - segment file %d\n", w.id, j.Filename, j.Pos)
	resp, err := w.client.Get(j.URL)
	if err != nil {
		Logger.Println("Failed to download ", j.URL)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		Logger.Println(resp)
		return
	}

	destination := segmentTmpPath(j.Filename, j.Pos)
	if fileAlreadyExists(destination) {
		return
	}

	if err := os.MkdirAll(j.DestPath, os.ModePerm); err != nil {
		j.Err = fmt.Errorf("m3u8 download failed, couldn't create the destination path - %v", err)
		Logger.Println(j.Err)
		return
	}

	out, err := os.Create(destination)
	if err != nil {
		j.Err = fmt.Errorf("error creating destination file - %v", err)
		Logger.Println(j.Err)
		return
	}
	defer out.Close()

	// We can't decrypt each segment if we have a global key.
	// In the case of a global key, segments have to be decrypted
	// in order

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		j.Err = fmt.Errorf("error copying resp body to file - %v", err)
		Logger.Println(j.Err)
		return
	}
	if Debug {
		Logger.Println("saved", destination)
	}
}

func segmentTmpPath(filename string, pos int) string {
	return filepath.Join(TmpFolder, fmt.Sprintf("%s_%d", filenameCleaner.Replace(filename), pos))
}

func CleanFilename(name string) string {
	return CleanPath(filenameCleaner.Replace(name))
}
