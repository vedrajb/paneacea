package terminal

import (
	"io"
	"sync"
	"time"
)

const (
	DefaultColumns uint16 = 120
	DefaultRows    uint16 = 30
)

type Options struct {
	Executable       string
	Arguments        []string
	WorkingDirectory string
	Environment      map[string]string
	Columns          uint16
	Rows             uint16
	BatchInterval    time.Duration
	BatchBytes       int
	OnOutput         func(string)
	OnExit           func(error)
}

type backend interface {
	io.Reader
	Write([]byte) (int, error)
	Resize(uint16, uint16) error
	Wait() error
	Close() error
	PID() uint32
}

type Session struct {
	backend    backend
	batcher    *OutputBatcher
	onExit     func(error)
	done       chan struct{}
	readDone   chan struct{}
	closeOnce  sync.Once
	finishOnce sync.Once
	closeMu    sync.Mutex
	closeError error
	writeMu    sync.Mutex
}

func NewSession(options Options) (*Session, error) {
	if options.Columns == 0 {
		options.Columns = DefaultColumns
	}
	if options.Rows == 0 {
		options.Rows = DefaultRows
	}
	if err := ValidateSize(options.Columns, options.Rows); err != nil {
		return nil, err
	}
	process, err := newConPTY(options)
	if err != nil {
		return nil, err
	}
	session := &Session{
		backend:  process,
		onExit:   options.OnExit,
		done:     make(chan struct{}),
		readDone: make(chan struct{}),
	}
	session.batcher = NewOutputBatcher(options.BatchInterval, options.BatchBytes, func(data []byte) {
		if options.OnOutput != nil {
			options.OnOutput(string(data))
		}
	})
	go session.readOutput()
	go session.waitForExit()
	return session, nil
}

func (s *Session) Write(data []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	for len(data) > 0 {
		written, err := s.backend.Write(data)
		if written > 0 {
			data = data[written:]
		}
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}

func (s *Session) Resize(columns, rows uint16) error {
	if err := ValidateSize(columns, rows); err != nil {
		return err
	}
	return s.backend.Resize(columns, rows)
}

func (s *Session) Close() error {
	s.closeOnce.Do(func() {
		err := s.backend.Close()
		s.closeMu.Lock()
		s.closeError = err
		s.closeMu.Unlock()
	})
	<-s.done
	s.closeMu.Lock()
	defer s.closeMu.Unlock()
	return s.closeError
}

func (s *Session) PID() uint32 {
	return s.backend.PID()
}

func (s *Session) readOutput() {
	defer close(s.readDone)
	buffer := make([]byte, 32*1024)
	for {
		read, err := s.backend.Read(buffer)
		if read > 0 {
			s.batcher.Add(buffer[:read])
		}
		if err != nil {
			return
		}
	}
}

func (s *Session) waitForExit() {
	exitErr := s.backend.Wait()
	<-s.readDone
	s.finishOnce.Do(func() {
		s.batcher.Close()
		close(s.done)
		if s.onExit != nil {
			s.onExit(exitErr)
		}
	})
}
