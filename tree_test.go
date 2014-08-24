package libpack

import (
	"os"
	"testing"

	git "github.com/libgit2/git2go"
)

func tmpRepo(t *testing.T) *git.Repository {
	tmp := tmpdir(t)
	repo, err := git.InitRepository(tmp, true)
	if err != nil {
		t.Fatal(err)
	}
	return repo
}

func nukeRepo(repo *git.Repository) {
	repo.Free()
	os.RemoveAll(repo.Path())
}

func TestEmptyTree(t *testing.T) {
	repo := tmpRepo(t)
	defer nukeRepo(repo)
	empty, err := emptyTree(repo)
	if err != nil {
		t.Fatal(err)
	}
	if empty.String() != "4b825dc642cb6eb9a060e54bf8d69288fbee4904" {
		t.Fatalf("%v", empty)
	}
}

func TestUpdateTree1(t *testing.T) {
	repo := tmpRepo(t)
	defer nukeRepo(repo)
	hello, err := repo.CreateBlobFromBuffer([]byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	emptyId, _ := emptyTree(repo)
	empty, _ := lookupTree(repo, emptyId)
	t1, err := TreeAdd(repo, empty, "foo", hello, true)
	if err != nil {
		t.Fatal(err)
	}
	assertBlobInTree(t, repo, t1, "foo", "hello")

	t2, err := TreeAdd(repo, t1, "subtree", t1.Id(), true)
	if err != nil {
		t.Fatal(err)
	}
	assertBlobInTree(t, repo, t2, "foo", "hello")
	assertBlobInTree(t, repo, t2, "subtree/foo", "hello")

	t3, err := TreeAdd(repo, empty, "subtree/subsubtree", t1.Id(), true)
	if err != nil {
		t.Fatal(err)
	}
	assertBlobInTree(t, repo, t3, "subtree/subsubtree/foo", "hello")

	t4, err := TreeAdd(repo, t2, "/", t3.Id(), true)
	if err != nil {
		t.Fatal(err)
	}
	assertBlobInTree(t, repo, t4, "foo", "hello")
	assertBlobInTree(t, repo, t4, "subtree/foo", "hello")
	assertBlobInTree(t, repo, t4, "subtree/subsubtree/foo", "hello")

	t1b, err := TreeAdd(repo, empty, "bar", hello, true)
	if err != nil {
		t.Fatal(err)
	}

	t2b, err := TreeAdd(repo, t1, "/", t1b.Id(), true)
	if err != nil {
		t.Fatal(err)
	}
	assertBlobInTree(t, repo, t2b, "foo", "hello")
	assertBlobInTree(t, repo, t2b, "bar", "hello")
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
