# RabbitMQ Client for Go

This package provides an improved RabbitMQ client implementation for Go applications. It addresses common issues in RabbitMQ client design, such as connection management, error handling, and resource utilization.

## Features

- **Connection pooling**: Maintains multiple connections to distribute load and improve throughput
- **Channel pooling**: Reuses channels to reduce the overhead of channel creation/destruction
- **Automatic reconnection**: Detects connection failures and automatically reconnects
- **Graceful shutdown**: Provides clean shutdown mechanisms for all resources
- **Context support**: Integrates with Go's context package for timeouts and cancellation
- **Publish confirmations**: Ensures messages are safely delivered to the broker
- **Flexible configuration**: Allows customization of all important parameters

## Basic Usage

### Creating a Client

```go
// Use default configuration
client, err := rabbit.NewClient(nil)
if err != nil {
    log.Fatalf("Failed to create RabbitMQ client: %v", err)
}
defer client.Close()

// Or with custom configuration
config := rabbit.DefaultConfig()
config.URL = "amqp://user:password@rabbitmq-host:5672/"
config.MaxConnections = 5
config.MaxChannelsPerConn = 10
config.ReconnectInterval = 3 * time.Second

client, err := rabbit.NewClient(config)
if err != nil {
    log.Fatalf("Failed to create RabbitMQ client: %v", err)
}
defer client.Close()
```

### Publishing Messages

```go
// Simple publishing directly to a queue
err = client.PublishSimple("", "queue_name", "Hello World!")

// Publishing with more control
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

err = client.Publish(
    ctx,
    "exchange_name",  // Exchange
    "routing_key",    // Routing key
    false,            // Mandatory
    false,            // Immediate
    amqp.Publishing{
        ContentType:  "text/plain",
        Body:         []byte("Hello World!"),
        DeliveryMode: amqp.Persistent,
    },
)
```

### Consuming Messages

```go
// Create a consumer
consumer, err := client.NewConsumer("queue_name", "consumer_tag")
if err != nil {
    log.Fatalf("Failed to create consumer: %v", err)
}

// Define message handler
messageHandler := func(body []byte) error {
    fmt.Printf("Received message: %s\n", string(body))
    return nil
}

// Start consuming (this will block)
consumer.Consume(messageHandler)

// Or with context control
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

messageHandlerWithCtx := func(ctx context.Context, body []byte) error {
    fmt.Printf("Received message: %s\n", string(body))
    return nil
}

consumer.ConsumeWithContext(ctx, messageHandlerWithCtx)
```

### Graceful Shutdown

```go
// Setup signal handling
sigs := make(chan os.Signal, 1)
signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

// Create cancellable context
ctx, cancel := context.WithCancel(context.Background())

// Start consumer in goroutine
go func() {
    consumer, _ := client.NewConsumer("queue_name", "consumer_tag")
    consumer.ConsumeWithContext(ctx, messageHandler)
}()

// Wait for termination signal
<-sigs

// Trigger shutdown
cancel()     // Cancel context
client.Close() // Close client and all its resources
```

## Advanced Features

### Declaring Exchanges and Queues

```go
// Declare an exchange
err = client.DeclareExchange(
    "exchange_name",  // Name
    "direct",         // Type (direct, fanout, topic, headers)
    true,             // Durable
    false,            // Auto-delete
    false,            // Internal
)

// Declare a queue
queue, err := client.DeclareQueue(
    "queue_name",     // Name
    true,             // Durable
    false,            // Auto-delete
    false,            // Exclusive
)

// Bind a queue to an exchange
err = client.BindQueue(
    "queue_name",     // Queue name
    "routing_key",    // Routing key
    "exchange_name",  // Exchange name
)
```

## Design Considerations

This client implementation addresses several common issues with RabbitMQ client design:

1. **Connection management**: The client maintains a pool of connections and distributes channels across them.

2. **Error handling**: All errors are properly propagated, and the client automatically attempts to recover from connection failures.

3. **Resource utilization**: Channels are reused when possible to avoid the overhead of frequent creation/destruction.

4. **Thread safety**: All operations are thread-safe, allowing concurrent use from multiple goroutines.

5. **Graceful shutdown**: The client provides mechanisms for clean shutdown, ensuring all resources are properly released.

## Examples

See `examples.go` for complete working examples showing different usage patterns:

- Basic publishing and consuming
- Working with exchanges
- Using context for cancellation
- Implementing the publish-subscribe pattern
- Handling graceful shutdown

## License

This package is available under the same license as the containing project. 