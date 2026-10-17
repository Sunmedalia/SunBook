package plugin

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

func openPluginFile(source string) (*os.File, error) {
	// Refuse a replaced symlink/FIFO and enforce the limit on actual bytes read,
	// not only the earlier directory listing's potentially stale size.
	in, err := os.OpenFile(source, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	info, err := in.Stat()
	if err != nil {
		in.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() {
		in.Close()
		return nil, errors.New("plugin source changed to a special file")
	}
	return in, nil
}

func copyPluginFile(source, target string, mode os.FileMode, remaining int64) (int64, error) {
	in, err := openPluginFile(source)
	if err != nil {
		return 0, err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode&0777)
	if err != nil {
		return 0, err
	}
	// Creation applies the editor's inherited umask. Restore the package's
	// ordinary mode so executable approval survives installation with umask 077.
	if err := out.Chmod(mode & 0777); err != nil {
		out.Close()
		return 0, err
	}
	n, err := io.Copy(out, io.LimitReader(in, remaining+1))
	closeErr := out.Close()
	if err != nil {
		return n, err
	}
	if n > remaining {
		return n, errors.New("plugin package exceeds 64 MiB")
	}
	return n, closeErr
}

// Approval covers installed code, not just its manifest. Generated state belongs
// in the disposable HOME/TMPDIR so it cannot invalidate package approval.
func packageFingerprint(dir string) (string, error) {
	return fingerprintPackage(dir, nil)
}

func fingerprintPackage(dir string, expectedManifest []byte) (string, error) {
	hash := sha256.New()
	files, entries := 0, 0
	var size int64
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		entries++
		if entries > 2000 {
			return errors.New("plugin package exceeds 2000 filesystem entries")
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() && !info.Mode().IsRegular() {
			return errors.New("plugin package must contain only regular files and directories")
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		mode := info.Mode().Perm() & 0111
		if d.IsDir() {
			mode = 0
		}
		fmt.Fprintf(hash, "%d:%s:%d:%t\n", len(rel), rel, mode, d.IsDir())
		if d.IsDir() {
			return nil
		}
		files++
		if files > 1000 || info.Size() > 64*1024*1024-size {
			return errors.New("plugin package exceeds 1000 files or 64 MiB")
		}
		in, err := openPluginFile(path)
		if err != nil {
			return err
		}
		fileHash := sha256.New()
		n, err := io.Copy(fileHash, io.LimitReader(in, 64*1024*1024-size+1))
		in.Close()
		if err != nil {
			return err
		}
		size += n
		if size > 64*1024*1024 {
			return errors.New("plugin package exceeds 64 MiB")
		}
		if rel == "plugin.json" && expectedManifest != nil {
			expected := sha256.Sum256(expectedManifest)
			if hex.EncodeToString(expected[:]) != hex.EncodeToString(fileHash.Sum(nil)) {
				return errors.New("plugin manifest changed while verifying package; refresh and try again")
			}
		}
		fmt.Fprintf(hash, "%d:%x\n", n, fileHash.Sum(nil))
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
