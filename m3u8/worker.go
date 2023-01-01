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

	"github.com/asticode/go-astisub"
)

var (
	TotalWorkers    = 4
	DlChan          = make(chan *WJob)
	segChan         = make(chan *WJob)
	audioChan       = make(chan *WJob)
	TmpFolder, _    = ioutil.TempDir("", "m3u8")
	filenameCleaner = strings.NewReplacer("/", "-", "!", "", "?", "", ",", "")
)

type WJobType int

const (
	_ WJobType = iota
	ListDL
	FileDL
	CCDL
	MasterAudioDL
	AudioSegmentDL
)

// LaunchWorkers starts download workers
func LaunchWorkers(wg *sync.WaitGroup, stop <-chan bool) {
	// we need to recreate the dlChan and the segChan in case we want to restart workers.
	DlChan = make(chan *WJob)
	segChan = make(chan *WJob)
	audioChan = make(chan *WJob)
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
	SubsOnly      bool
	URL           string
	AbsolutePath  string
	DestPath      string
	Filename      string
	Pos           int
	// Err gets populated if something goes wrong while processing the job
	Err    error
	Crypto string
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
	case FileDL, AudioSegmentDL:
		w.downloadM3u8Segment(job)
	case CCDL:
		w.downloadM3u8CC(job)
	case MasterAudioDL:
		w.downloadM3u8Audio(job)
	default:
		Logger.Println("format not supported")
		return
	}

}

func (w *Worker) downloadM3u8List(j *WJob) {
	m3f := &M3u8File{Url: j.URL}
	if err := m3f.Process(); err != nil {
		j.Err = fmt.Errorf("failed to process the m3u8 file - %w", err)
		Logger.Printf("ERROR: %s", j.Err)
		return
	}
	j.Filename = CleanFilename(j.Filename)
	j.DestPath = CleanPath(j.DestPath)

	// Queue up the subs first
	if len(m3f.ClosedCaptions) > 0 {
		ccWG := &sync.WaitGroup{}
		for _, cc := range m3f.ClosedCaptions {
			// queue up the subtitles
			// FIXME: properly support multiple subtitles for a given source
			ccjob := &WJob{
				Type:          CCDL,
				URL:           cc,
				SkipConverter: true,
				DestPath:      j.DestPath,
				Filename:      j.Filename,
			}
			// kinda useless since there is usually only 1 cc file but still good to have
			ccWG.Add(1)
			ccjob.wg = ccWG
			segChan <- ccjob
		}
		ccWG.Wait()
	}

	var defaultAudiostreamPath string
	// check if there is a default audio stream to download
	if hasStream, s := m3f.HasDefaultExtAudioStream(); hasStream {
		// download and assemble the audio file
		audiostreamFilename := j.Filename +
			"_audio_" + s.Name + "_" + s.Lang
		defaultAudiostreamPath = j.DestPath + "/" + audiostreamFilename
		if Debug {
			Logger.Println("--> Queuing up default audio stream")
		}
		audioJob := &WJob{
			Type:          MasterAudioDL,
			URL:           s.URI,
			SkipConverter: true,
			DestPath:      j.DestPath,
			AbsolutePath:  defaultAudiostreamPath,
			Filename:      audiostreamFilename,
			Crypto:        m3f.CryptoMethod,
			Key:           m3f.GlobalKey,
			IV:            m3f.IV,
			wg:            &sync.WaitGroup{},
		}

		if !j.SubsOnly {
			audioJob.wg.Add(1)

			segChan <- audioJob
			audioJob.wg.Wait()
		}
	}
	// TODO: check if we should be getting other variant audio streams

	if !j.SubsOnly {
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
				Crypto:   m3f.CryptoMethod,
				Key:      m3f.GlobalKey,
				IV:       m3f.IV,
			}
		}

		Logger.Printf("[%d] waiting for the segments to be downloaded", w.id)
		j.wg.Wait()
		if len(m3f.Segments) == 0 {
			j.Err = errors.New("invalid m3u8 file, no segments to download found")
			Logger.Printf("ERROR: %s", j.Err)
			return
		}
	}

	// put the segments together
	Logger.Printf("All segments (%d) downloaded!\n", len(m3f.Segments))

	if j.SubsOnly {
		Logger.Printf("Only subs - executed!\n")
		return
	}
	Logger.Printf("Rebuilding the file now, this step might take a little while.")

	// create the temp video file
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
	defer out.Close()
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

	if failed {
		return
	}

	if j.SkipConverter {
		// TODO: merge the audio stream
		Logger.Printf("Content available at %s\n", tmpTsFile)
		return
	}

	Logger.Printf("Preparing to convert to %s\n", mp4Path)
	inputs := []string{tmpTsFile}
	if hasExtAudio, _ := m3f.HasDefaultExtAudioStream(); hasExtAudio {
		// add the audio stream to the mix
		inputs = append(inputs, defaultAudiostreamPath)
	}
	if err := TsToMp4(inputs, mp4Path, w.CCPath(j)); err != nil {
		j.Err = fmt.Errorf("mp4 conversion error error - %v", err)
		Logger.Println(j.Err)
		return
	}

	Logger.Printf("Episode available at %s\n", mp4Path)
}

