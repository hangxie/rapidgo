package jobs

import "bytes"

// maxLineBytes caps one line, bounding a command that emits no newline.
const maxLineBytes = 64 << 10

// lineWriter splits one stream of one job into lines, written from that
// stream's goroutine alone.
type lineWriter struct {
	emit   func(string)
	buffer []byte
}

func (w *lineWriter) Write(data []byte) (int, error) {
	total := len(data)
	for len(data) > 0 {
		index := bytes.IndexByte(data, '\n')
		if index < 0 {
			w.buffer = append(w.buffer, data...)
			break
		}
		w.buffer = append(w.buffer, data[:index]...)
		w.emitLine()
		data = data[index+1:]
	}
	for len(w.buffer) >= maxLineBytes {
		w.emit(string(w.buffer[:maxLineBytes]))
		w.buffer = append(w.buffer[:0], w.buffer[maxLineBytes:]...)
	}
	return total, nil
}

// flush reports a trailing line that the command left without a newline.
func (w *lineWriter) flush() {
	if len(w.buffer) > 0 {
		w.emitLine()
	}
}

func (w *lineWriter) emitLine() {
	line := w.buffer
	if len(line) > 0 && line[len(line)-1] == '\r' {
		line = line[:len(line)-1] // Tolerate CRLF output from Windows tools.
	}
	w.emit(string(line))
	w.buffer = w.buffer[:0]
}
