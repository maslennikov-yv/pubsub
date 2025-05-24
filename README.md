# PubSub - Thread-Safe Publish-Subscribe System

A high-performance, thread-safe publish-subscribe (pub-sub) implementation in Go that allows subscribers to wait for events with timeout mechanisms. Particularly useful for sensor polling scenarios where you need to collect responses from multiple sources within a limited time window.

[![Go Version](https://img.shields.io/badge/Go-1.21+-blue.svg)](https://golang.org/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Build Status](https://img.shields.io/badge/Build-Passing-brightgreen.svg)]()
[![Coverage](https://img.shields.io/badge/Coverage-95%25-brightgreen.svg)]()

## Table of Contents

- [Features](#features)
- [Quick Start](#quick-start)
- [Installation](#installation)
- [Usage Examples](#usage-examples)
- [API Documentation](#api-documentation)
- [Architecture](#architecture)
- [Development](#development)
- [Testing](#testing)
- [Performance](#performance)
- [Use Cases](#use-cases)
- [Contributing](#contributing)
- [License](#license)
- [Support](#support)

## Features

### 🚀 **Simple API Design**
- Create hubs and subscribers with minimal configuration
- Subscribe to events using string keys
- Wait for events with configurable timeout
- Publish events with success/failure feedback

### ⏱️ **Timeout Behavior**
- **Early completion**: If all subscribed events occur before timeout, returns immediately with complete results
- **Graceful degradation**: If timeout occurs, returns only events that were received in time
- **Non-blocking operations**: All operations are designed to be non-blocking with graceful degradation

### 🔄 **Event Overwriting**
- If the same key receives multiple events during the timeout period, only the latest event is preserved
- Prevents stale data and ensures subscribers receive the most recent information
- Atomic updates ensure data consistency

### 📊 **Publisher Feedback**
- `Publish()` returns `true` if at least one subscriber received the event
- `Publish()` returns `false` if no one is listening to that key
- Enables publishers to know if their messages are being consumed

### 🛡️ **Thread Safety**
- All operations are protected by mutexes to prevent race conditions
- Safe for concurrent use across multiple goroutines
- Proper synchronization using WaitGroups and channels

### 🧹 **Memory Management**
- Automatic cleanup of unused topics and subscribers
- Prevents memory leaks through timeout-based cleanup
- Graceful shutdown capabilities with resource cleanup

## Quick Start

\`\`\`go
package main

import (
"fmt"
"time"
"pubsub"
)

func main() {
// Create a pub-sub hub
hub := pubsub.NewPubSub()
defer hub.Close()

    // Create a subscriber
    subscriber := hub.NewSubscriber()
    
    // Subscribe to events by string key
    subscriber.Subscribe("temperature")
    subscriber.Subscribe("humidity")
    
    // Publish events from other goroutines
    go func() {
        time.Sleep(50 * time.Millisecond)
        hub.Publish("temperature", map[string]interface{}{
            "value": 25.5,
            "unit":  "°C",
        })
        hub.Publish("humidity", map[string]interface{}{
            "value": 60.0,
            "unit":  "%",
        })
    }()
    
    // Wait up to 1 second for events
    results := subscriber.Wait(time.Second)
    
    fmt.Printf("Received %d events: %v\\n", len(results), results)
    // Output: Received 2 events: map[temperature:map[value:25.5 unit:°C] humidity:map[value:60.0 unit:%]]
}
\`\`\`

## Installation

### Prerequisites

- Go 1.21 or higher
- Git

### Install from Source

\`\`\`bash
git clone https://github.com/maslennikov-yv/pubsub.git
cd pubsub
go mod download
\`\`\`

### Use as Module

\`\`\`bash
go mod init your-project
go get github.com/maslennikov-yv/pubsub
\`\`\`

Then import in your Go code:

\`\`\`go
import "github.com/maslennikov-yv/pubsub"
\`\`\`

## Usage Examples

### Basic Sensor Data Collection

\`\`\`go
func collectSensorData() {
hub := pubsub.NewPubSub()
defer hub.Close()

    subscriber := hub.NewSubscriber()
    subscriber.Subscribe("temperature")
    subscriber.Subscribe("pressure")
    subscriber.Subscribe("humidity")
    
    // Simulate sensors publishing data
    go func() {
        time.Sleep(10 * time.Millisecond)
        hub.Publish("temperature", 23.5)
        time.Sleep(20 * time.Millisecond)
        hub.Publish("pressure", 1013.25)
        // humidity sensor doesn't respond
    }()
    
    // Wait 100ms for all sensors
    results := subscriber.Wait(100 * time.Millisecond)
    
    // Process available data
    for sensor, value := range results {
        fmt.Printf("Sensor %s: %v\\n", sensor, value)
    }
    // Output:
    // Sensor temperature: 23.5
    // Sensor pressure: 1013.25
}
\`\`\`

### Event Overwriting Demonstration

\`\`\`go
func demonstrateOverwriting() {
hub := pubsub.NewPubSub()
defer hub.Close()

    subscriber := hub.NewSubscriber()
    subscriber.Subscribe("stock_price")
    
    go func() {
        // Rapid price updates
        hub.Publish("stock_price", 100.50)  // Initial price
        hub.Publish("stock_price", 100.75)  // Price update
        hub.Publish("stock_price", 101.00)  // Latest price
    }()
    
    results := subscriber.Wait(50 * time.Millisecond)
    
    // Only the latest price is preserved
    fmt.Printf("Latest stock price: %v\\n", results["stock_price"])
    // Output: Latest stock price: 101.00 (or any of the published values)
}
\`\`\`

### Microservice Communication

\`\`\`go
func microserviceAggregation() {
hub := pubsub.NewPubSub()
defer hub.Close()

    subscriber := hub.NewSubscriber()
    subscriber.Subscribe("user_service")
    subscriber.Subscribe("order_service")
    subscriber.Subscribe("inventory_service")
    
    // Simulate microservice responses
    var wg sync.WaitGroup
    services := []string{"user_service", "order_service", "inventory_service"}
    
    for _, service := range services {
        wg.Add(1)
        go func(svc string) {
            defer wg.Done()
            time.Sleep(time.Duration(rand.Intn(80)) * time.Millisecond)
            
            success := hub.Publish(svc, map[string]interface{}{
                "status": "ok",
                "data":   fmt.Sprintf("response from %s", svc),
            })
            
            if !success {
                fmt.Printf("No subscribers for %s\\n", svc)
            }
        }(service)
    }
    
    // Wait for responses with 100ms timeout
    results := subscriber.Wait(100 * time.Millisecond)
    
    fmt.Printf("Received responses from %d services\\n", len(results))
    for service, response := range results {
        fmt.Printf("%s: %v\\n", service, response)
    }
}
\`\`\`

### Real-time Monitoring Dashboard

\`\`\`go
func monitoringDashboard() {
hub := pubsub.NewPubSub()
defer hub.Close()

    // Multiple subscribers for different dashboard components
    systemSubscriber := hub.NewSubscriber()
    systemSubscriber.Subscribe("cpu_usage")
    systemSubscriber.Subscribe("memory_usage")
    systemSubscriber.Subscribe("disk_usage")
    
    networkSubscriber := hub.NewSubscriber()
    networkSubscriber.Subscribe("network_in")
    networkSubscriber.Subscribe("network_out")
    
    // Simulate metric collection
    go func() {
        metrics := map[string]interface{}{
            "cpu_usage":    75.5,
            "memory_usage": 82.3,
            "disk_usage":   45.1,
            "network_in":   1024000,
            "network_out":  512000,
        }
        
        for metric, value := range metrics {
            time.Sleep(10 * time.Millisecond)
            hub.Publish(metric, value)
        }
    }()
    
    // Collect system metrics
    systemMetrics := systemSubscriber.Wait(200 * time.Millisecond)
    networkMetrics := networkSubscriber.Wait(200 * time.Millisecond)
    
    fmt.Println("System Metrics:", systemMetrics)
    fmt.Println("Network Metrics:", networkMetrics)
}
\`\`\`

## API Documentation

### Core Types

#### PubSub

The main hub for publish-subscribe operations.

\`\`\`go
type PubSub struct {
// Internal fields are not exported
}
\`\`\`

#### Subscriber

Represents a subscriber that can wait for multiple events.

\`\`\`go
type Subscriber struct {
// Internal fields are not exported
}
\`\`\`

### Methods

#### NewPubSub() *PubSub

Creates a new PubSub instance.

\`\`\`go
hub := pubsub.NewPubSub()
\`\`\`

**Returns:**
- `*PubSub`: A new PubSub instance

#### (*PubSub) NewSubscriber() *Subscriber

Creates a new subscriber for this PubSub instance.

\`\`\`go
subscriber := hub.NewSubscriber()
\`\`\`

**Returns:**
- `*Subscriber`: A new subscriber instance, or `nil` if PubSub is closed

#### (*PubSub) Publish(key string, payload interface{}) bool

Publishes a message to all subscribers of the specified topic.

\`\`\`go
success := hub.Publish("sensor_data", map[string]float64{"temperature": 25.5})
\`\`\`

**Parameters:**
- `key`: Topic key to publish to
- `payload`: Data to publish (can be any type)

**Returns:**
- `bool`: `true` if at least one subscriber received the message, `false` otherwise

#### (*PubSub) Close() error

Gracefully shuts down the PubSub system.

\`\`\`go
err := hub.Close()
if err != nil {
log.Printf("Error during shutdown: %v", err)
}
\`\`\`

**Returns:**
- `error`: Error if cleanup timeout occurred, `nil` otherwise

#### (*PubSub) IsClosed() bool

Returns whether the PubSub instance has been closed.

\`\`\`go
if hub.IsClosed() {
fmt.Println("PubSub is closed")
}
\`\`\`

**Returns:**
- `bool`: `true` if closed, `false` otherwise

#### (*Subscriber) Subscribe(key string)

Subscribes to events for the specified topic key.

\`\`\`go
subscriber.Subscribe("temperature")
subscriber.Subscribe("humidity")
\`\`\`

**Parameters:**
- `key`: Topic key to subscribe to

#### (*Subscriber) Wait(timeout time.Duration) map[string]interface{}

Waits for subscribed events with the specified timeout.

\`\`\`go
results := subscriber.Wait(100 * time.Millisecond)
\`\`\`

**Parameters:**
- `timeout`: Maximum time to wait for events

**Returns:**
- `map[string]interface{}`: Map of topic keys to received payloads

**Behavior:**
- Returns immediately if all subscribed events are received
- Returns partial results if timeout occurs
- Returns empty map if no events received or timeout is zero

#### (*PubSub) Hash(key string) string

Generates an MD5 hash of the provided key.

\`\`\`go
hash := hub.Hash("my-topic-key")
\`\`\`

**Parameters:**
- `key`: String to hash

**Returns:**
- `string`: MD5 hash as hexadecimal string

### Utility Methods (Testing)

#### (*PubSub) GetTopicCount() int

Returns the number of active topics (primarily for testing).

#### (*PubSub) GetSubscriberCount(key string) int

Returns the number of subscribers for a specific topic (primarily for testing).

## Architecture

### System Overview

\`\`\`
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Publisher 1   │    │   Publisher 2   │    │   Publisher N   │
└─────────┬───────┘    └─────────┬───────┘    └─────────┬───────┘
│                      │                      │
│              Publish(key, payload)          │
└──────────────────────┼──────────────────────┘
│
┌─────────────▼─────────────┐
│        PubSub Hub         │
│  ┌─────────────────────┐  │
│  │   Topic Registry    │  │
│  │ ┌─────┐ ┌─────────┐ │  │
│  │ │Topic│ │Subscriber│ │  │
│  │ │  A  │ │   List   │ │  │
│  │ └─────┘ └─────────┘ │  │
│  │ ┌─────┐ ┌─────────┐ │  │
│  │ │Topic│ │Subscriber│ │  │
│  │ │  B  │ │   List   │ │  │
│  │ └─────┘ └─────────┘ │  │
│  └─────────────────────┘  │
└─────────────┬─────────────┘
│
Message Delivery
│
┌───────────────────────┼───────────────────────┐
│                       │                       │
┌─────────▼───────┐    ┌─────────▼───────┐    ┌─────────▼───────┐
│  Subscriber 1   │    │  Subscriber 2   │    │  Subscriber N   │
│ ┌─────────────┐ │    │ ┌─────────────┐ │    │ ┌─────────────┐ │
│ │Subscriptions│ │    │ │Subscriptions│ │    │ │Subscriptions│ │
│ │   [A, B]    │ │    │ │   [B, C]    │ │    │ │   [A, C]    │ │
│ └─────────────┘ │    │ └─────────────┘ │    │ └─────────────┘ │
│ ┌─────────────┐ │    │ ┌─────────────┐ │    │ ┌─────────────┐ │
│ │   Results   │ │    │ │   Results   │ │    │ │    Map      │ │
│ └─────────────┘ │    │ └─────────────┘ │    │ └─────────────┘ │
└─────────────────┘    └─────────────────┘    └─────────────────┘
\`\`\`

### Key Components

#### 1. **PubSub Hub**
- Central coordinator for all publish-subscribe operations
- Maintains topic registry and subscriber mappings
- Handles graceful shutdown and resource cleanup
- Thread-safe operations with mutex protection

#### 2. **Topic Registry**
- Dynamic topic creation and management
- Automatic cleanup of empty topics
- Efficient subscriber lookup and message routing

#### 3. **Subscriber Management**
- Individual subscriber state tracking
- Timeout-based waiting with early completion
- Automatic cleanup after completion or timeout

#### 4. **Message Delivery System**
- Concurrent message delivery to multiple subscribers
- Event overwriting for latest-value semantics
- Publisher feedback for delivery confirmation

### Concurrency Model

The system uses several concurrency patterns:

- **Mutex-based synchronization** for shared state protection
- **Channel-based signaling** for timeout and shutdown coordination
- **WaitGroup synchronization** for graceful cleanup
- **Goroutine-safe operations** throughout the API

### Memory Management

- **Automatic cleanup** of completed subscribers
- **Topic garbage collection** when no subscribers remain
- **Resource pooling** for efficient memory usage
- **Graceful shutdown** with timeout-based cleanup

## Development

### Prerequisites

- Go 1.21 or higher
- Make (optional, for using Makefile)
- Git

### Development Setup

1. **Clone the repository:**
   \`\`\`bash
   git clone https://github.com/maslennikov-yv/pubsub.git
   cd pubsub
   \`\`\`

2. **Install dependencies:**
   \`\`\`bash
   go mod download
   \`\`\`

3. **Install development tools:**
   \`\`\`bash
   make tools
   \`\`\`

### Build Commands

\`\`\`bash
# Build the project
make build

# Build example binary
make build-example

# Run the example
make run

# Development workflow (clean, format, vet, test, run)
make dev
\`\`\`

### Code Quality

\`\`\`bash
# Format code
make fmt

# Run static analysis
make vet

# Run linter
make lint

# Run all quality checks
make check
\`\`\`

### Project Structure

\`\`\`
pubsub/
├── README.md              # This file
├── README_RU.md          # Russian documentation
├── go.mod                # Go module definition
├── go.sum                # Go module checksums
├── Makefile              # Build automation
├── Dockerfile            # Container build
├── pubsub.go             # Main implementation
├── pubsub_test.go        # Comprehensive tests
├── readme_test.go        # README example tests
└── example/
└── main.go           # Usage example
\`\`\`

## Testing

### Running Tests

\`\`\`bash
# Run all tests with race detection
make test

# Run tests without race detection (faster)
make test-fast

# Run tests with coverage
make coverage

# View coverage in browser
make coverage
open coverage.html
\`\`\`

### Test Categories

#### Unit Tests
- Core functionality testing
- Edge case validation
- Error condition handling

#### Integration Tests
- Multi-subscriber scenarios
- Concurrent operation testing
- Resource cleanup validation

#### Performance Tests
- Benchmark suite for critical paths
- Memory usage profiling
- Concurrency stress testing

#### README Tests
- Validation of documentation examples
- API usage verification
- Behavior confirmation

### Test Coverage

Current test coverage: **95%+**

Coverage includes:
- All public API methods
- Concurrent operation scenarios
- Error conditions and edge cases
- Resource cleanup and shutdown
- Performance characteristics

## Performance

### Benchmarks

\`\`\`bash
# Run all benchmarks
make bench

# Example benchmark results (on modern hardware):
BenchmarkPublishSubscribe-8      	   50000	     25000 ns/op	    1024 B/op	      15 allocs/op
BenchmarkConcurrentPublish-8     	  100000	     12000 ns/op	     512 B/op	       8 allocs/op
BenchmarkHash-8                  	 1000000	      1200 ns/op	      32 B/op	       1 allocs/op
\`\`\`

### Performance Characteristics

- **Low latency**: Sub-millisecond message delivery
- **High throughput**: Thousands of operations per second
- **Memory efficient**: Minimal allocation overhead
- **Scalable**: Linear performance with subscriber count

### Optimization Tips

1. **Reuse subscribers** when possible to reduce allocation overhead
2. **Use appropriate timeouts** to balance responsiveness and resource usage
3. **Batch operations** when publishing multiple events
4. **Monitor topic count** to prevent memory leaks

## Use Cases

### 1. **Sensor Data Collection**
Perfect for IoT scenarios where you need to collect data from multiple sensors within a time window.

\`\`\`go
// Collect temperature readings from multiple sensors
subscriber.Subscribe("sensor_1")
subscriber.Subscribe("sensor_2")
subscriber.Subscribe("sensor_3")
results := subscriber.Wait(500 * time.Millisecond)
\`\`\`

### 2. **Microservice Communication**
Aggregate responses from multiple microservices with timeout handling.

\`\`\`go
// Wait for responses from user, order, and inventory services
subscriber.Subscribe("user_service")
subscriber.Subscribe("order_service")
subscriber.Subscribe("inventory_service")
responses := subscriber.Wait(2 * time.Second)
\`\`\`

### 3. **Real-time Event Aggregation**
Collect events from various sources before processing.

\`\`\`go
// Aggregate trading events
subscriber.Subscribe("trade_executed")
subscriber.Subscribe("price_updated")
subscriber.Subscribe("volume_changed")
events := subscriber.Wait(100 * time.Millisecond)
\`\`\`

### 4. **Distributed System Monitoring**
Monitor health and status from distributed components.

\`\`\`go
// Health check aggregation
subscriber.Subscribe("database_health")
subscriber.Subscribe("cache_health")
subscriber.Subscribe("api_health")
healthStatus := subscriber.Wait(1 * time.Second)
\`\`\`

### 5. **Batch Processing**
Collect data items until timeout or completion.

\`\`\`go
// Batch processing with timeout
subscriber.Subscribe("data_item")
subscriber.Subscribe("processing_complete")
batch := subscriber.Wait(5 * time.Second)
\`\`\`

## Contributing

We welcome contributions! Please follow these guidelines:

### Getting Started

1. **Fork the repository** on GitHub
2. **Clone your fork** locally
3. **Create a feature branch** from `main`
4. **Make your changes** following our coding standards
5. **Add tests** for new functionality
6. **Run the test suite** to ensure everything works
7. **Submit a pull request** with a clear description

### Coding Standards

- **Follow Go conventions** (gofmt, golint, go vet)
- **Write comprehensive tests** for new features
- **Document public APIs** with clear comments
- **Use meaningful variable names** and function names
- **Keep functions focused** and reasonably sized

### Pull Request Process

1. **Update documentation** if you change APIs
2. **Add tests** that cover your changes
3. **Ensure all tests pass** including race detection
4. **Update CHANGELOG.md** with your changes
5. **Request review** from maintainers

### Code Review Guidelines

- **Be respectful** and constructive in feedback
- **Focus on code quality** and maintainability
- **Suggest improvements** rather than just pointing out problems
- **Test thoroughly** before approving changes

### Development Workflow

\`\`\`bash
# 1. Create feature branch
git checkout -b feature/your-feature-name

# 2. Make changes and test
make dev

# 3. Run full test suite
make ci

# 4. Commit and push
git commit -m "Add your feature description"
git push origin feature/your-feature-name

# 5. Create pull request on GitHub
\`\`\`

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

\`\`\`
MIT License

Copyright (c) 2024 PubSub Contributors

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
\`\`\`

## Support

### Getting Help

- **Documentation**: Check this README and code comments
- **Examples**: See the `example/` directory for usage examples
- **Issues**: Report bugs or request features on [GitHub Issues](https://github.com/maslennikov-yv/pubsub/issues)
- **Discussions**: Join conversations on [GitHub Discussions](https://github.com/maslennikov-yv/pubsub/discussions)

### Reporting Issues

When reporting issues, please include:

1. **Go version** and operating system
2. **Minimal code example** that reproduces the issue
3. **Expected behavior** vs actual behavior
4. **Error messages** or logs if applicable
5. **Steps to reproduce** the issue

### Feature Requests

We welcome feature requests! Please:

1. **Check existing issues** to avoid duplicates
2. **Describe the use case** and motivation
3. **Provide examples** of how the feature would be used
4. **Consider implementation** complexity and impact

### Community

- **GitHub**: [https://github.com/maslennikov-yv/pubsub](https://github.com/maslennikov-yv/pubsub)
- **Issues**: [https://github.com/maslennikov-yv/pubsub/issues](https://github.com/maslennikov-yv/pubsub/issues)
- **Discussions**: [https://github.com/maslennikov-yv/pubsub/discussions](https://github.com/maslennikov-yv/pubsub/discussions)

### Maintainers

- **Primary Maintainer**: [@maslennikov-yv](https://github.com/maslennikov-yv)
- **Contributors**: See [CONTRIBUTORS.md](CONTRIBUTORS.md)

---

**Built with ❤️ in Go**

This implementation provides a robust foundation for event-driven architectures where timing and reliability are critical. Whether you're building IoT systems, microservice architectures, or real-time monitoring solutions, this PubSub system offers the performance and reliability you need.
