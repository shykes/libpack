package libpack

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	"io"
	"os"
	"path"
	"sync"

	gitdb "github.com/docker/libpack/db"
	"github.com/dotcloud/docker/archive"
	git "github.com/libgit2/git2go"

	"github.com/dotcloud/docker/vendor/src/code.google.com/p/go/src/pkg/archive/tar"
)

const (
	MetaTree = "_fs_meta"
	DataTree = "_fs_data"
)

func Pack(repo, dir, branch string) (hash string, err error) {
	a, err := archive.TarWithOptions(dir, &archive.TarOptions{Excludes: []string{".git"}})
	if err != nil {
		return "", err
	}
	return Tar2git(a, repo, branch)
}

func Unpack(repo, dir, hash string) error {
	r, w := io.Pipe()
	var (
		inErr  error
		outErr error
	)
	var tasks sync.WaitGroup
	tasks.Add(2)
	go func() {
		defer tasks.Done()
		inErr = Git2tar(repo, hash, os.Stdout)
		w.Close()
	}()
	go func() {
		defer tasks.Done()
		outErr = archive.Untar(r, dir, &archive.TarOptions{})
	}()
	tasks.Wait()
	if inErr != nil {
		return fmt.Errorf("git2tar: %v", inErr)
	}
	if outErr != nil {
		return fmt.Errorf("untar: %v", outErr)
	}
	return nil
}

// Git2tar looks for a git tree object at `hash` in a git repository at the path
// `repo`, then extracts it as a tar stream written to `dst`.
// The tree is not buffered on disk or in memory before being streamed.
func Git2tar(repo, hash string, dst io.Writer) error {
	tw := tar.NewWriter(dst)
	defer tw.Close()
	db, err := gitdb.Init(repo, hash, "")
	if err != nil {
		return err
	}
	defer db.Free()
	fmt.Fprintf(os.Stderr, "head = %s\n", db.Head().String())
	// Walk the data tree
	return db.Walk(DataTree, func(name string, obj git.Object) error {
		fmt.Fprintf(os.Stderr, "Generating tar entry for '%s'...\n", name)
		metaBlob, err := db.Get(metaPath(name))
		if err != nil {
			return err
		}
		tr := tar.NewReader(bytes.NewReader([]byte(metaBlob)))
		hdr, err := tr.Next()
		if err != nil {
			return err
		}
		// Write the reconstituted tar header+content
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if blob, isBlob := obj.(*git.Blob); isBlob {
			fmt.Fprintf(os.Stderr, "--> writing %d bytes for blob %s\n", hdr.Size, hdr.Name)
			if _, err := tw.Write(blob.Contents()[:hdr.Size]); err != nil {
				return err
			}
		}
		return nil
	})
	return nil
}

// metaPath computes a path at which the metadata can be stored for a given path.
// For example if `name` is "/etc/resolv.conf", the corresponding metapath is
// "_fs_meta/194c1cbe5a8cfcb85c6a46b936da12ffdc32f90f"
// This path will be used to store and retrieve the tar header encoding the metadata
// for the corresponding file.
func metaPath(name string) string {
	name = path.Clean(name)
	// FIXME: this doesn't seem to yield the expected result.
	return path.Join(MetaTree, fmt.Sprintf("%x", sha1.Sum([]byte(name))))
}

// Tar2git decodes a tar stream from src, then encodes it into a new git commit
// such that the full tar stream can be reconsistuted from the git data alone.
// It retusn hash of the git commit, or an error if any.
func Tar2git(src io.Reader, repo, branch string) (hash string, err error) {
	db, err := gitdb.Init(repo, branch, "")
	if err != nil {
		return "", err
	}
	tr := tar.NewReader(src)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		fmt.Printf("[META] %s\n", hdr.Name)
		metaBlob, err := headerReader(hdr)
		if err != nil {
			return "", err
		}
		fmt.Printf("    ---> storing metadata in %s\n", metaPath(hdr.Name))
		if err := db.SetStream(metaPath(hdr.Name), metaBlob); err != nil {
			return "", err
		}
		// FIXME: git can carry symlinks as well
		if hdr.Typeflag == tar.TypeReg {
			fmt.Printf("[DATA] %s %d bytes\n", hdr.Name, hdr.Size)
			if err := db.SetStream(path.Join("_fs_data", hdr.Name), tr); err != nil {
				return "", err
			}
		}
	}
	if err := db.Commit("imported tar filesystem tree"); err != nil {
		return "", err
	}
	if head := db.Head(); head != nil {
		return head.String(), nil
	}
	return "", nil
}

func headerReader(hdr *tar.Header) (io.Reader, error) {
	var buf bytes.Buffer
	w := tar.NewWriter(&buf)
	defer w.Close()
	if err := w.WriteHeader(hdr); err != nil {
		return nil, err
	}
	return &buf, nil
}
