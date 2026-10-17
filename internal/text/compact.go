package text

import "sort"

const maintenanceThreshold = 8 * 1024 * 1024

type storageInterval struct{ start, end, dest int }

// Compaction is an immutable worker result. It may only be installed in the
// exact buffer lineage from which its task was captured.
type Compaction struct {
	version    int
	generation uint64
	skip       bool
	added      []byte
	pieces     []piece
	history    []transaction
}

func (b *Buffer) NeedsCompaction() bool { return b.maintenanceBytes >= maintenanceThreshold }
func (b *Buffer) RetryCompaction() {
	b.maintenanceBytes = max(b.maintenanceBytes, maintenanceThreshold)
}

// CompactionTask captures immutable piece lists and backing-store prefixes.
// Appends never change existing bytes; the worker doesn't read mutable Buffer state.
func (b *Buffer) CompactionTask() func() *Compaction {
	if !b.NeedsCompaction() {
		return nil
	}
	b.maintenanceBytes = 0
	original, added := b.original, b.added
	pieces := append([]piece(nil), b.pieces...)
	history := append([]transaction(nil), b.history...)
	version, generation := b.Version, b.historyGeneration
	return func() *Compaction {
		result := &Compaction{version: version, generation: generation, skip: true}
		var refs [2][]storageInterval
		collect := func(ps []piece) {
			for _, p := range ps {
				k := 0
				if p.Added {
					k = 1
				}
				refs[k] = append(refs[k], storageInterval{start: p.Start, end: p.Start + p.Len})
			}
		}
		collect(pieces)
		for _, t := range history {
			collect(t.Before.Pieces)
			collect(t.After.Pieces)
		}
		live := 0
		for k := range refs {
			intervals := refs[k]
			sort.Slice(intervals, func(i, j int) bool { return intervals[i].start < intervals[j].start })
			merged := intervals[:0]
			for _, r := range intervals {
				n := len(merged)
				if n > 0 && r.start <= merged[n-1].end {
					merged[n-1].end = max(merged[n-1].end, r.end)
				} else {
					merged = append(merged, r)
				}
			}
			for i := range merged {
				merged[i].dest = live
				live += merged[i].end - merged[i].start
			}
			refs[k] = merged
		}
		total := len(original) + len(added)
		waste := total - live
		if waste <= maintenanceThreshold || waste <= total/2 {
			return result
		}
		result.skip = false
		result.added = make([]byte, 0, live)
		for _, r := range refs[0] {
			result.added = append(result.added, original[r.start:r.end]...)
		}
		for _, r := range refs[1] {
			result.added = append(result.added, added[r.start:r.end]...)
		}
		remap := func(ps []piece) []piece {
			out := make([]piece, len(ps))
			for i, p := range ps {
				k := 0
				if p.Added {
					k = 1
				}
				intervals := refs[k]
				at := sort.Search(len(intervals), func(i int) bool { return intervals[i].end > p.Start })
				r := intervals[at]
				out[i] = piece{Added: true, Start: r.dest + p.Start - r.start, Len: p.Len}
			}
			return out
		}
		result.pieces = remap(pieces)
		result.history = history
		for i := range result.history {
			result.history[i].Before.Pieces = remap(history[i].Before.Pieces)
			result.history[i].After.Pieces = remap(history[i].After.Pieces)
		}
		return result
	}
}
func (b *Buffer) InstallCompaction(c *Compaction) bool {
	if c.version != b.Version || c.generation != b.historyGeneration {
		return false
	}
	if c.skip {
		return true
	}
	b.original, b.added, b.pieces, b.history = "", c.added, c.pieces, c.history
	b.cache, b.valid = "", false
	return true
}
