package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

var queue chan *dlRequest

type dlRequest struct {
	Url    string
	Output string
}

type response struct {
	Url     string
	Output  string
	Message string
}

func init() {
	http.HandleFunc("/", mainHandler)
}

func ErrorCheck(err error) {
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}
}

func Start(port int, rootDir string) {
	log.Printf("About to start the server on port %d\n", port)
	queue = make(chan *dlRequest)
	go receiveJobs(queue)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), nil))
}

func receiveJobs(jobChan chan *dlRequest) {
	fmt.Println("Waiting for a new job ...")
	for {
		select {
		case job := <-jobChan:
			log.Println(job)

			// TODO: trigger the download job in another go routine
			//time.Sleep(5000 * time.Millisecond)
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
		queue <- &req
		res := response{req.Url, req.Output, "Added to the queue"}
		json.NewEncoder(w).Encode(res)
		return
	}
}
