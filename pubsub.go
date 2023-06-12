package pubsub

import (
	"crypto/md5"
	"encoding/hex"
	"sync"
	"time"
)

type PubSub struct {
	mutex  sync.Mutex
	topics map[string]*Topic
}

func NewPubSub() *PubSub {
	return &PubSub{
		topics: make(map[string]*Topic),
	}
}

type Subscriber struct {
	ps     *PubSub
	wg     sync.WaitGroup
	done   chan interface{}
	topics map[string]*Topic
	result map[string]interface{}
}

type Topic struct {
	ps          *PubSub
	wg          sync.WaitGroup
	done        chan interface{}
	subscribers map[*Subscriber]*Subscriber
}

func (ps *PubSub) NewSubscriber() *Subscriber {
	return &Subscriber{
		ps:     ps,
		done:   make(chan interface{}),
		topics: make(map[string]*Topic),
		result: make(map[string]interface{}),
	}
}

func (s *Subscriber) Subscribe(key string) {
	s.ps.mutex.Lock()
	defer s.ps.mutex.Unlock()

	var topic *Topic
	var exists bool

	if topic, exists = s.ps.topics[key]; !exists {
		topic = &Topic{
			ps:          s.ps,
			done:        make(chan interface{}),
			subscribers: make(map[*Subscriber]*Subscriber),
		}
		s.ps.topics[key] = topic
	}

	if _, already := topic.subscribers[s]; !already {
		topic.subscribers[s] = s
		topic.wg.Add(1)
	}

	if _, already := s.topics[key]; !already {
		s.topics[key] = topic
		s.wg.Add(1)
	}

	if !exists {
		go func(wg sync.WaitGroup, ps *PubSub, key string) {
			topic.wg.Wait()
			ps.mutex.Lock()
			delete(ps.topics, key)
			ps.mutex.Unlock()
		}(topic.wg, s.ps, key)
	}
}

func (ps *PubSub) Publish(key string, payload interface{}) bool {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()

	var topic *Topic
	var exists bool

	if topic, exists = ps.topics[key]; !exists {
		return false
	}

	for _, sub := range topic.subscribers {
		_, already := sub.result[key]
		sub.result[key] = payload
		if !already {
			sub.wg.Done()
		}
	}

	return true
}

func (s *Subscriber) Wait(t time.Duration) map[string]interface{} {
	go func() {
		defer close(s.done)
		s.wg.Wait()
	}()
	select {
	case <-s.done:
	case <-time.After(t):
	}
	s.ps.mutex.Lock()
	for _, topic := range s.topics {
		delete(topic.subscribers, s)
		topic.wg.Done()
	}
	s.ps.mutex.Unlock()
	return s.result
}

func (ps *PubSub) Hash(key string) string {
	hash := md5.Sum([]byte(key))
	return hex.EncodeToString(hash[:])
}
