package m3u8Utils

import (
	"log"
	"os"
  "container/list"
)

func ErrorCheck(err error) {
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}
}

func FileAlreadyExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

// Queue wrapper on top of a list container
type Queue struct {
  list *list.List
}

func NewCmdQueue() Queue {
  return Queue{list: list.New()}
}

func (q Queue) Push(cmd *QueueCommand) {
  q.list.PushBack(cmd)
}

func (q Queue) Pop() *QueueCommand {
  cmdEl := q.list.Front()
  if cmdEl == nil {
    return nil
  }
  cmd, ok := cmdEl.Value.(*QueueCommand)
  if ok {
    q.list.Remove(cmdEl)
    return cmd
  } else {
    return nil
  }
}

func (q Queue) Len() int {
  return q.list.Len()
}

type QueueCommand struct {
  M3u8Url string
  DestinationPath string
}
