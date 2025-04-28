package rabbit

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
)

// ExamplePublisher demonstrates how to use the client to publish messages
func ExamplePublisher() {
	// Create a client with default configuration
	client, err := NewClient(nil)
	if err != nil {
		log.Fatalf("Failed to create RabbitMQ client: %v", err)
	}
	defer client.Close()

	// Declare a queue
	queueName := "example_queue"
	_, err = client.DeclareQueue(queueName, true, false, false)
	if err != nil {
		log.Fatalf("Failed to declare queue: %v", err)
	}

	// Publish a message
	message := fmt.Sprintf("Hello, RabbitMQ! Time: %s", time.Now().Format(time.RFC3339))
	err = client.PublishSimple("", queueName, message)
	if err != nil {
		log.Fatalf("Failed to publish message: %v", err)
	}

	log.Infof("Published message: %s", message)
}

// ExamplePublisherWithExchange demonstrates how to use an exchange for publishing
func ExamplePublisherWithExchange() {
	// Create a client with custom configuration
	config := DefaultConfig()
	config.MaxConnections = 2
	config.MaxChannelsPerConn = 5

	client, err := NewClient(config)
	if err != nil {
		log.Fatalf("Failed to create RabbitMQ client: %v", err)
	}
	defer client.Close()

	// Declare an exchange
	exchangeName := "example_exchange"
	err = client.DeclareExchange(exchangeName, "direct", true, false, false)
	if err != nil {
		log.Fatalf("Failed to declare exchange: %v", err)
	}

	// Declare a queue
	queueName := "example_queue"
	_, err = client.DeclareQueue(queueName, true, false, false)
	if err != nil {
		log.Fatalf("Failed to declare queue: %v", err)
	}

	// Bind the queue to the exchange
	err = client.BindQueue(queueName, queueName, exchangeName)
	if err != nil {
		log.Fatalf("Failed to bind queue to exchange: %v", err)
	}

	// Publish messages
	for i := 0; i < 5; i++ {
		message := fmt.Sprintf("Message %d at %s", i, time.Now().Format(time.RFC3339))

		err = client.PublishSimple(exchangeName, queueName, message)
		if err != nil {
			log.Errorf("Failed to publish message %d: %v", i, err)
			continue
		}

		log.Infof("Published message %d: %s", i, message)
		time.Sleep(500 * time.Millisecond)
	}
}

// ExampleConsumer demonstrates how to consume messages
func ExampleConsumer() {
	// Create a client
	client, err := NewClient(nil)
	if err != nil {
		log.Fatalf("Failed to create RabbitMQ client: %v", err)
	}
	defer client.Close()

	// Declare a queue
	queueName := "example_queue"
	_, err = client.DeclareQueue(queueName, true, false, false)
	if err != nil {
		log.Fatalf("Failed to declare queue: %v", err)
	}

	// Create a consumer
	consumer, err := client.NewConsumer(queueName, "example_consumer")
	if err != nil {
		log.Fatalf("Failed to create consumer: %v", err)
	}

	// Setup signal handling for graceful shutdown
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	// WaitGroup to wait for the consumer to finish
	var wg sync.WaitGroup
	wg.Add(1)

	// Start consuming in a goroutine
	go func() {
		defer wg.Done()

		// Define the message processing callback
		messageHandler := func(body []byte) error {
			log.Infof("Received message: %s", string(body))
			// Simulate processing time
			time.Sleep(200 * time.Millisecond)
			return nil
		}

		// Start consuming
		consumer.Consume(messageHandler)
	}()

	// Wait for termination signal
	<-sigs
	log.Info("Received termination signal, shutting down...")

	// Stop the consumer
	if err := consumer.Stop(); err != nil {
		log.Errorf("Error stopping consumer: %v", err)
	}

	// Wait for consumer to finish
	wg.Wait()
	log.Info("Consumer has been shut down gracefully")
}

