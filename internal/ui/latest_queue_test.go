package ui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLatestQueueReplacesPendingValue(t *testing.T) {
	queue := make(chan int, 1)
	queue <- 1
	latestQueue[int]{channel: queue}.replace(2)
	assert.Equal(t, 2, <-queue)
}

func TestLatestQueueConsumerDrainsAfterFullCheck(t *testing.T) {
	queue := make(chan int, 1)
	queue <- 1
	drain := make(chan struct{})
	drained := make(chan struct{})
	go func() {
		<-drain
		<-queue
		close(drained)
	}()
	replacer := latestQueue[int]{channel: queue, afterFull: func() {
		close(drain)
		<-drained
	}}
	done := make(chan struct{})
	go func() {
		replacer.replace(2)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("replacement blocked after the consumer drained the queue")
	}
	require.Len(t, queue, 1)
	assert.Equal(t, 2, <-queue)
}
