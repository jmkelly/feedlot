package db

import (
	"io"
	"log"
	"os"
	"strings"
	"sync"
)

type LogWriter struct {
	db  *DB
	mu  sync.Mutex
	out io.Writer
}

func NewLogWriter(database *DB) *LogWriter {
	lw := &LogWriter{
		db:  database,
		out: os.Stdout,
	}
	log.SetOutput(lw)
	return lw
}

func (w *LogWriter) Write(p []byte) (n int, err error) {
	n, err = w.out.Write(p)

	msg := strings.TrimRight(string(p), "\n\r\t ")
	w.mu.Lock()
	if err := w.db.InsertLog("info", msg); err != nil {
		log.Printf("failed to write log to db: %v", err)
	}
	w.mu.Unlock()

	return
}
