package main

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/dotcloud/docker/vendor/src/code.google.com/p/go/src/pkg/archive/tar"
)

func main() {
	result, err := tar2git(os.Stdin, os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(result)
}

func git(repo, idx string, stdin io.Reader, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"--git-dir", repo}, args...)...)
	if stdin != nil {
		cmd.Stdin = stdin
	}
	if idx != "" {
		cmd.Env = append(cmd.Env, "GIT_INDEX_FILE=" + idx)
	}
	out, err := cmd.Output()
	return string(out), err
}

func gitHashObject(repo string, src io.Reader) (string, error) {
	out, err := git(repo, "", src, "hash-object", "-w", "--stdin")
	if err != nil {
		return "", err
	}
	return strings.Trim(string(out), " \t\r\n"), nil
}

func gitInit(repo string) error {
	_, err := git(repo, "", nil, "init", "--nare", repo)
	return err
}

// tar2git decodes a tar stream from src, then encodes it into a new git commit
// such that the full tar stream can be reconsistuted from the git data alone.
// It retusn hash of the git commit, or an error if any.
func tar2git(src io.Reader, repo string) (hash string, err error) {
	if err := gitInit(repo); err != nil {
		return "", err
	}
	tree := make(map[string]string)
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
		metaHash, err := gitHashObject(repo, metaBlob)
		if err != nil {
			return "", err
		}
		metaDst := path.Join("_fs_meta", fmt.Sprintf("%0x", sha1.Sum([]byte(hdr.Name))))
		fmt.Printf("    ---> storing metadata in %s\n", metaDst)
		tree[metaDst] = metaHash
		// FIXME: git can carry symlinks as well
		if hdr.Typeflag == tar.TypeReg {
			fmt.Printf("[DATA] %s %d bytes\n", hdr.Name, hdr.Size)
			dataHash, err := gitHashObject(repo, tr)
			if err != nil {
				return "", err
			}
			dataDst := path.Join("_fs_data", hdr.Name)
			tree[dataDst] = dataHash
		}
	}
	fmt.Printf("Ready to write %d items to new tree:\n", len(tree))
	for key, hash := range tree {
		fmt.Printf("    %s  %s\n", hash, key)
	}
	return
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
