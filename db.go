package libpack

import (
	git "github.com/libgit2/git2go"
)

type DB interface {
	Get(string) (string, error)
	Set(string, string) error
	List(string) ([]string, error)
	Mkdir(string) error
	Commit(string) error
	Checkout(string) (string, error)
	Walk(string, func(string, git.Object) error) error
	Scope(string) DB
}
