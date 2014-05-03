package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
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

func (j *dlRequest) Download() error {
	log.Println(j)
	return m3u8.DownloadM3u8ContentWithRetries(j.Url, j.Path, j.Filename, httpProxy, socksProxy, 2)
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
	log.Printf("About to start the server on port %d\n", port)
	httpProxy = httpP
	socksProxy = socksP
	queue = make(chan *dlRequest)
	go receiveJobs(queue)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), nil))
}

// function designed to run in a goroutine
// and pull requests from a queue
func receiveJobs(jobChan chan *dlRequest) {
	fmt.Println("Waiting for a new job ...")
	var err error
	for {
		select {
		case job := <-jobChan:
			// TODO: trigger the download job and wait until it's done
			// to move on to the next job.
			err = job.Download()
			if err != nil {
				fmt.Printf("Error: %s", err)
			}
		case <-time.After(30 * time.Second):
			fmt.Printf(".")
		}
	}
}

// http handler used to add a new job to the queue
func mainHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		var req dlRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			log.Println(err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		// add to queue
		go func(qreq *dlRequest) {
			queue <- qreq
		}(&req)
		res := response{req.Url, req.Filename, "Added to the queue"}
		json.NewEncoder(w).Encode(res)
		return
	}
}
