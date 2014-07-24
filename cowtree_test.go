package libpack

import (
	"os"
	"testing"
)

func TestCowTreeGet(t *testing.T) {
	orig := make(Tree)
	changes := make(Tree)
	cow := NewCowTree(orig, changes)

	orig.SetBlob("/foo/bar/baz/hello", "world")
	orig.SubTree("/sub", true)

	_, err := cow.SubTree("sub", false)
	if err != nil {
		t.Fatal(err)
	}
	if hello, err := cow.GetBlob("/foo/bar/baz/hello"); err != nil || hello != "world" {
		t.Fatalf("%#v %v\n", hello, err)
	}
	changes.SetBlob("/foo/bar/baz/hello", "new world")
	if hello, err := cow.GetBlob("/foo/bar/baz/hello"); err != nil || hello != "new world" {
		t.Fatalf("%#v %v\n", hello, err)
	}
	if _, err := cow.GetBlob("/something/that/doesnt/exist"); !os.IsNotExist(err) {
		t.Fatalf("%#v", cow)
	}
}
