package libpack

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strings"
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

func TestTreeList(t *testing.T) {
	var err error
	r, tree := tmpTree(t)
	defer nukeRepo(r)

	if tree, err = tree.Set("foo", "bar"); err != nil {
		t.Fatal(err)
	}
	for _, rootpath := range []string{"", ".", "/", "////", "///."} {
		names, err := tree.List(rootpath)
		if err != nil {
			t.Fatalf("%s: %v", rootpath, err)
		}
		if fmt.Sprintf("%v", names) != "[foo]" {
			t.Fatalf("List(%v) =  %#v", rootpath, names)
		}
	}
	for _, wrongpath := range []string{
		"does-not-exist",
		"sldhfsjkdfhkjsdfh",
		"a/b/c/d",
		"foo/sdfsdf",
	} {
		_, err := tree.List(wrongpath)
		if err == nil {
			t.Fatalf("should fail: %s", wrongpath)
		}
		if !strings.Contains(err.Error(), "does not exist in the given tree") {
			t.Fatalf("wrong error: %v", err)
		}
	}

}

func TestTreePipeline(t *testing.T) {
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

func TestTreeEmpty(t *testing.T) {
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

func TestTreeScopeNoop(t *testing.T) {
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

func TestTreeScope(t *testing.T) {
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

func TestTreeMultiScope(t *testing.T) {
	r := tmpRepo(t)
	defer nukeRepo(r)

	db, err := r.DB("")
	if err != nil {
		t.Fatal(err)
	}

	root, err := db.Query().Set("a/b/c/d", "hello").Run()
	if err != nil {
		t.Fatal(err)
	}
	a, err := root.Scope("a")
	if err != nil {
		t.Fatal(err)
	}
	ab, err := a.Scope("b")
	if err != nil {
		t.Fatal(err)
	}

	var abDump bytes.Buffer
	if err := ab.Dump(&abDump); err != nil {
		t.Fatal(err)
	}
	if s := abDump.String(); s != "c/\nc/d = hello\n" {
		t.Fatalf("%#v.Dump() = |%v|\n", ab, abDump.String())
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

func TestTreeSetGetSimple(t *testing.T) {
	var err error
	r, tree := tmpTree(t)
	defer nukeRepo(r)

	if tree, err = tree.Set("foo", "bar"); err != nil {
		t.Fatal(err)
	}
	if key, err := tree.Get("foo"); err != nil {
		t.Fatal(err)
	} else if key != "bar" {
		t.Fatalf("%#v", key)
	}
}

func TestTreeCheckout(t *testing.T) {
	t.Skip("FIXME: Tree.Checkout does not work properly at the moment.")
	r := tmpRepo(t)
	defer nukeRepo(r)

	db, err := r.DB("")
	if err != nil {
		t.Fatal(err)
	}
	tree, err := db.Query().Set("foo/bar/baz", "hello world").Run()
	if err != nil {
		t.Fatal(err)
	}

	checkoutTmp := tmpdir(t)
	defer os.RemoveAll(checkoutTmp)
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
