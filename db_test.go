package libpack

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"testing"
)

var (
	// Scope values which should not actually change the scope
	nopScopes = []string{"", "/", "."}
)

func tmpdir(t *testing.T) string {
	dir, err := ioutil.TempDir("", "test-")
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestInit(t *testing.T) {
	var err error
	// Init existing dir
	tmp1 := tmpdir(t)
	defer os.RemoveAll(tmp1)
	_, err = Init(tmp1, "refs/heads/test")
	if err != nil {
		t.Fatal(err)
	}
	// Test that tmp1 is a bare git repo
	if _, err := os.Stat(path.Join(tmp1, "refs")); err != nil {
		t.Fatal(err)
	}

	// Init a non-existing dir
	tmp2 := path.Join(tmp1, "new")
	_, err = Init(tmp2, "refs/heads/test")
	if err != nil {
		t.Fatal(err)
	}
	// Test that tmp2 is a bare git repo
	if _, err := os.Stat(path.Join(tmp2, "refs")); err != nil {
		t.Fatal(err)
	}

	// Init an already-initialized dir
	_, err = Init(tmp2, "refs/heads/test")
	if err != nil {
		t.Fatal(err)
	}
}

func TestScopeNoop(t *testing.T) {
	tmp := tmpdir(t)
	defer os.RemoveAll(tmp)
	root, err := Init(tmp, "refs/heads/test")
	if err != nil {
		t.Fatal(err)
	}
	root.Set("foo/bar", "hello")
	for _, s := range nopScopes {
		scoped := root.Scope(s)
		assertGet(t, scoped, "foo/bar", "hello")
	}
}

func TestScopeSetGet(t *testing.T) {
	tmp := tmpdir(t)
	defer os.RemoveAll(tmp)
	root, err := Init(tmp, "refs/heads/test")
	if err != nil {
		t.Fatal(err)
	}
	scoped := root.Scope("foo/bar")
	scoped.Set("hello", "world")
	assertGet(t, scoped, "hello", "world")
	assertGet(t, root, "foo/bar/hello", "world")
}

func assertGet(t *testing.T, db *DB, key, val string) {
	if v, err := db.Get(key); err != nil {
		t.Fatalf("assert %v=%v db:%#v\n=> %v", key, val, db, err)
	} else if v != val {
		t.Fatalf("assert %v=%v db:%#v\n=> %v=%v", key, val, db, key, v)
	}
}

func TestSetEmpty(t *testing.T) {
	tmp := tmpdir(t)
	defer os.RemoveAll(tmp)
	db, err := Init(tmp, "refs/heads/test")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Set("foo", ""); err != nil {
		t.Fatal(err)
	}
}

func TestList(t *testing.T) {
	tmp := tmpdir(t)
	defer os.RemoveAll(tmp)
	db, err := Init(tmp, "refs/heads/test")
	if err != nil {
		t.Fatal(err)
	}
	db.Set("foo", "bar")
	if db.tree == nil {
		t.Fatalf("%#v\n")
	}
	for _, rootpath := range []string{"", ".", "/", "////", "///."} {
		names, err := db.List(rootpath)
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
		_, err := db.List(wrongpath)
		if err == nil {
			t.Fatalf("should fail: %s", wrongpath)
		}
		if !strings.Contains(err.Error(), "does not exist in the given tree") {
			t.Fatalf("wrong error: %v", err)
		}
	}
}

func TestSetGetSimple(t *testing.T) {
	tmp := tmpdir(t)
	defer os.RemoveAll(tmp)
	db, err := Init(tmp, "refs/heads/test")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Set("foo", "bar"); err != nil {
		t.Fatal(err)
	}
	if key, err := db.Get("foo"); err != nil {
		t.Fatal(err)
	} else if key != "bar" {
		t.Fatalf("%#v", key)
	}
}

func TestSetGetMultiple(t *testing.T) {
	tmp := tmpdir(t)
	defer os.RemoveAll(tmp)
	db, err := Init(tmp, "refs/heads/test")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Set("foo", "bar"); err != nil {
		t.Fatal(err)
	}
	if err := db.Set("ga", "bu"); err != nil {
		t.Fatal(err)
	}
	if key, err := db.Get("foo"); err != nil {
		t.Fatal(err)
	} else if key != "bar" {
		t.Fatalf("%#v", key)
	}
	if key, err := db.Get("ga"); err != nil {
		t.Fatal(err)
	} else if key != "bu" {
		t.Fatalf("%#v", key)
	}
}

func TestSetCommitGet(t *testing.T) {
	tmp := tmpdir(t)
	defer os.RemoveAll(tmp)
	db, err := Init(tmp, "refs/heads/test")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Set("foo", "bar"); err != nil {
		t.Fatal(err)
	}
	if err := db.Set("ga", "bu"); err != nil {
		t.Fatal(err)
	}
	if err := db.Commit("test"); err != nil {
		t.Fatal(err)
	}
	if err := db.Set("ga", "added after commit"); err != nil {
		t.Fatal(err)
	}
	db.Free()
	db, err = Init(tmp, "refs/heads/test")
	if err != nil {
		t.Fatal(err)
	}
	if val, err := db.Get("foo"); err != nil {
		t.Fatal(err)
	} else if val != "bar" {
		t.Fatalf("%#v", val)
	}
	if val, err := db.Get("ga"); err != nil {
		t.Fatal(err)
	} else if val != "bu" {
		t.Fatalf("%#v", val)
	}
}

func TestSetGetNested(t *testing.T) {
	tmp := tmpdir(t)
	defer os.RemoveAll(tmp)
	db, err := Init(tmp, "refs/heads/test")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Set("a/b/c/d/hello", "world"); err != nil {
		t.Fatal(err)
	}
	if key, err := db.Get("a/b/c/d/hello"); err != nil {
		t.Fatal(err)
	} else if key != "world" {
		t.Fatalf("%#v", key)
	}
}

func testSetGet(t *testing.T, refs []string, scopes []string, components ...[]string) {
	for _, ref := range refs {
		tmp := tmpdir(t)
		defer os.RemoveAll(tmp)
		rootdb, err := Init(tmp, ref)
		if err != nil {
			t.Fatal(err)
		}
		for _, scope := range scopes {
			db := rootdb.Scope(scope)
			if len(components) == 0 {
				return
			}
			if len(components) == 1 {
				for _, k := range components[0] {
					if err := db.Set(k, "hello world"); err != nil {
						t.Fatal(err)
					}
				}
				for _, k := range components[0] {
					if v, err := db.Get(k); err != nil {
						t.Fatalf("Get('%s'): %v\n\troot=%#v\n\tscoped=%#v", k, err, rootdb, db)
					} else if v != "hello world" {
						t.Fatal(err)
					}
				}
				return
			}
			// len(components) >= 2
			first := make([]string, 0, len(components[0])*len(components[1]))
			for _, prefix := range components[0] {
				for _, suffix := range components[1] {
					first = append(first, path.Join(prefix, suffix))
				}
			}
			newComponents := append([][]string{first}, components[2:]...)
			testSetGet(t, []string{ref}, []string{scope}, newComponents...)
		}
	}
}

func TestSetGetNestedMultiple1(t *testing.T) {
	testSetGet(t,
		[]string{"refs/heads/test"},
		[]string{""},
		[]string{"foo"}, []string{"1", "2", "3", "4"}, []string{"/a/b/c/d/hello"},
	)
}

func TestSetGetNestedMultiple(t *testing.T) {
	testSetGet(t,
		[]string{"refs/heads/test"},
		[]string{""},
		[]string{"1", "2", "3", "4"}, []string{"/a/b/c/d/hello"},
	)
}

func TestSetGetNestedMultipleScoped(t *testing.T) {
	testSetGet(t,
		[]string{"refs/heads/test"},
		[]string{"0.1"},
		[]string{"1", "2", "3", "4"}, []string{"/a/b/c/d/hello"},
	)
}

func TestMkdir(t *testing.T) {
	tmp := tmpdir(t)
	defer os.RemoveAll(tmp)
	db, err := Init(tmp, "refs/heads/test")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Mkdir("/"); err != nil {
		t.Fatal(err)
	}
	if err := db.Mkdir("something"); err != nil {
		t.Fatal(err)
	}
	if err := db.Mkdir("something"); err != nil {
		t.Fatal(err)
	}
	if err := db.Mkdir("foo/bar"); err != nil {
		t.Fatal(err)
	}
}

func TestCheckout(t *testing.T) {
	tmp := tmpdir(t)
	defer os.RemoveAll(tmp)
	db, err := Init(tmp, "refs/heads/test")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Set("foo/bar/baz", "hello world"); err != nil {
		t.Fatal(err)
	}
	if err := db.Commit("test"); err != nil {
		t.Fatal(err)
	}
	checkoutTmp := tmpdir(t)
	defer os.RemoveAll(checkoutTmp)
	if _, err := db.Checkout(checkoutTmp); err != nil {
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

func TestCheckoutTmp(t *testing.T) {
	tmp := tmpdir(t)
	defer os.RemoveAll(tmp)
	db, err := Init(tmp, "refs/heads/test")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Set("foo/bar/baz", "hello world"); err != nil {
		t.Fatal(err)
	}
	if err := db.Commit("test"); err != nil {
		t.Fatal(err)
	}
	checkoutTmp, err := db.Checkout("")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(checkoutTmp)
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

func TestCheckoutUncommitted(t *testing.T) {
	t.Skip("FIXME: DB.CheckoutUncommitted does not work properly at the moment")
	tmp := tmpdir(t)
	defer os.RemoveAll(tmp)
	db, err := Init(tmp, "refs/heads/test")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Set("foo/bar/baz", "hello world"); err != nil {
		t.Fatal(err)
	}
	if err := db.Commit("test"); err != nil {
		t.Fatal(err)
	}
	checkoutTmp := tmpdir(t)
	if err := db.CheckoutUncommitted(checkoutTmp); err != nil {
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
