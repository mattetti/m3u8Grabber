package m3u8Utils

import (
  "net/http"
  "fmt"
  "log"
)

var Jobs Queue

func StartServer(port int){
  log.Printf("About to start the server on port %d\n", port)
  log.Fatal(http.ListenAndServe( fmt.Sprintf(":%d", port), nil))
}

func init(){
  Jobs = NewCmdQueue()
  http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
    fmt.Fprintf(w, "Hello, %s", "World")
    // TODO
    // check post
    // process params
    // add to queue
  })
  // TODO:
  // start looping through the queue
}
