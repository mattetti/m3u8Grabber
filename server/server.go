package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/mattetti/m3u8Grabber/m3u8"
)

var (
	queue      chan *dlRequest
	httpProxy  string
	socksProxy string
)

func init() {
	http.HandleFunc("/", mainHandler)
}

type dlRequest struct {
	// source of the m3u8 file to download
	Url string
	// path to download the file to
	Path string
	// output filename
	Filename string
}

func (j *dlRequest) Download() {
	//log.Printf("job: %#v\n", j)
	//return m3u8.DownloadM3u8ContentWithRetries(j.Url, j.Path, j.Filename, httpProxy, socksProxy, 0)
	m3u8.DlChan <- &m3u8.WJob{Type: m3u8.ListDL, URL: j.Url, DestPath: j.Path, Filename: j.Filename}
}

type response struct {
	Url     string
	Output  string
	Message string
}

func ErrorCheck(err error) {
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}
}

func Start(port int, rootDir, httpP, socksP string) {
	log.Printf("About to start the https server on port %d\n", port)
	httpProxy = httpP
	socksProxy = socksP
	queue = make(chan *dlRequest)
	var w sync.WaitGroup
	stopChan := make(chan bool)
	m3u8.LaunchWorkers(&w, stopChan)
	go receiveJobs(queue)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), nil))
	//log.Fatal(http.ListenAndServeTLS(fmt.Sprintf(":%d", port), "cert.pem", "key.pem", nil))
	// TODO: handle safe shutdown
}

// function designed to run in a goroutine
// and pull requests from a queue
func receiveJobs(jobChan chan *dlRequest) {
	fmt.Println("Waiting for a new job ...")
	for {
		select {
		case job := <-jobChan:
			job.Download()
		case <-time.After(30 * time.Second):
			fmt.Printf(".")
		}
	}
}

// http handler used to add a new job to the queue
func mainHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, HEAD")

	if r.Method == "POST" {
		var req dlRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			log.Println(err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		// add to queue
		go func(qreq *dlRequest) {
			m3u8.DlChan <- &m3u8.WJob{Type: m3u8.ListDL, URL: req.Url, DestPath: req.Path, Filename: req.Filename}
			//go func(qreq *dlRequest) {
			//log.Printf("Queued up %s\n\n", qreq.Filename)
			//queue <- qreq
		}(&req)
		res := response{req.Url, req.Filename, "Added to the queue"}
		json.NewEncoder(w).Encode(res)
		return
	}
}