// ExampleConsumerWithContext demonstrates consuming messages with context control
func ExampleConsumerWithContext() {
	// Create a client
	client, err := NewClient(nil)
	if err != nil {
		log.Fatalf("Failed to create RabbitMQ client: %v", err)
	}
	defer client.Close()

	// Declare an exchange and queue
	exchangeName := "example_exchange"
	queueName := "example_queue"

	err = client.DeclareExchange(exchangeName, "direct", true, false, false)
	if err != nil {
		log.Fatalf("Failed to declare exchange: %v", err)
	}

	_, err = client.DeclareQueue(queueName, true, false, false)
	if err != nil {
		log.Fatalf("Failed to declare queue: %v", err)
	}

	err = client.BindQueue(queueName, queueName, exchangeName)
	if err != nil {
		log.Fatalf("Failed to bind queue to exchange: %v", err)
	}

	// Create a consumer
	consumer, err := client.NewConsumer(queueName, "example_consumer_with_ctx")
	if err != nil {
		log.Fatalf("Failed to create consumer: %v", err)
	}

	// Create a cancellable context
	ctx, cancel := context.WithCancel(context.Background())

	// Setup signal handling for graceful shutdown
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	// WaitGroup to wait for the consumer to finish
	var wg sync.WaitGroup
	wg.Add(1)

	// Start consuming in a goroutine
	go func() {
		defer wg.Done()

		// Define the message processing callback with context
		messageHandler := func(ctx context.Context, body []byte) error {
			log.Infof("Received message: %s", string(body))

			// Use context for cancellation
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(200 * time.Millisecond):
				// Message processed successfully
				return nil
			}
		}

		// Start consuming with context
		consumer.ConsumeWithContext(ctx, messageHandler)
	}()

	// Wait for termination signal
	<-sigs
	log.Info("Received termination signal, shutting down...")

	// Cancel the context to signal stop
	cancel()

	// Also stop the consumer explicitly
	if err := consumer.Stop(); err != nil {
		log.Errorf("Error stopping consumer: %v", err)
	}

	// Wait for consumer to finish
	wg.Wait()
	log.Info("Consumer has been shut down gracefully")
}

// ExamplePublishSubscribe demonstrates a complete publish-subscribe pattern
func ExamplePublishSubscribe() {
	// Create a client
	client, err := NewClient(nil)
	if err != nil {
		log.Fatalf("Failed to create RabbitMQ client: %v", err)
	}
	defer client.Close()

	// Setup exchange, queue and binding
	exchangeName := "pub_sub_exchange"
	queueName := "pub_sub_queue"

	err = client.DeclareExchange(exchangeName, "fanout", true, false, false)
	if err != nil {
		log.Fatalf("Failed to declare exchange: %v", err)
	}

	_, err = client.DeclareQueue(queueName, true, false, false)
	if err != nil {
		log.Fatalf("Failed to declare queue: %v", err)
	}

	err = client.BindQueue(queueName, "", exchangeName) // Empty routing key for fanout
	if err != nil {
		log.Fatalf("Failed to bind queue to exchange: %v", err)
	}

	// Setup signal handling
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	// Create context for shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start consumer
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()

		consumer, err := client.NewConsumer(queueName, "pub_sub_consumer")
		if err != nil {
			log.Fatalf("Failed to create consumer: %v", err)
			return
		}

		messageHandler := func(ctx context.Context, body []byte) error {
			log.Infof("Consumer received: %s", string(body))
			return nil
		}

		consumer.ConsumeWithContext(ctx, messageHandler)
	}()

	// Publish messages periodically
	ticker := time.NewTicker(1 * time.Second)
	counter := 0

	for {
		select {
		case <-ticker.C:
			message := fmt.Sprintf("Broadcast message %d at %s",
				counter, time.Now().Format(time.RFC3339))

			if err := client.PublishSimple(exchangeName, "", message); err != nil {
				log.Errorf("Failed to publish message: %v", err)
			} else {
				log.Infof("Published: %s", message)
			}

			counter++

		case <-sigs:
			log.Info("Shutdown signal received, stopping...")
			ticker.Stop()
			cancel() // Cancel context to stop consumer
			wg.Wait()
			return
		}
	}
}
