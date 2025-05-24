# Publish-Subscribe System with Timeout
## Suitable for Sensor Group Polling

This is a thread-safe implementation of a publish-subscribe (pub-sub) system in Go that allows subscribers to wait for events with a timeout mechanism. Particularly useful for sensor data polling scenarios where you need to collect responses from multiple sources within a limited time.

## Key Features

### 1. **Simple API Design**
- Create hub and subscribers with minimal configuration
- Subscribe to events by string keys
- Wait for events with configurable timeout
- Publish events with success/failure return status

### 2. **Timeout Behavior**
- If all subscribed events occur before timeout: immediately returns complete results
- If timeout occurs: returns only events that were received in time
- Non-blocking operation with graceful degradation

### 3. **Event Overwriting**
- If the same key receives multiple events during the timeout period, only the latest event is preserved
- This prevents stale data and ensures subscribers receive the most recent information

### 4. **Publisher Feedback**
- `Publish()` returns `true` if at least one subscriber received the event
- `Publish()` returns `false` if no one is listening to that key
- Allows publishers to know if their messages are being consumed

## Usage Example

```go
// Create pub-sub hub
hub := NewPubSub()
subscriber := hub.NewSubscriber()

// Subscribe to events by string key
subscriber.Subscribe("foo")
subscriber.Subscribe("buz")

// Wait up to 1 second for events
results := subscriber.Wait(time.Second * 1)

// Publish events from other goroutines
hub.Publish("foo", map[string]int{"foo": 90})  // returns true
hub.Publish("foo", map[string]int{"foo": 100}) // returns true (overwrites previous)
hub.Publish("bar", map[string]int{"bar": 50})  // returns false (no subscribers)

// Results will contain: {"foo": {"foo": 100}}
// Note: "buz" is not in results because nothing was published for it
// Note: "foo" contains the latest value (100, not 90)
```

## Architecture Benefits

### **Thread Safety**
- All operations are protected by mutexes to prevent race conditions
- Safe for concurrent use across multiple goroutines
- Proper synchronization using WaitGroups

### **Memory Management**
- Automatic cleanup of unused topics and subscribers
- Prevents memory leaks through timeout-based cleanup
- Graceful shutdown capabilities

### **Error Resilience**
- Panic recovery in goroutines with logging
- Graceful degradation when components are closed
- Warning logs instead of crashes for invalid operations

## Use Cases

1. **Sensor Data Collection**: Poll multiple sensors and collect responses within a time window
2. **Microservice Communication**: Wait for responses from multiple services with timeout
3. **Event Aggregation**: Collect events from various sources before processing
4. **Real-time Monitoring**: Gather status updates from distributed components
5. **Batch Processing**: Collect data items until timeout or completion

This implementation provides a reliable foundation for event-driven architectures where timing and reliability are critical.

## Creating a Subscriber
```go
hub := NewPubSub()
subscriber := hub.NewSubscriber()
```

## Subscribing to Events by Text Key
```go
subscriber.Subscribe("foo")
subscriber.Subscribe("buz")
```

## Waiting Until Timeout
```go
results := subscriber.Wait(time.Second * 1)
```

If all events that the subscriber is subscribed to happen before the timeout, we finish waiting and return the result.

If a timeout occurs, we return only those events that managed to happen.

When publishing events that no one is subscribed to, false is returned, or true if the event was read by at least one subscriber.

If during the timeout period an event with the same key occurred twice or more, the subscriber receives the latest event by time.

## Publishing Events
```go
hub.Publish("foo", map[string]int{"foo":90}) // true
hub.Publish("foo", map[string]int{"foo":100}) // true
hub.Publish("bar", map[string]int{"bar":50}) // false

/*
results:
{"foo":100}
*/
