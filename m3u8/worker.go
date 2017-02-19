package m3u8

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var (
	TotalWorkers = 4
	DlChan       = make(chan *WJob)
	segChan      = make(chan *WJob)
	TmpFolder, _ = ioutil.TempDir("", "m3u8worker")
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
	Type     WJobType
	URL      string
	DestPath string
	Filename string
	Pos      int
	wg       *sync.WaitGroup
}

type Worker struct {
	id     int
	wg     *sync.WaitGroup
	master bool
	client *http.Client
}

func (w *Worker) Work() {
	log.Printf("worker %d is ready for action\n", w.id)
	if w.master {
		for msg := range DlChan {
			w.dispatch(msg)
		}
	} else {
		for msg := range segChan {
			w.dispatch(msg)
		}
	}

	log.Printf("worker %d is out", w.id)
}

func (w *Worker) dispatch(job *WJob) {
	switch job.Type {
	case ListDL:
		w.downloadM3u8List(job)
	case FileDL:
		w.downloadM3u8Segment(job)
	default:
		log.Println("format not supported")
		return
	}

}

func (w *Worker) downloadM3u8List(j *WJob) {
	m3f := &M3u8File{Url: j.URL}
	m3f.getSegments("", "")
	j.wg = &sync.WaitGroup{}
	j.Filename = CleanFilename(j.Filename)
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
	log.Printf("[%d] waiting for the segments to be downloaded", w.id)
	j.wg.Wait()
	// put the segments together
	log.Printf("All segments (%d) downloaded!\n", len(m3f.Segments))
	// assemble
	tmpTsFile := j.DestPath + "/" + j.Filename + ".ts"
	if _, err := os.Stat(j.DestPath); err != nil {
		if os.IsNotExist(err) {
			// file does not exist
			if err := os.MkdirAll(j.DestPath, os.ModePerm); err != nil {
				log.Printf("Failed to create path to %s - %s\n", j.DestPath, err)
			}
		} else {
			log.Printf("Failed to create tmp ts file: %s - %s", tmpTsFile, err)
			return
		}
	}
	mp4Path := filepath.Join(j.DestPath, j.Filename) + ".mp4"
	out, err := os.Create(tmpTsFile) //OpenFile(outputFilePath, os.O_APPEND|os.O_WRONLY, os.ModePerm)
	if err != nil {
		log.Printf("Failed to create output ts file - %s - %s\n", tmpTsFile, err)
		return
	}
	log.Printf("Preparing to convert to %s\n", mp4Path)

	for i := 0; i < len(m3f.Segments); i++ {
		file := segmentTmpPath(j.DestPath, j.Filename, i)
		if _, err := os.Stat(file); err != nil {
			log.Printf("skipping opening %s because %v\n", file, err)
			continue
		}

		in, err := os.OpenFile(file, os.O_RDONLY, 0666)
		if err != nil {
			log.Printf("Can't open %s because %s\n", file, err)
			out.Close()
			return
		}
		_, err = io.Copy(out, in)
		in.Close()
		if err != nil {
			log.Println(err)
			out.Close()
			return
		}
		out.Sync()
		err = os.Remove(file)
		if err != nil {
			log.Println(err)
			out.Close()
			return
		}
	}
	out.Close()
	if err := TsToMp4(tmpTsFile, mp4Path); err != nil {
		log.Println("ts to mp4 error", err)
		return
	}
	log.Printf("Episode available at %s\n", mp4Path)
}

// downloadM3u8Segment downloads one segment of a m3u8 file
func (w *Worker) downloadM3u8Segment(j *WJob) {
	defer func() {
		if j.wg != nil {
			j.wg.Done()
		}
	}()

	log.Printf("[%d] - %s - segment file %d\n", w.id, j.Filename, j.Pos)
	resp, err := w.client.Get(j.URL)
	if err != nil {
		log.Println("Failed to download ", j.URL)
		return
	}

	if resp.StatusCode != 200 {
		log.Println(resp)
		return
	}

	destination := segmentTmpPath(j.DestPath, j.Filename, j.Pos)
	if fileAlreadyExists(destination) {
		return
	}

	if err := os.MkdirAll(j.DestPath, os.ModePerm); err != nil {
		log.Printf("m3u8 download failed - %s\n", err)
		return
	}

	out, err := os.Create(destination)
	if err != nil {
		log.Println("error creating file", err)
		return
	}
	defer out.Close()
	defer resp.Body.Close()
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		log.Println("error copying resp body to file", err)
		return
	}
	log.Println("saved", destination)
}

func segmentTmpPath(path, filename string, pos int) string {
	return fmt.Sprintf("%s/%s._%d", TmpFolder, strings.Replace(filename, "/", "-", -1), pos)
}

func CleanFilename(name string) string {
	return strings.Replace(name, "/", "-", -1)
}
