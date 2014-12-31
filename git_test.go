package libpack

import (
	"fmt"
	"os"
	"testing"

	git "github.com/libgit2/git2go"
)

func tmpGitRepo(t *testing.T) *git.Repository {
	tmp := tmpdir(t)
	repo, err := git.InitRepository(tmp, true)
	if err != nil {
		t.Fatal(err)
	}
	return repo
}

func nukeGitRepo(repo *git.Repository) {
	repo.Free()
	os.RemoveAll(repo.Path())
}

func TestGitEmptyTree(t *testing.T) {
	repo := tmpGitRepo(t)
	defer nukeGitRepo(repo)
	empty, err := emptyTree(repo)
	if err != nil {
		t.Fatal(err)
	}
	if empty.String() != "4b825dc642cb6eb9a060e54bf8d69288fbee4904" {
		t.Fatalf("%v", empty)
	}
}

func TestGitUpdateTree1(t *testing.T) {
	repo := tmpGitRepo(t)
	defer nukeGitRepo(repo)
	hello, err := repo.CreateBlobFromBuffer([]byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	emptyId, _ := emptyTree(repo)
	empty, _ := lookupTree(repo, emptyId)
	t1, err := treeAdd(repo, empty, "foo", hello, true)
	if err != nil {
		t.Fatal(err)
	}
	assertBlobInTree(t, repo, t1, "foo", "hello")

	t2, err := treeAdd(repo, t1, "subtree", t1.Id(), true)
	if err != nil {
		t.Fatal(err)
	}
	assertBlobInTree(t, repo, t2, "foo", "hello")
	assertBlobInTree(t, repo, t2, "subtree/foo", "hello")

	t3, err := treeAdd(repo, empty, "subtree/subsubtree", t1.Id(), true)
	if err != nil {
		t.Fatal(err)
	}
	assertBlobInTree(t, repo, t3, "subtree/subsubtree/foo", "hello")

	t4, err := treeAdd(repo, t2, "/", t3.Id(), true)
	if err != nil {
		t.Fatal(err)
	}
	assertBlobInTree(t, repo, t4, "foo", "hello")
	assertBlobInTree(t, repo, t4, "subtree/foo", "hello")
	assertBlobInTree(t, repo, t4, "subtree/subsubtree/foo", "hello")

	t1b, err := treeAdd(repo, empty, "bar", hello, true)
	if err != nil {
		t.Fatal(err)
	}

	t2b, err := treeAdd(repo, t1, "/", t1b.Id(), true)
	if err != nil {
		t.Fatal(err)
	}
	assertBlobInTree(t, repo, t2b, "foo", "hello")
	assertBlobInTree(t, repo, t2b, "bar", "hello")
}

func TestGitDetectNoRefError(t *testing.T) {
	// Crude detection of "reference not found" errors
	goodErr := fmt.Errorf("refs/heads/test: Reference 'refs/heads/test' not found")
	{
		detected := isGitNoRefErr(goodErr)
		if !detected {
			fmt.Errorf("False negative")
		}
	}

	badErr := fmt.Errorf("something completely different")
	{
		detected := isGitNoRefErr(badErr)
		if detected {
			fmt.Errorf("False positive")
		}
	}
}

func assertBlobInTree(t *testing.T, repo *git.Repository, tree *git.Tree, key, value string) {
	e, err := tree.EntryByPath(key)
	if err != nil || e == nil {
		t.Fatalf("No blob at key %v.\n\ttree=%#v\n", key, tree)
	}
	blob, err := lookupBlob(repo, e.Id)
	if err != nil {
		t.Fatalf("No blob at key %v.\\n\terr=%v\n\ttree=%#v\n", key, err, tree)
	}
	if string(blob.Contents()) != value {
		t.Fatalf("blob at key %v != %v.\n\ttree=%#v\n\treal val = %v\n", key, value, tree, string(blob.Contents()))
	}
	blob.Free()
}

func assertBlobNotInTree(t *testing.T, repo *git.Repository, tree *git.Tree, key string) {
	_, err := tree.EntryByPath(key)
	if err == nil {
		t.Fatalf("Key %q still exists in tree", key)
	}
}
