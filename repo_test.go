package libpack

import (
	"io/ioutil"
	"os"
	"path"
	"testing"
)

func tmpdir(t *testing.T) string {
	dir, err := ioutil.TempDir("", "libpack-test-")
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func tmpRepo(t *testing.T) *Repository {
	tmp := tmpdir(t)
	r, err := Init(tmp, true)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func nukeRepo(r *Repository) {
	dir := r.gr.Path()
	os.RemoveAll(dir)
}

func TestInitNoCreate(t *testing.T) {
	tmp := tmpdir(t)
	defer os.RemoveAll(tmp)
	db, err := Init(tmp, true)
	if err != nil {
		t.Fatal(err)
	}

	if db == nil {
		t.Fatal("db was nil after init")
	}

	db2, err := Init(tmp, false)
	if err != nil {
		t.Fatal(err)
	}

	if db2 == nil {
		t.Fatal("db was nil after init")
	}

	_, err = Init("/nonexistentpath", false)

	if err == nil {
		t.Fatal("Initializing nonexistent path without create=true did not yield an error")
	}
}

func TestInitCreate(t *testing.T) {
	tmp := tmpdir(t)
	defer os.RemoveAll(tmp)

	db, err := Init(tmp, true)
	if err != nil {
		t.Fatal(err)
	}

	if db == nil {
		t.Fatal("db was nil after create=true")
	}

	fi, err := os.Stat(path.Join(tmp, "refs"))
	if err != nil {
		t.Fatal("ForceInit did not create a repository")
	}

	db, err = Init(tmp, true)
	if err != nil {
		t.Fatal(err)
	}

	if db == nil {
		t.Fatal("db was nil after create=true")
	}

	fi2, err := os.Stat(path.Join(tmp, "refs"))
	if err != nil {
		t.Fatal("create=true wiped out the repository!")
	}

	if fi.ModTime() != fi2.ModTime() {
		t.Fatal("create=true created a new repository and should have just opened it")
	}
}

func TestPush(t *testing.T) {
	srcRepo, src := tmpDB(t)
	defer nukeRepo(srcRepo)

	dstRepo, dst := tmpDB(t)
	defer nukeRepo(dstRepo)

	src.Set("foo/bar/baz", "hello world")

	dst.Set("pre-push content in dst", "this should go away")

	if err := srcRepo.Push(dstRepo.gr.Path(), src.Name(), dst.Name()); err != nil {
		t.Fatal(err)
	}

	dst2, err := dstRepo.DB(dst.Name())
	if err != nil {
		t.Fatal(err)
	}

	assert := dst2.Query().AssertEq("foo/bar/baz", "hello world").AssertNotExist("committed-key")
	if _, err := assert.Run(); err != nil {
		t.Fatal(err)
	}
}

// Pull on an empty destination (ref not set)
func TestPullToEmpty(t *testing.T) {
	srcRepo := tmpRepo(t)
	defer nukeRepo(srcRepo)

	src, err := srcRepo.DB("refs/heads/samedb")
	if err != nil {
		t.Fatal(err)
	}

	dstRepo := tmpRepo(t)
	defer nukeRepo(dstRepo)

	dst, err := dstRepo.DB("refs/heads/samedb")
	if err != nil {
		t.Fatal(err)
	}

	src.Set("foo/bar/baz", "hello world")
	src.Mkdir("/etc/something")

	if err := dstRepo.Pull(srcRepo.gr.Path(), "", "samedb"); err != nil {
		t.Fatal(err)
	}

	assert := dst.Query().AssertEq("foo/bar/baz", "hello world")
	if _, err := assert.Run(); err != nil {
		t.Fatal(err)
	}
}
