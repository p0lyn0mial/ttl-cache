package eviction_store

import (
	"container/list"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/util/clock"
)

type keyFunction func(obj interface{}) string

type item struct {
	obj       interface{}
	timestamp time.Time
}

type evictionStore struct {
	store            map[string]*list.Element
	queue            *list.List
	lock             sync.Mutex
	keyFunc          keyFunction
	ttl              time.Duration
	lastEvictionTime time.Time
	clock            clock.Clock
}

func New(keyFunc keyFunction, ttl time.Duration, clock clock.Clock) *evictionStore {
	return &evictionStore{
		keyFunc: keyFunc,
		store:   map[string]*list.Element{},
		queue:   list.New(),
		ttl:     ttl,
		clock:   clock,
	}
}

func (s *evictionStore) Add(obj interface{}) {
	ts := s.clock.Now()
	s.lock.Lock()
	defer s.lock.Unlock()
	defer s.evictLocked(ts)

	key := s.keyFunc(obj)
	if e, ok := s.store[key]; ok {
		e.Value.(*item).timestamp = ts
		s.queue.MoveToFront(e)
		return
	}
	s.store[key] = s.queue.PushFront(&item{obj: obj, timestamp: ts})
}

func (s *evictionStore) Get(key string) interface{} {
	ts := s.clock.Now()
	defer s.evictLocked(ts)

	if e, ok := s.store[key]; ok {
		e.Value.(*item).timestamp = ts
		s.queue.MoveToFront(e)
		return e.Value.(*item).obj
	}

	return nil
}

func (s *evictionStore) evictLocked(timestamp time.Time) {
	if s.lastEvictionTime.Add(s.ttl).After(timestamp) {
		return
	}
	for {
		if s.queue.Len() == 0 {
			break
		}
		e := s.queue.Back()
		if e.Value.(*item).timestamp.Add(s.ttl).After(timestamp) {
			break
		}
		delete(s.store, s.keyFunc(e.Value.(*item).obj))
		s.queue.Remove(e)
	}
	s.lastEvictionTime = timestamp
}
