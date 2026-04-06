package factory

import "sync"

func New() *Singleton {
	return &Singleton{}
}

type Singleton struct {
	instMap sync.Map
}

func (s *Singleton) Get(name string, newFunc func() interface{}) interface{} {
	inst, ok := s.instMap.Load(name)
	if ok {
		return inst
	} else {
		newInst := newFunc()
		actual, loaded := s.instMap.LoadOrStore(name, newInst)
		if loaded {
			return actual
		} else {
			return newInst
		}
	}
}
