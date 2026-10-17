package editor

import (
	"crypto/sha256"
	"io"
	"os"
	"sync"
)

// DiskState describes the identity and metadata associated with a content hash.
// It is immutable and safe to pass to background workers.
type DiskState struct {
	info os.FileInfo
	Hash [32]byte
}

func (s DiskState) SameMetadata(other DiskState) bool {
	if s.info == nil || other.info == nil {
		return s.info == nil && other.info == nil
	}
	return os.SameFile(s.info, other.info) && s.info.Size() == other.info.Size() && s.info.ModTime().Equal(other.info.ModTime()) && s.info.Mode() == other.info.Mode()
}

var diskBuffers = sync.Pool{New: func() any { p := make([]byte, 32*1024); return &p }}

// InspectDisk avoids opening unchanged files on metadata-only polls. Full
// inspections stream content and reject files that change during the read.
func InspectDisk(path string, previous DiskState, full bool) (DiskState, error) {
	info, err := os.Stat(path)
	if err != nil {
		return DiskState{}, err
	}
	state := DiskState{info: info, Hash: previous.Hash}
	if !full && state.SameMetadata(previous) {
		return state, nil
	}
	if !info.Mode().IsRegular() {
		return DiskState{}, &os.PathError{Op: "inspect", Path: path, Err: os.ErrInvalid}
	}
	f, err := os.Open(path)
	if err != nil {
		return DiskState{}, err
	}
	defer f.Close()
	before, err := f.Stat()
	if err != nil {
		return DiskState{}, err
	}
	hash := sha256.New()
	buf := diskBuffers.Get().(*[]byte)
	_, err = io.CopyBuffer(hash, struct{ io.Reader }{f}, *buf)
	diskBuffers.Put(buf)
	if err != nil {
		return DiskState{}, err
	}
	after, err := os.Stat(path)
	if err != nil {
		return DiskState{}, err
	}
	if !(DiskState{info: before}).SameMetadata(DiskState{info: after}) {
		return DiskState{}, ErrConflict
	}
	state.info = after
	copy(state.Hash[:], hash.Sum(nil))
	return state, nil
}
func (d *Document) DiskState() DiskState         { return d.diskState }
func (d *Document) SaveGeneration() uint64       { return d.saveGeneration }
func (d *Document) MatchesDisk(s DiskState) bool { return d.Exists && d.diskHash == s.Hash }
