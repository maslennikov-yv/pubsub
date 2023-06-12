package pubsub

import (
	"math/rand"
	"testing"
	"time"
)

func TestWait(t *testing.T) {
	hub := NewPubSub()

	data := map[string]string{"a": "foo", "b": "bar", "c": "buz"}
	subscriber := hub.NewSubscriber()

	if hub.Publish("", "") == true {
		t.Errorf("Publish returns true without subscribers")
	}
	for key, value := range data {
		subscriber.Subscribe(key)
		subscriber.Subscribe(key)
		go func(key, value string) {
			time.Sleep(time.Millisecond * time.Duration(rand.Intn(100)))
			if !hub.Publish(key, "first") || !hub.Publish(key, value) {
				t.Errorf("Publish returns false with subscribers")
			}
		}(key, value)
	}
	results := subscriber.Wait(time.Second * 1)

	for _, d := range data {
		r := false
		for _, value := range results {
			s, _ := value.(string)
			if s == d {
				r = true
			}
		}
		if !r {
			t.Errorf("Result %s lost", d)
		}
	}

	time.Sleep(time.Millisecond)
}
