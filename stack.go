package libpack

import (
	"fmt"
	"sync"

	git "github.com/libgit2/git2go"
)

type Stack struct {
	rw DB
	ro []DB
	l  sync.RWMutex
}

func NewStack() *Stack {
	return new(Stack)
}

func (s *Stack) SetRW(rw DB) {
	s.l.Lock()
	s.rw = rw
	s.l.Unlock()
}

func (s *Stack) AddRO(ro DB) {
	s.l.Lock()
	s.ro = append(s.ro, ro)
	s.l.Unlock()
}

func (s *Stack) r() []DB {
	if s.rw != nil {
		return append([]DB{s.rw}, s.ro...)
	}
	return s.ro
}

func (s *Stack) Get(key string) (val string, err error) {
	s.l.RLock()
	defer s.l.RUnlock()
	for _, db := range s.r() {
		val, err = db.Get(key)
		if err == nil {
			return
		}
	}
	return "", fmt.Errorf("no such key: %s", key)
}

func (s *Stack) Set(key, value string) error {
	s.l.RLock()
	defer s.l.RUnlock()
	if s.rw == nil {
		return fmt.Errorf("no writeable db")
	}
	return s.rw.Set(key, value)
}

func (s *Stack) List(key string) (children []string, err error) {
	s.l.RLock()
	defer s.l.RUnlock()
	for _, db := range s.r() {
		children, err = db.List(key)
		if err == nil {
			return
		}
	}
	return nil, fmt.Errorf("no such key: %s", key)
}

func (s *Stack) Mkdir(key string) error {
	s.l.RLock()
	defer s.l.RUnlock()
	if s.rw == nil {
		return fmt.Errorf("no writeable db")
	}
	return s.rw.Mkdir(key)
}

func (s *Stack) Commit(msg string) error {
	s.l.RLock()
	defer s.l.RUnlock()
	if s.rw == nil {
		return fmt.Errorf("no writeable db")
	}
	return s.rw.Commit(msg)
}

func (s *Stack) Checkout(dir string) (string, error) {
	s.l.RLock()
	defer s.l.RUnlock()
	for _, db := range s.r() {
		var err error
		dir, err = db.Checkout(dir)
		if err != nil {
			return "", err
		}
	}
	return dir, nil
}

func (s *Stack) Walk(key string, h func(string, git.Object) error) error {
	s.l.RLock()
	defer s.l.RUnlock()
	r := s.r()
	if len(r) == 0 {
		return fmt.Errorf("no DB to walk")
	}
	return r[0].Walk(key, h)
}

func (s *Stack) Scope(scope string) DB {
	s.l.RLock()
	defer s.l.RUnlock()
	child := new(Stack)
	if s.rw != nil {
		child.rw = s.rw.Scope(scope)
	}
	child.ro = make([]DB, 0, len(s.ro))
	for _, ro := range s.ro {
		child.ro = append(child.ro, ro)
	}
	return child
}