func (w *Worker) downloadM3u8Audio(j *WJob) {
	defer func() {
		if j.wg != nil {
			j.wg.Done()
		}
	}()
	if Debug {
		Logger.Printf("Downloading audio stream from %s\n", j.URL)
	}
	m3f := &M3u8File{Url: j.URL, GlobalKey: j.Key}
	if err := m3f.Process(); err != nil {
		Logger.Printf("Failed to download audio stream: %v\n", err)
	}
	if len(m3f.Segments) < 1 {
		Logger.Printf("Empty audio stream playlist")
		return
	}
	wg := &sync.WaitGroup{}
	for i, segURL := range m3f.Segments {
		wg.Add(1)
		key := m3f.GlobalKey
		if key == nil && j.Key != nil {
			key = j.Key
		}
		segChan <- &WJob{
			Type:     AudioSegmentDL,
			URL:      segURL,
			Pos:      i,
			wg:       wg,
			DestPath: j.DestPath,
			Filename: j.Filename,
			Crypto:   m3f.CryptoMethod,
			Key:      key,
			IV:       j.IV,
		}
	}
	wg.Wait()

	// assemble the segments and save the file
	if len(m3f.Segments) == 0 {
		j.Err = errors.New("invalid m3u8 file, no audio segments to download found")
		Logger.Printf("ERROR: %s", j.Err)
		return
	}

	// get ready put the segments together
	Logger.Printf("All audio segments (%d) downloaded!\n", len(m3f.Segments))
	Logger.Printf("Rebuilding the audio file now, this step might take a little while.")

	// set destination file
	tmpAudioFile := j.AbsolutePath
	if _, err := os.Stat(j.DestPath); err != nil {
		if os.IsNotExist(err) {
			// file does not exist
			if err := os.MkdirAll(j.DestPath, os.ModePerm); err != nil {
				Logger.Printf("Failed to create path to %s - %s\n", j.DestPath, err)
			}
		} else {
			Logger.Printf("Failed to create tmp audio file: %s - %s", tmpAudioFile, err)
			return
		}
	}
	out, err := os.Create(tmpAudioFile)
	if err != nil {
		j.Err = fmt.Errorf("failed to create output ts file - %s - %s", tmpAudioFile, err)
		Logger.Println(j.Err)
		return
	}
	defer out.Close()
	if Debug {
		Logger.Printf("Reassembling %s\n", tmpAudioFile)
	}

	// assemble and cleanup
	for i := 0; i < len(m3f.Segments); i++ {
		segmentFile := segmentTmpPath(j.Filename, i)
		if _, err := os.Stat(segmentFile); err != nil {
			Logger.Printf("skipping opening %s because %v\n", segmentFile, err)
			continue
		}

		in, err := os.Open(segmentFile)
		if err != nil {
			Logger.Println(err)
			break
		}
		_, err = io.Copy(out, in)

		in.Close()
		if err != nil {
			Logger.Println(err)
			break
		}
		out.Sync()
		err = os.Remove(segmentFile)
		if err != nil {
			Logger.Println("failed to remove", segmentFile, err)
		}
	}
}

