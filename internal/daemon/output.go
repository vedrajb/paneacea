package daemon

import (
	"context"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
	"github.com/paneacea/paneacea/internal/ipc"
	"strings"
	"sync"
)

const historyLimit = 2 * 1024 * 1024
const defaultSnapshotLimit = 512 * 1024

type outputBuffer struct {
	mu                  sync.Mutex
	data                []byte
	start               uint64
	end                 uint64
	exited              bool
	generation          uint64
	savedGeneration     uint64
	restored            bool
	restorationAttached bool
	changed             chan struct{}
	emulator            *vt.Emulator
	modes               map[ansi.Mode]bool
}

func newOutput() *outputBuffer { return &outputBuffer{changed: make(chan struct{})} }
func (o *outputBuffer) append(data []byte) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if len(data) > 0 {
		o.generation++
	}
	if o.emulator != nil {
		_, _ = o.emulator.Write(data)
	}
	if o.data == nil {
		o.data = make([]byte, historyLimit)
	}
	for len(data) > 0 {
		offset := int(o.end % historyLimit)
		written := copy(o.data[offset:], data)
		o.end += uint64(written)
		data = data[written:]
	}
	if o.end > historyLimit {
		o.start = o.end - historyLimit
	}
	close(o.changed)
	o.changed = make(chan struct{})
}
func (o *outputBuffer) exit() {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.exited {
		o.exited = true
		close(o.changed)
		o.changed = make(chan struct{})
	}
}
func (o *outputBuffer) read(ctx context.Context, sequence uint64) (ipc.Output, error) {
	for {
		o.mu.Lock()
		if sequence < o.start || sequence < o.end || o.exited {
			truncated := sequence < o.start
			if truncated && o.emulator != nil {
				result := o.snapshotLocked()
				result.Truncated = true
				o.mu.Unlock()
				return result, nil
			}
			if truncated {
				sequence = o.start
			}
			if sequence > o.end {
				sequence = o.end
			}
			end := sequence + 65536
			if end > o.end {
				end = o.end
			}
			data := o.rangeBytes(sequence, end)
			result := ipc.Output{Data: data, Sequence: end, Truncated: truncated, Exited: o.exited && end == o.end}
			o.mu.Unlock()
			return result, nil
		}
		changed := o.changed
		o.mu.Unlock()
		select {
		case <-ctx.Done():
			return ipc.Output{}, ctx.Err()
		case <-changed:
		}
	}
}

func (o *outputBuffer) snapshot() ipc.Output {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.snapshotLocked()
}
func (o *outputBuffer) snapshotLocked() ipc.Output {
	return o.snapshotLockedWithLimits(0, defaultSnapshotLimit, true, false)
}

func (o *outputBuffer) attachSnapshot(viewportOffset int) ipc.Output {
	o.mu.Lock()
	defer o.mu.Unlock()
	result := o.snapshotLocked()
	result.Restored = o.restored && !o.restorationAttached
	o.restorationAttached = true
	result.ViewportOffset = viewportOffset
	return result
}

