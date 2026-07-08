package ingest

// CursorState is a durable, exported snapshot of one file's tail position,
// so a restarted poller resumes without re-reading or skipping.
type CursorState struct {
	Path   string `json:"path"`
	Inode  uint64 `json:"inode"`
	Offset int64  `json:"offset"`
}

// ExportCursors snapshots the internal per-file cursor map.
func (p *Poller) ExportCursors() []CursorState {
	out := make([]CursorState, 0, len(p.cursors))
	for path, c := range p.cursors {
		out = append(out, CursorState{Path: path, Inode: c.inode, Offset: c.offset})
	}
	return out
}

// ImportCursors restores cursor positions captured by ExportCursors.
func (p *Poller) ImportCursors(cs []CursorState) {
	for _, c := range cs {
		p.cursors[c.Path] = fileCursor{inode: c.Inode, offset: c.Offset}
	}
}
