package libpack

import (
	"os"
	"strings"
	"testing"
)

func TestUpdateTreeString(t *testing.T) {
	tree := make(Tree)
	tree.Update("foo", "bar")
	if tree["foo"] != "bar" {
		t.Fatalf("%#v", tree)
	}
}

func TestUpdateTree1level(t *testing.T) {
	keyVariations := []string{
		"foo/bar",
		"./foo/bar",
		"./foo/bar/",
		"foo///bar////",
		"foo/////bar",
		"/foo/bar",
		"////foo////bar/",
		"foo/bar////////",
	}
	for _, key := range keyVariations {
		tree := make(Tree)
		tree.Update(key, "hello")
		foo := tree["foo"].(Tree)
		bar := foo["bar"].(string)
		if bar != "hello" {
			t.Fatalf("%#v", tree)
		}
	}
}

func TestUpdateTree2levels(t *testing.T) {
	keyVariations := []string{
		"foo/bar/baz",
		"./foo/bar/baz",
		"./foo/bar/baz/",
		"foo///bar////baz/////",
		"foo/////bar//////baz",
		"/foo/bar/baz",
		"////foo////bar/baz/",
		"foo/bar////////baz//////",
	}
	for _, key := range keyVariations {
		tree := make(Tree)
		tree.Update(key, "hello world")
		tree.Update(strings.Replace(key, "baz", "second", 1), "hello again")
		foo := tree["foo"].(Tree)
		bar := foo["bar"].(Tree)
		if baz := bar["baz"].(string); baz != "hello world" {
			t.Fatalf("%#v", tree)
		}
		if second := bar["second"].(string); second != "hello again" {
			t.Fatalf("%#v", tree)
		}
	}
}

func TestPathParts(t *testing.T) {
	if parts := pathParts(""); len(parts) != 0 {
		t.Fatalf("%#v", parts)
	}
	if parts := pathParts("/"); len(parts) != 0 {
		t.Fatalf("%#v", parts)
	}
}

func TestSubTree(t *testing.T) {
	tree := make(Tree)
	var (
		subtree Tree
		err     error
	)
	tree["marker"] = "root"
	// SubTree("") == self
	if subtree, err := tree.SubTree("", false); err != nil || subtree["marker"].(string) != "root" {
		t.Fatalf("%#v %v", subtree, err)
	}
	// SubTree("/") == self
	if subtree, err := tree.SubTree("/", false); err != nil || subtree["marker"].(string) != "root" {
		t.Fatalf("%#v %v", subtree, err)
	}
	// SubTree with create=False
	subtree, err = tree.SubTree("/foo/bar/baz", false)
	if !os.IsNotExist(err) {
		t.Fatalf("%#v", err)
	}
	// SubTree with create=True
	subtree, err = tree.SubTree("/foo/bar/baz", true)
	if err != nil {
		t.Fatal(err)
	}
	subtree["hello"] = "world"
	if hello, err := tree.GetBlob("foo/bar/baz/hello"); err != nil {
		t.Fatal(err)
	} else if hello != "world" {
		t.Fatalf("%#v", tree)
	}
}

func TestGetBlob(t *testing.T) {
	tree := make(Tree)
	if _, err := tree.GetBlob("something/that/does/not/exist"); !os.IsNotExist(err) {
		t.Fatal(err)
	}
	tree["foo"] = "bar"
	if foo, err := tree.GetBlob("foo"); err != nil || foo != "bar" {
		t.Fatalf("%#v %v", tree, err)
	}
}

func TestSetBlob(t *testing.T) {
	tree := make(Tree)
	if err := tree.SetBlob("hello", "world"); err != nil {
		t.Fatal(err)
	}
	if hello, err := tree.GetBlob("hello"); err != nil || hello != "world" {
		t.Fatalf("%#v", tree)
	}
	if err := tree.SetBlob("in/a/subtree/hello", "world"); err != nil {
		t.Fatal(err)
	}
	if hello, err := tree.GetBlob("in/a/subtree/hello"); hello != "world" {
		t.Fatal(err)
	}
	if err := tree.SetBlob("/in/a/subtree", "this should not work..."); err == nil {
		t.Fatalf("%#v", tree)
	}
}
