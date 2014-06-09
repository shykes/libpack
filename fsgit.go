package main

import (
	"crypto/sha1"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"

	"github.com/dotcloud/docker/vendor/src/code.google.com/p/go/src/pkg/archive/tar"
)

func main() {
	result, err := tar2git(os.Stdin, "")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(result)
}

// tar2git decodes a tar stream from src, then encodes it into a new git commit
// such that the full tar stream can be reconsistuted from the git data alone.
// It retusn hash of the git commit, or an error if any.
func tar2git(src io.Reader, repo string) (hash string, err error) {
	// FIXME: write straight to the git object filesystem
	tmp, err := ioutil.TempDir("", "tmp")
	if err != nil {
		return "", err
	}
	hash = tmp
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
		metaDst := path.Join(tmp, "_fs_meta", fmt.Sprintf("%0x", sha1.Sum([]byte(hdr.Name))))
		fmt.Printf("    ---> storing metadata in %s\n", metaDst)
		if err := os.MkdirAll(path.Dir(metaDst), 0700); err != nil {
			return "", err
		}
		metaFile, err := os.OpenFile(metaDst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0700)
		if err != nil {
			return "", err
		}
		metaWriter := tar.NewWriter(metaFile)
		if err := metaWriter.WriteHeader(hdr); err != nil {
			return "", err
		}
		metaWriter.Close()
		// FIXME: git can carry symlinks as well
		if hdr.Typeflag == tar.TypeReg {
			fmt.Printf("[DATA] %s %d bytes\n", hdr.Name, hdr.Size)
			dataDst := path.Join(tmp, "_fs_data", hdr.Name)
			if err := os.MkdirAll(path.Dir(dataDst), 0700); err != nil {
				return "", err
			}
			dataFile, err := os.OpenFile(dataDst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0700)
			if err != nil {
				return "", err
			}
			if _, err := io.Copy(dataFile, tr); err != nil {
				return "", err
			}
		}
	}
	return
}