func (w *Worker) downloadM3u8CC(j *WJob) {
	if Debug {
		Logger.Printf("Downloading subtitles from %s\n", j.URL)
	}
	defer func() {
		if j.wg != nil {
			Logger.Printf("Reducing the wg")
			j.wg.Done()
		}
	}()
	m3f := &M3u8File{Url: j.URL}
	if err := m3f.getSegments("", ""); err != nil {
		Logger.Printf("Failed to download CC: %v\n", err)
	}
	if len(m3f.Segments) < 1 {
		Logger.Printf("Empty sub playlist")
		return
	}
	subFile := w.CCPath(j)
	Logger.Printf("Downloading sub file abs path: %s\n", subFile)
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
	defer out.Close()
	subs := &astisub.Subtitles{}
	var segSubs *astisub.Subtitles

	// downloading on the same working, which is a bit slow
	for i, segURL := range m3f.Segments {
		if (i+1)%10 == 0 {
			Logger.Printf("Downloading sub segment %d/%d", i+1, len(m3f.Segments))
		}
		res, err := http.Get(segURL)
		if err != nil {
			Logger.Printf("Failed to get subtitle part %s, %v\n", segURL, err)
			return
		}

		// TODO: check the format, don't assume VTT
		segSubs, err = astisub.ReadFromWebVTT(res.Body)
		if err != nil {
			Logger.Printf("Failed to get subtitle part %s, %v\n", segURL, err)
			res.Body.Close()
			continue
			// _, err = io.Copy(out, res.Body)
			// if err != nil {
			// 	Logger.Printf("Failed to append to the subtitle file, %v\n", err)
			// }
		}
		subs.Merge(segSubs)
		res.Body.Close()
	}
	// realign the subtitles
	if len(subs.Items) > 0 {
		subs.Add(-subs.Items[0].StartAt)
	}
	if err = subs.WriteToSRT(out); err != nil {
		Logger.Printf("Failed to write the subtitle file, %v\n", err)
	}

	Logger.Printf("Sub file available at %s\n", subFile)
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

	if len(j.Key) > 0 {
		if Debug {
			Logger.Printf("[%d] - Decrypting %s\n", w.id, destination)
		}
		// Decrypt in order if we have a global key
		if err := decryptFile(destination, j.Pos, j); err != nil {
			j.Err = fmt.Errorf("failed to decrypt segment %d - %v", j.Pos, err)
			Logger.Println(j.Err)
			return
		}
		fmt.Println(destination)
	}

	if j.Type == AudioSegmentDL {
		// we can't append ADTS files together, we have to convert the audio to
		// aac first.
		// We used to do that on the assembled file but that doesn't work with audio only m3u8 since you can't simply concatenate the audio adts files.
		if Debug {
			Logger.Printf("Converting audio segment %d to AAC\n", j.Pos)
		}
		if err := AdtsToAac(destination); err != nil {
			j.Err = fmt.Errorf("failed to convert audio segment %d - %v", j.Pos, err)
			Logger.Println(j.Err)
			return
		}
	}
}

func (w *Worker) CCPath(j *WJob) string {
	if w == nil || j == nil {
		return ""
	}

	if j.AbsolutePath == "" {
		j.AbsolutePath = j.DestPath + "/" + j.Filename + ".srt" // + filepath.Ext(m3f.Segments[0])
	}
	return j.AbsolutePath
}

func decryptFile(segmentFile string, i int, j *WJob) error {
	iv := j.IV
	if len(iv) == 0 {
		// TODO: check if true for sample-aes
		//
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
		return fmt.Errorf("can't create %s because %v", decryptedFilePath, err)
	}

	in, err := os.Open(segmentFile)
	if err != nil {
		return fmt.Errorf("can't open %s because %v", segmentFile, err)
	}

	if Debug {
		Logger.Printf("Decrypting segment %d\n", i)
	}

	if j.Crypto == "sample-aes" {
		// TODO: sample-aes cbcs (AES-128 CBC) but might be cenc (AES-128 CTR)
		err = sampleAESdecrypt(in, tOut, j)
	} else {
		err = aesDecrypt(in, tOut, j)
	}
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
	return os.Rename(decryptedFilePath, segmentFile)
}

func segmentTmpPath(filename string, pos int) string {
	return filepath.Join(TmpFolder, fmt.Sprintf("%s_%d", filenameCleaner.Replace(filename), pos))
}

func CleanFilename(name string) string {
	return CleanPath(filenameCleaner.Replace(name))
}
