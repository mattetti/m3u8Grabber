package m3u8

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
)

var (
	TotalWorkers = 4
	DlChan       = make(chan *WJob)
)

type WJobType int

const (
	_ WJobType = iota
	ListDL
	FileDL
)

// LaunchWorkers starts download workers
func LaunchWorkers(wg *sync.WaitGroup, stop <-chan bool) {
	for i := 0; i < TotalWorkers; i++ {
		w := &Worker{i, wg}
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
	id int
	wg *sync.WaitGroup
}

func (w *Worker) Work() {
	fmt.Printf("worker %d is ready for action\n", w.id)
	for msg := range DlChan {
		w.dispatch(msg)
	}

	fmt.Printf("worker %d is out", w.id)
	w.wg.Done()
}

func (w *Worker) dispatch(job *WJob) {
	switch job.Type {
	case ListDL:
		// don't block the worker
		go w.downloadM3u8List(job)
	case FileDL:
		w.downloadM3u8Segment(job)
	default:
		fmt.Println("format not supported")
		return
	}

}

func (w *Worker) downloadM3u8List(j *WJob) {
	m3f := &M3u8File{Url: j.URL}
	m3f.getSegments("", "")
	j.wg = &sync.WaitGroup{}
	for i, segURL := range m3f.Segments {
		j.wg.Add(1)
		DlChan <- &WJob{
			Type:     FileDL,
			URL:      segURL,
			Pos:      i,
			wg:       j.wg,
			DestPath: j.DestPath,
			Filename: j.Filename,
		}
	}
	j.wg.Wait()
	// put the segments together
	fmt.Println("All segments downloaded")
	// assemble
	tmpTsFile := j.DestPath + "/" + j.Filename + ".ts"
	if _, err := os.Stat(j.DestPath); err != nil {
		if os.IsNotExist(err) {
			// file does not exist
			if err := os.MkdirAll(j.DestPath, os.ModePerm); err != nil {
				fmt.Printf("Failed to create path to %s - %s\n", j.DestPath, err)
			}
		} else {
			fmt.Printf("Failed to create tmp ts file: %s - %s", tmpTsFile, err)
			return
		}
	}
	mkvPath := filepath.Join(j.DestPath, j.Filename) + ".mkv"
	out, err := os.Create(tmpTsFile) //OpenFile(outputFilePath, os.O_APPEND|os.O_WRONLY, os.ModePerm)
	if err != nil {
		fmt.Printf("Failed to create output ts file - %s - %s\n", tmpTsFile, err)
		return
	}
	defer out.Close()

	if err != nil {
		fmt.Printf("Failed to open tmp ts file - %s - %s\n", tmpTsFile, err)
		return
	}

	for i := 0; i < len(m3f.Segments); i++ {
		file := segmentTmpPath(j.DestPath, j.Filename, i)
		in, err := os.OpenFile(file, os.O_RDONLY, 0666)
		if err != nil {
			fmt.Printf("Can't open %s because %s\n", file, err)
			return
		}
		_, err = io.Copy(out, in)
		in.Close()
		if err != nil {
			fmt.Println(err)
			return
		}
		out.Sync()
		err = os.Remove(file)
		if err != nil {
			fmt.Println(err)
			return
		}
	}
	if err := TsToMkv(tmpTsFile, mkvPath); err != nil {
		fmt.Println("ts to mkv error", err)
		return
	}
	fmt.Printf("Episode available at %s\n", mkvPath)
}

// downloadM3u8Segment downloads one segment of a m3u8 file
func (w *Worker) downloadM3u8Segment(j *WJob) {
	defer func() {
		if j.wg != nil {
			j.wg.Done()
		}
	}()

	fmt.Printf("worker %d - (%#s)", w.id, filepath.Join(j.DestPath, j.Filename))
	fmt.Printf(" - segment file %d\n", j.Pos)
	client := &http.Client{}
	resp, err := client.Get(j.URL)
	if err != nil {
		fmt.Println("Failed to download ", j.URL)
		return
	}

	if resp.StatusCode != 200 {
		fmt.Println(resp)
		return
	}

	destination := segmentTmpPath(j.DestPath, j.Filename, j.Pos)
	if fileAlreadyExists(destination) {
		return
	}

	if err := os.MkdirAll(j.DestPath, os.ModePerm); err != nil {
		fmt.Printf("m3u8 download failed - %s\n", err)
		return
	}

	out, err := os.Create(destination)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer out.Close()
	defer resp.Body.Close()
	_, err = io.Copy(out, resp.Body)
	fmt.Println("saved", destination)
}

func segmentTmpPath(path, filename string, pos int) string {
	return fmt.Sprintf("%s/%s._%d", path, filename, pos)
}