func (o *outputBuffer) snapshotLockedWithLimits(maxLines, maxBytes int, includeModes, persistable bool) ipc.Output {
	if o.emulator == nil {
		return ipc.Output{Data: o.rangeBytes(o.start, o.end), Sequence: o.end, Exited: o.exited, Snapshot: true}
	}
	var data strings.Builder
	data.WriteString("\x1bc\x1b[?7l")
	renderedScreen := ""
	if !o.emulator.IsAltScreen() || !persistable {
		renderedScreen = strings.ReplaceAll(o.emulator.Render(), "\n", "\r\n")
	}
	if !o.emulator.IsAltScreen() || persistable {
		scrollback := o.emulator.Scrollback()
		if scrollback != nil {
			lines := make([]string, 0, min(scrollback.Len(), maxLines))
			size := 0
			for i := scrollback.Len() - 1; i >= 0; i-- {
				if maxLines > 0 && len(lines) >= maxLines {
					break
				}
				line := scrollback.Line(i).Render()
				limit := defaultSnapshotLimit
				if maxBytes > 0 {
					limit = maxBytes - len(renderedScreen) - 256
				}
				if size+len(line)+2 > limit {
					break
				}
				lines = append(lines, line)
				size += len(line)
			}
			for i := len(lines) - 1; i >= 0; i-- {
				data.WriteString(lines[i])
				data.WriteString("\r\n")
			}
			if len(lines) > 0 {
				for i := 0; i < o.emulator.Height()-1; i++ {
					data.WriteString("\r\n")
				}
				data.WriteString("\x1b[H")
			}
		}
	}
	if o.emulator.IsAltScreen() && !persistable {
		data.WriteString("\x1b[?1049h")
	}
	data.WriteString(renderedScreen)
	cursor := o.emulator.CursorPosition()
	fmt.Fprintf(&data, "\x1b[%d;%dH\x1b[?7h", cursor.Y+1, cursor.X+1)
	if includeModes {
		for mode, enabled := range o.modes {
			if enabled {
				data.WriteString(ansi.SetMode(mode))
			} else {
				data.WriteString(ansi.ResetMode(mode))
			}
		}
	}
	return ipc.Output{Data: []byte(data.String()), Sequence: o.end, Exited: o.exited, Snapshot: true, Restored: o.restored, Columns: o.emulator.Width(), Rows: o.emulator.Height()}
}

func (o *outputBuffer) persistedSnapshot(maxLines, maxBytes int) ([]byte, int, int, uint64, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.emulator == nil {
		return nil, 0, 0, 0, fmt.Errorf("terminal emulator is unavailable")
	}
	snapshot := o.snapshotLockedWithLimits(maxLines, maxBytes, false, true)
	if len(snapshot.Data) == 0 || len(snapshot.Data) > maxBytes {
		return nil, 0, 0, 0, fmt.Errorf("terminal history exceeds the %d byte limit", maxBytes)
	}
	return snapshot.Data, snapshot.Columns, snapshot.Rows, o.generation, nil
}

func (o *outputBuffer) restore(data []byte) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.emulator == nil {
		return fmt.Errorf("terminal emulator is unavailable")
	}
	if _, err := o.emulator.Write(data); err != nil {
		return err
	}
	// ConPTY clears and repaints its new screen; archive the old screen before clients attach.
	lines := strings.Split(o.emulator.Render(), "\n")
	for len(lines) > 0 && strings.TrimSpace(ansi.Strip(lines[len(lines)-1])) == "" {
		lines = lines[:len(lines)-1]
	}
	if _, err := o.emulator.Write([]byte(fmt.Sprintf("\x1b[0m\x1b[%d;1H%s\x1b[H", o.emulator.Height(), strings.Repeat("\r\n", len(lines))))); err != nil {
		return err
	}
	o.restored = true
	o.generation = 1
	o.savedGeneration = 1
	return nil
}

func (o *outputBuffer) markSaved(generation uint64) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if generation > o.savedGeneration {
		o.savedGeneration = generation
	}
}

func (o *outputBuffer) needsSave() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.generation > o.savedGeneration
}

func (o *outputBuffer) scrollbackLines() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.emulator == nil {
		return 0
	}
	return o.emulator.ScrollbackLen()
}

func (o *outputBuffer) applyHistoryLimit(lines int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.emulator != nil {
		o.emulator.SetScrollbackSize(lines)
		o.generation++
	}
}

func (o *outputBuffer) rangeBytes(start, end uint64) []byte {
	data := make([]byte, int(end-start))
	for written := 0; written < len(data); {
		offset := int((start + uint64(written)) % historyLimit)
		written += copy(data[written:], o.data[offset:])
	}
	return data
}
func (o *outputBuffer) resize(columns, rows int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.emulator != nil && (o.emulator.Width() != columns || o.emulator.Height() != rows) {
		o.emulator.Resize(columns, rows)
		o.generation++
	}
}
