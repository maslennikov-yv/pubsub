# Timeout-Based Publish/Subscribe System
## Ideal for Sensor Polling and Distributed Data Collection

This is a lightweight, thread-safe publish/subscribe (pub-sub) system written in Go that enables efficient collection of data from multiple sources within specified time limits. The system is particularly well-suited for scenarios where you need to gather responses from distributed sensors, microservices, or any data sources that may respond at different times.

## What This System Does

The pub-sub system allows you to:
- **Subscribe** to specific data channels using simple string keys
- **Wait** for data with configurable timeouts
- **Publish** data to subscribers with immediate feedback
- **Collect** partial results even when some sources don't respond in time

Think of it as a smart mailbox system where you can wait for specific letters (data) from different senders (sources), but you don't have to wait forever - you get whatever arrives within your specified time window.

## Key Benefits

### 🚀 **Simple and Intuitive API**
- Create a hub and subscribers with just a few lines of code
- No complex configuration or setup required
- Clean, readable code that's easy to maintain

### ⏱️ **Intelligent Timeout Handling**
- Set custom timeout periods for data collection
- Get immediate results when all expected data arrives early
- Receive partial results when timeout occurs - no data loss
- Non-blocking operations that won't freeze your application

### 🔄 **Smart Data Management**
- Automatically keeps only the latest data when multiple updates occur
- Prevents stale information from affecting your results
- Built-in memory management prevents resource leaks

### 🛡️ **Production-Ready Reliability**
- Thread-safe for concurrent use across multiple goroutines
- Graceful error handling and recovery
- Proper resource cleanup and shutdown procedures

## Real-World Use Cases

### 🌡️ **IoT Sensor Networks**
Poll temperature, humidity, and pressure sensors across a building. Get readings from all available sensors within 5 seconds, even if some sensors are temporarily offline.

### 🔧 **Microservice Orchestration**
Coordinate responses from multiple microservices. Wait for user data, inventory status, and payment processing, but proceed with available information if any service is slow.

### 📊 **Real-Time Monitoring**
Collect status updates from distributed system components. Gather health checks from all servers within a time window for dashboard updates.

### 📦 **Batch Processing**
Accumulate data items from various sources until either all expected items arrive or a deadline is reached, then process the collected batch.

## How It Works

```go
// 1. Create the system
hub := NewPubSub()
subscriber := hub.NewSubscriber()

// 2. Subscribe to data channels
subscriber.Subscribe("temperature")
subscriber.Subscribe("humidity")

// 3. Wait for data (with timeout)
results := subscriber.Wait(time.Second * 5)

// 4. Publish data from other parts of your application
hub.Publish("temperature", 23.5)  // Returns true (someone listening)
hub.Publish("humidity", 65.2)     // Returns true (someone listening)
hub.Publish("pressure", 1013.25)  // Returns false (no subscribers)
```

## Architecture Advantages

### **Memory Efficient**
- Automatic cleanup of unused channels and subscribers
- No memory leaks even with long-running applications
- Minimal resource overhead

### **Fault Tolerant**
- Continues working even if individual components fail
- Graceful degradation when timeouts occur
- Warning logs instead of application crashes

### **Scalable Design**
- Supports unlimited number of subscribers and publishers
- Efficient concurrent operations
- Built-in performance monitoring capabilities

## Perfect For

- **IoT Applications**: Collecting sensor data with reliability requirements
- **Distributed Systems**: Coordinating responses from multiple services
- **Real-Time Analytics**: Gathering data streams with time constraints
- **Event-Driven Architecture**: Building responsive, loosely-coupled systems
- **Data Aggregation**: Combining information from various sources efficiently

This implementation provides a robust foundation for any application that needs to collect data from multiple sources within time constraints, ensuring your system remains responsive and reliable even when dealing with unpredictable data sources.

## Quick Start Example

```go
// Create the pub-sub hub
hub := NewPubSub()
subscriber := hub.NewSubscriber()

// Subscribe to events by string key
subscriber.Subscribe("sensor1")
subscriber.Subscribe("sensor2")

// Wait up to 1 second for data
results := subscriber.Wait(time.Second * 1)

// Publish data from other goroutines
hub.Publish("sensor1", 25.3)      // Returns true
hub.Publish("sensor2", 68.1)      // Returns true
hub.Publish("sensor3", 42.0)      // Returns false (no subscribers)

// Results contain: {"sensor1": 25.3, "sensor2": 68.1}
// Note: sensor3 data is not included (not subscribed)
```

The system automatically handles all the complex synchronization, timeout management, and resource cleanup, letting you focus on your application logic rather than infrastructure concerns.
