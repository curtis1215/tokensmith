package ingest

import "os"

// CursorState is a durable, exported snapshot of one file's tail position,
// so a restarted poller resumes without re-reading or skipping.
type CursorState struct {
	Path   string `json:"path"`
	Device uint64 `json:"device,omitempty"`
	Inode  uint64 `json:"inode"`
	Offset int64  `json:"offset"`
}

// ExportCursors snapshots the internal per-file cursor map.
func (p *Poller) ExportCursors() []CursorState {
	out := make([]CursorState, 0, len(p.cursors))
	for path, c := range p.cursors {
		out = append(out, CursorState{Path: path, Device: c.device, Inode: c.inode, Offset: c.offset})
	}
	return out
}

// ImportCursors restores cursor positions captured by ExportCursors.
func (p *Poller) ImportCursors(cs []CursorState) {
	for _, c := range cs {
		device := c.Device
		if device == 0 {
			if fi, err := os.Stat(c.Path); err == nil && inodeOf(fi) == c.Inode {
				device = identityOf(fi).device
			}
		}
		p.cursors[c.Path] = fileCursor{device: device, inode: c.Inode, offset: c.Offset}
	}
}
