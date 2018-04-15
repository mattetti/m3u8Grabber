package m3u8

import (
	"encoding/binary"
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
)

// LaunchWorkers starts download workers
func LaunchWorkers(wg *sync.WaitGroup, stop <-chan bool) {
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
	default:
		Logger.Println("format not supported")
		return
	}

}

func (w *Worker) downloadM3u8List(j *WJob) {
	m3f := &M3u8File{Url: j.URL}
	m3f.getSegments("", "")
	j.wg = &sync.WaitGroup{}
	j.Filename = CleanFilename(j.Filename)
	j.DestPath = CleanPath(j.DestPath)
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
	// put the segments together
	Logger.Printf("All segments (%d) downloaded!\n", len(m3f.Segments))
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
		Logger.Printf("Failed to create output ts file - %s - %s\n", tmpTsFile, err)
		return
	}
	if Debug {
		Logger.Printf("Reassembling %s\n", tmpTsFile)
	}

	var failed bool
	for i := 0; i < len(m3f.Segments); i++ {
		file := segmentTmpPath(j.DestPath, j.Filename, i)
		if _, err := os.Stat(file); err != nil {
			Logger.Printf("skipping opening %s because %v\n", file, err)
			continue
		}

		in, err := os.Open(file)
		if err != nil {
			Logger.Printf("Can't open %s because %s\n", file, err)
			failed = true
			break
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
			err = aesDecrypt(in, out, m3f.GlobalKey, iv)
			if Debug {
				Logger.Printf("Segment %d decrypted, error: %v\n", i, err)
			}
		} else {
			_, err = io.Copy(out, in)
		}

		in.Close()
		if err != nil {
			Logger.Println(err)
			failed = true
			break
		}
		out.Sync()
		err = os.Remove(file)
		if err != nil {
			Logger.Println("failed to remove", file, err)
		}
	}
	out.Close()
	if failed {
		out.Close()
		return
	}

	if j.SkipConverter {
		Logger.Printf("Content available at %s\n", tmpTsFile)
		return
	}

	Logger.Printf("Preparing to convert to %s\n", mp4Path)
	if err := TsToMp4(tmpTsFile, mp4Path); err != nil {
		Logger.Println("ts to mp4 error", err)
		return
	}
	Logger.Printf("Episode available at %s\n", mp4Path)
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

	destination := segmentTmpPath(j.DestPath, j.Filename, j.Pos)
	if fileAlreadyExists(destination) {
		return
	}

	if err := os.MkdirAll(j.DestPath, os.ModePerm); err != nil {
		Logger.Printf("m3u8 download failed - %s\n", err)
		return
	}

	out, err := os.Create(destination)
	if err != nil {
		Logger.Println("error creating file", err)
		return
	}
	defer out.Close()

	// We can't decrypt each segment if we have a global key.
	// In the case of a global key, segments bave to be decrypted
	// in order

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		Logger.Println("error copying resp body to file", err)
		return
	}
	if Debug {
		Logger.Println("saved", destination)
	}
}

func segmentTmpPath(path, filename string, pos int) string {
	return filepath.Join(TmpFolder, fmt.Sprintf("%s._%d", filenameCleaner.Replace(filename), pos))
}

func CleanFilename(name string) string {
	return CleanPath(filenameCleaner.Replace(name))
}
