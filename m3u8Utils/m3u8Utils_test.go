package m3u8Utils

import (
  "testing"
  "github.com/stretchr/testify/assert"
)

func TestQueue(t *testing.T){
  q := NewCmdQueue()
  assert.Nil(t, q.Pop())
  cmd := &QueueCommand{
    M3u8Url: "test.m3u8", 
    DestinationPath: "/tmp/file.mkv",
  }
  q.Push(cmd)
  assert.Equal(t, cmd, q.Pop())
  assert.Equal(t, 0, q.Len())
  q.Push(cmd)
  assert.Equal(t, 1, q.Len())
  q.Push(cmd)
  assert.Equal(t, 2, q.Len())
  q.Pop()
  assert.Equal(t, 1, q.Len())
  q.Pop()
  assert.Equal(t, 0, q.Len())
  q.Pop()
  assert.Equal(t, 0, q.Len())
}
