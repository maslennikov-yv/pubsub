# Time-limited PubSub
## Suitable for polling a group of sensors

## Creating a subscriber
```
hub := NewPubSub()
subscriber := hub.NewSubscriber()
```
## Subscribing to events using the text key
```
subscriber.Subscribe("foo")
subscriber.Subscribe("buz")
```
## Waiting until timeout
```
results := subscriber.Wait(time.Second * 1)
```
If all the events that the subscriber is subscribed to happened before the timeout, we end the wait and return the result

If a timeout has occurred, Wait return only those events that managed to occur

When publishing events to which no one subscribed, the false value is returned or true if at least one subscriber has read the event

If an event with the same key occurred twice or more times during the timeout, the latest event is sent to the subscriber

## Publishing events
```
hub.Publish("foo", map[string]int{"foo":90}) // true
hub.Publish("foo", map[string]int{"foo":100}) // true
hub.Publish("bar", map[string]int{"bar":50}) // false

/*
results:
{"foo":100}
*/
```