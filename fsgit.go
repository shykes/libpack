package main

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	"io"
	_ "io/ioutil"
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
	_, err := git(repo, "", nil, "init", "--bare", repo)
	if err != nil {
		return fmt.Errorf("git init: %v", err)
	}
	return nil
}

// tar2git decodes a tar stream from src, then encodes it into a new git commit
// such that the full tar stream can be reconsistuted from the git data alone.
// It retusn hash of the git commit, or an error if any.
func tar2git(src io.Reader, repo string) (hash string, err error) {
	if err := gitInit(repo); err != nil {
		return "", err
	}

	tree := make(Tree)
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
		if err := tree.Update(metaDst, metaHash); err != nil {
			return "", err
		}
		// FIXME: git can carry symlinks as well
		if hdr.Typeflag == tar.TypeReg {
			fmt.Printf("[DATA] %s %d bytes\n", hdr.Name, hdr.Size)
			dataHash, err := gitHashObject(repo, tr)
			if err != nil {
				return "", err
			}
			dataDst := path.Join("_fs_data", hdr.Name)
			if err := tree.Update(dataDst, dataHash); err != nil {
				return "", err
			}
		}
	}
	tree.Walk(
		func(k string, v Tree) {
			fmt.Printf("[TREE] %40.40s %s\n", "", k)
		},
		func(k, v string) {
			fmt.Printf("[BLOB] %s %s\n", v, k)
		},
	)

	// Commit the new tree
	/*
		idx, err := ioutil.TempFile("", "tmpidx")
		if err != nil {
			return "", err
		}
		for key, hash := range tree {

			_, err := git(repo, idx.Name(), nil, "update-index", "--add", "--cacheinfo")
			if err != nil {
				return "", err
			}
		}
	*/
	return "", nil
}

type Tree map[string]interface{}

func (tree Tree) Walk(onTree func(string, Tree), onString func(string, string)) {
	for k, v := range tree {
		vString, isString := v.(string)
		if isString && onString != nil {
			onString(k, vString)
			continue
		}
		vTree, isTree := v.(Tree)
		if isTree && onTree != nil {
			onTree(k, vTree)
			vTree.Walk(
				func(subkey string, subtree Tree) {
					onTree(path.Join(k, subkey), subtree)
				},
				func(subkey string, subval string) {
					onString(path.Join(k, subkey), subval)
				},
			)
			continue
		}
	}
}

func (tree Tree) Update(key string, val interface{}) error {
	key = path.Clean(key)
	key = strings.TrimLeft(key, "/") // Remove trailing slashes
	base, leaf := path.Split(key)
	if base == "" {
		if valString, ok := val.(string); ok {
			tree[leaf] = valString
			return nil
		}
		valTree, ok := val.(Tree)
		if !ok {
			return fmt.Errorf("value must be a string or subtree")
		}
		if old, exists := tree[leaf]; exists {
			oldTree, isTree := old.(Tree)
			if !isTree {
				return fmt.Errorf("key %s has existing value of unexpected type: %#v", key, old)
			}
			for k, v := range valTree {
				oldTree.Update(k, v)
			}
			return nil
		}
		tree[leaf] = val
		return nil
	}
	subtree := make(Tree)
	subtree[leaf] = val
	return tree.Update(base, subtree)
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
