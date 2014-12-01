package libpack

import (
	"bytes"
	"io/ioutil"
	"os"
	"path"
	"testing"

	git "github.com/libgit2/git2go"
)

// tmpTree creates a temporary repository, creates
// an empty tree in it, and returns both.
// The caller is responsible for nuking the repository
// with nukeRepo.
func tmpTree(t *testing.T) (*Repository, *Tree) {
	r := tmpRepo(t)
	empty, err := r.EmptyTree()
	if err != nil {
		t.Fatal(err)
	}
	return r, empty
}

func TestPipeline(t *testing.T) {
	r := tmpRepo(t)
	p := NewPipeline(r).Set("foo", "bar").Set("a/b/c", "hello").Mkdir("a/b/c/d")
	if p == nil {
		t.Fatalf("%#v", p)
	}
	out, err := p.Run()
	if err != nil {
		t.Fatal(err)
	}
	e, err := out.EntryByPath("foo")
	if err != nil {
		t.Fatal(err)
	}
	val, err := lookupBlob(r.gr, e.Id)
	if err != nil {
		t.Fatal(err)
	}
	if string(val.Contents()) != "bar" {
		t.Fatalf("%#v", out)
	}
	e, err = out.EntryByPath("a/b/c")
	if err != nil {
		t.Fatalf("%#v", e)
	}
	if e.Type != git.ObjectTree {
		t.Fatalf("%#v", e)
	}
}

func TestEmptyTree(t *testing.T) {
	repo := tmpRepo(t)
	defer nukeRepo(repo)
	empty, err := repo.EmptyTree()
	if err != nil {
		t.Fatal(err)
	}
	if empty.Hash() != "4b825dc642cb6eb9a060e54bf8d69288fbee4904" {
		t.Fatalf("%v", empty)
	}
}

func TestScopeNoop(t *testing.T) {
	r, empty := tmpTree(t)
	defer nukeRepo(r)
	tree1, err := empty.Set("foo/bar", "hello")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range nopScopes {
		scoped, err := tree1.Scope(s)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := scoped.Pipeline().AssertEq("foo/bar", "hello").Run(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestScopeTree(t *testing.T) {
	r := tmpRepo(t)
	defer nukeRepo(r)

	db, err := r.DB("")
	if err != nil {
		t.Fatal(err)
	}

	q := db.Query().Set("a/b/c/d/hello", "world").Scope("a/b/c/d")
	var buf bytes.Buffer
	if _, err := q.Dump(&buf).Run(); err != nil {
		t.Fatal(err)
	}
	if s := buf.String(); s != "hello = world\n" {
		t.Fatalf("%v", s)
	}
}

func TestMultiScope(t *testing.T) {
	r := tmpRepo(t)
	defer nukeRepo(r)

	db, err := r.DB("")
	if err != nil {
		t.Fatal(err)
	}

	a := db.Query().Set("a/b/c/d", "hello").Scope("a")
	ab := a.Scope("b")
	var abDump bytes.Buffer
	ab.Dump(&abDump)
	if s := abDump.String(); s != "c/\nc/d = hello\n" {
		t.Fatalf("%v\n", s)
	}
}

func TestTreeSetEmpty(t *testing.T) {
	r := tmpRepo(t)
	defer nukeRepo(r)

	db, err := r.DB("")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := db.Query().Set("foo", "").Run(); err != nil {
		t.Fatal(err)
	}
}

func TestCheckout(t *testing.T) {
	r := tmpRepo(t)
	defer nukeRepo(r)

	db, err := r.DB("")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := db.Set("foo/bar/baz", "hello world"); err != nil {
		t.Fatal(err)
	}
	checkoutTmp := tmpdir(t)
	defer os.RemoveAll(checkoutTmp)
	tree, err := db.Query().Run()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tree.Checkout(checkoutTmp); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path.Join(checkoutTmp, "foo/bar/baz"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := ioutil.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello world" {
		t.Fatalf("%#v", data)
	}
}
