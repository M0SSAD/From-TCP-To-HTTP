package response

import (
	"fmt"
	"io"

	"boot.mossad.http/internal/headers"
)


type WriterState int

const (
	StateStatusPending WriterState = iota // 0
	StateHeadersPending                   // 1
	StateBodyPending                      // 2
)


type Writer struct {
	writer io.Writer
	state WriterState
}

func NewWriter (w io.Writer) *Writer {
	return &Writer{
		writer: w,
		state: StateStatusPending,
	}
}

func (w *Writer) WriteStatusLine(statusCode StatusCode) error {
	if w.state != StateStatusPending {
		return fmt.Errorf("cannot write status line: current state is %v", w.state)
	}

	if err := WriteStatusLine(w.writer, statusCode); err != nil {
		return err
	}

	w.state = StateHeadersPending
	return nil
}


func (w *Writer) WriteHeaders(headers headers.Headers) error {
	if w.state != StateHeadersPending {
		return fmt.Errorf("cannot write header line: current state is %v", w.state)
	}

	if err := WriteHeaders(w.writer, headers); err != nil {
		return err
	}

	if _, err := w.writer.Write([]byte("\r\n")); err != nil {
		return err
	}

	w.state = StateBodyPending
	return nil
}


func (w *Writer) WriteBody(p []byte) (int, error) {
	if w.state != StateBodyPending {
		return 0, fmt.Errorf("cannot write body: headers not written yet")
	}

	return w.writer.Write(p)
}

