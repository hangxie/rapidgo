package ui

// latestQueue keeps the newest pending value in a one-slot worker channel.
type latestQueue[T any] struct {
	channel   chan T
	afterFull func() // allows tests to interleave a consumer after the full check
}

func (queue latestQueue[T]) replace(value T) {
	select {
	case queue.channel <- value:
		return
	default:
	}
	if queue.afterFull != nil {
		queue.afterFull()
	}
	select {
	case <-queue.channel:
	default:
	}
	queue.channel <- value
}
