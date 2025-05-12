package rabbit

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/streadway/amqp"
)

// Config represents all RabbitMQ client configuration options
type Config struct {
	// Connection settings
	URL               string
	Heartbeat         time.Duration // heartbeat interval
	ConnectionTimeout time.Duration // connection timeout

	// Connection pool settings
	MaxConnections     int // maximum number of connections in the pool
	MaxChannelsPerConn int // maximum number of channels per connection

	// Auto recovery settings
	AutoReconnect     bool
	ReconnectInterval time.Duration

	// TLS settings
	EnableTLS bool
	// TLS config can be added here if needed

	// Consumer settings
	PrefetchCount int  // QoS prefetch count for better load distribution
	AutoAck       bool // Whether to use auto acknowledgement mode

	// Auto-create settings
	AutoCreate bool // Whether to auto-create exchange/queue if not exists
}

// DefaultConfig returns a Config with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		URL:                "amqp://guest:guest@localhost:5672/",
		Heartbeat:          10 * time.Second,
		ConnectionTimeout:  30 * time.Second,
		MaxConnections:     3,
		MaxChannelsPerConn: 10,
		AutoReconnect:      true,
		ReconnectInterval:  5 * time.Second,
		EnableTLS:          false,
		PrefetchCount:      3,
		AutoAck:            false,
		AutoCreate:         false,
	}
}

// Client is the RabbitMQ client that manages connections and channels
type Client struct {
	config *Config
	mu     sync.Mutex

	// Connection pool
	connPool []*amqp.Connection

	// Channel management - maps connection to a slice of channels
	chanPool map[*amqp.Connection][]*amqp.Channel

	// Connection status tracking
	connStatus map[*amqp.Connection]bool

	// Shutdown signal
	done chan struct{}

	// Notification channels
	connNotify map[*amqp.Connection]chan *amqp.Error
}

// NewClient creates a new RabbitMQ client with the given configuration
func NewClient(config *Config) (*Client, error) {
	if config == nil {
		config = DefaultConfig()
	}

	client := &Client{
		config:     config,
		chanPool:   make(map[*amqp.Connection][]*amqp.Channel),
		connStatus: make(map[*amqp.Connection]bool),
		connNotify: make(map[*amqp.Connection]chan *amqp.Error),
		done:       make(chan struct{}),
	}

	// Initialize connection pool
	if err := client.initConnectionPool(); err != nil {
		return nil, fmt.Errorf("failed to initialize connection pool: %w", err)
	}

	// Start monitoring connections
	if client.config.AutoReconnect {
		go client.monitorConnections()
	}

	return client, nil
}

// initConnectionPool initializes the connection pool
func (c *Client) initConnectionPool() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i := 0; i < c.config.MaxConnections; i++ {
		conn, err := c.dial()
		if err != nil {
			return err
		}

		c.connPool = append(c.connPool, conn)
		c.chanPool[conn] = make([]*amqp.Channel, 0, c.config.MaxChannelsPerConn)
		c.connStatus[conn] = true

		// Setup notification channel for this connection
		ch := make(chan *amqp.Error)
		conn.NotifyClose(ch)
		c.connNotify[conn] = ch
	}

	return nil
}

// dial creates a new connection to RabbitMQ
func (c *Client) dial() (*amqp.Connection, error) {
	config := amqp.Config{
		Heartbeat: c.config.Heartbeat,
		Dial:      amqp.DefaultDial(c.config.ConnectionTimeout),
	}

	conn, err := amqp.DialConfig(c.config.URL, config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	return conn, nil
}

// isChannelClosed checks if a channel is closed
func isChannelClosed(ch *amqp.Channel) bool {
	if ch == nil {
		return true
	}

	// Try a harmless operation to check if channel is closed
	_, err := ch.QueueInspect("")
	return err != nil
}

// getConnection returns an available connection from the pool
func (c *Client) getConnection() (*amqp.Connection, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Find an active connection
	for _, conn := range c.connPool {
		if !conn.IsClosed() && c.connStatus[conn] {
			return conn, nil
		}
	}

	return nil, errors.New("no available connections")
}

// getChannel returns a channel from the pool or creates a new one
func (c *Client) getChannel() (*amqp.Channel, *amqp.Connection, error) {
	conn, err := c.getConnection()
	if err != nil {
		return nil, nil, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if we have any available channels
	connChannels := c.chanPool[conn]
	if len(connChannels) > 0 {
		// Get last channel in slice
		ch := connChannels[len(connChannels)-1]
		// Remove it from the pool
		c.chanPool[conn] = connChannels[:len(connChannels)-1]

		// Check if channel is still usable
		if !isChannelClosed(ch) {
			return ch, conn, nil
		}
		// If channel is closed, continue and create a new one
	}

	// Create a new channel
	ch, err := conn.Channel()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create channel: %w", err)
	}

	return ch, conn, nil
}

// releaseChannel returns a channel to the pool
func (c *Client) releaseChannel(ch *amqp.Channel, conn *amqp.Connection) {
	if ch == nil || conn == nil || conn.IsClosed() || isChannelClosed(ch) {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.chanPool[conn]) < c.config.MaxChannelsPerConn {
		c.chanPool[conn] = append(c.chanPool[conn], ch)
	} else {
		// Too many channels, just close it
		_ = ch.Close()
	}
}

// monitorConnections monitors connection health and reconnects if necessary
func (c *Client) monitorConnections() {
	for {
		select {
		case <-c.done:
			return
		default:
			for conn, notify := range c.connNotify {
				select {
				case err := <-notify:
					if err != nil {
						log.Warnf("Connection closed with error: %v", err)
						c.mu.Lock()
						c.connStatus[conn] = false
						c.mu.Unlock()

						// Try to reconnect
						go c.reconnect(conn)
					}
				default:
					// Continue to next connection
				}
			}

			// Sleep before next check
			time.Sleep(1 * time.Second)
		}
	}
}

// reconnect attempts to reconnect a failed connection
func (c *Client) reconnect(oldConn *amqp.Connection) {
	for {
		select {
		case <-c.done:
			return
		default:
			log.Info("Attempting to reconnect to RabbitMQ...")

			conn, err := c.dial()
			if err != nil {
				log.Warnf("Failed to reconnect: %v. Retrying in %v...",
					err, c.config.ReconnectInterval)
				time.Sleep(c.config.ReconnectInterval)
				continue
			}

			// Setup notification channel for this connection
			ch := make(chan *amqp.Error)
			conn.NotifyClose(ch)

			c.mu.Lock()
			// Replace old connection with new one in the pool
			for i, connection := range c.connPool {
				if connection == oldConn {
					c.connPool[i] = conn
					break
				}
			}

			// Update connection status and channel maps
			delete(c.connStatus, oldConn)
			delete(c.chanPool, oldConn)
			delete(c.connNotify, oldConn)

			c.connStatus[conn] = true
			c.chanPool[conn] = make([]*amqp.Channel, 0, c.config.MaxChannelsPerConn)
			c.connNotify[conn] = ch
			c.mu.Unlock()

			log.Info("Successfully reconnected to RabbitMQ")
			return
		}
	}
}

// Close closes all connections and channels
func (c *Client) Close() error {
	close(c.done)

	c.mu.Lock()
	defer c.mu.Unlock()

	// Close all channels first
	for conn, channels := range c.chanPool {
		for _, ch := range channels {
			_ = ch.Close()
		}
		delete(c.chanPool, conn)
	}

	// Then close all connections
	for _, conn := range c.connPool {
		_ = conn.Close()
	}

	// Clear all maps and slice
	c.connPool = nil
	c.connStatus = make(map[*amqp.Connection]bool)
	c.connNotify = make(map[*amqp.Connection]chan *amqp.Error)

	return nil
}

// DeclareQueue declares a queue with the given name and settings
func (c *Client) DeclareQueue(queueName string, durable, autoDelete, exclusive bool) (amqp.Queue, error) {
	ch, conn, err := c.getChannel()
	if err != nil {
		return amqp.Queue{}, err
	}
	defer c.releaseChannel(ch, conn)

	// Existence check and auto-create logic
	if !c.config.AutoCreate {
		// Try passive declare (existence check)
		q, err := ch.QueueDeclarePassive(
			queueName,  // name
			durable,    // durable
			autoDelete, // auto-delete
			exclusive,  // exclusive
			false,      // no-wait
			nil,        // arguments
		)
		if err != nil {
			return amqp.Queue{}, fmt.Errorf("queue '%s' does not exist and auto-create is disabled: %w", queueName, err)
		}
		return q, nil
	}

	// Auto-create or declare as usual
	return ch.QueueDeclare(
		queueName,  // name
		durable,    // durable
		autoDelete, // auto-delete
		exclusive,  // exclusive
		false,      // no-wait
		nil,        // arguments
	)
}

// DeclareExchange declares an exchange with the given name and type
func (c *Client) DeclareExchange(exchangeName, exchangeType string, durable, autoDelete, internal bool) error {
	ch, conn, err := c.getChannel()
	if err != nil {
		return err
	}
	defer c.releaseChannel(ch, conn)

	// Existence check and auto-create logic
	if !c.config.AutoCreate {
		// Try passive declare (existence check)
		err := ch.ExchangeDeclarePassive(
			exchangeName, // name
			exchangeType, // type
			durable,      // durable
			autoDelete,   // auto-delete
			internal,     // internal
			false,        // no-wait
			nil,          // arguments
		)
		if err != nil {
			return fmt.Errorf("exchange '%s' does not exist and auto-create is disabled: %w", exchangeName, err)
		}
		return nil
	}

	// Auto-create or declare as usual
	return ch.ExchangeDeclare(
		exchangeName, // name
		exchangeType, // type
		durable,      // durable
		autoDelete,   // auto-delete
		internal,     // internal
		false,        // no-wait
		nil,          // arguments
	)
}

// BindQueue binds a queue to an exchange with a routing key
func (c *Client) BindQueue(queueName, routingKey, exchangeName string) error {
	ch, conn, err := c.getChannel()
	if err != nil {
		return err
	}
	defer c.releaseChannel(ch, conn)

	return ch.QueueBind(
		queueName,    // queue name
		routingKey,   // routing key
		exchangeName, // exchange
		false,        // no-wait
		nil,          // arguments
	)
}

// Publish publishes a message to an exchange with a routing key
func (c *Client) Publish(ctx context.Context, exchange, routingKey string, mandatory, immediate bool, msg amqp.Publishing) error {
	ch, conn, err := c.getChannel()
	if err != nil {
		return err
	}
	defer c.releaseChannel(ch, conn)

	// Set up confirm mode if not already done
	if err := ch.Confirm(false); err != nil {
		return fmt.Errorf("channel could not be put into confirm mode: %w", err)
	}

	confirms := ch.NotifyPublish(make(chan amqp.Confirmation, 1))

	// Publish the message
	if err := ch.Publish(
		exchange,   // exchange
		routingKey, // routing key
		mandatory,  // mandatory
		immediate,  // immediate
		msg,        // message
	); err != nil {
		return fmt.Errorf("failed to publish message: %w", err)
	}

	// Wait for confirmation or context cancellation
	select {
	case confirm := <-confirms:
		if !confirm.Ack {
			return errors.New("message was not confirmed by server")
		}
	case <-ctx.Done():
		return ctx.Err()
	}

	return nil
}

// PublishSimple publishes a plain text message with sensible defaults
func (c *Client) PublishSimple(exchange, routingKey, message string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return c.Publish(
		ctx,
		exchange,
		routingKey,
		false, // mandatory
		false, // immediate
		amqp.Publishing{
			DeliveryMode: amqp.Persistent,
			ContentType:  "text/plain",
			Body:         []byte(message),
			Timestamp:    time.Now(),
		},
	)
}

// Consumer represents a message consumer
type Consumer struct {
	client         *Client
	deliveries     <-chan amqp.Delivery
	channel        *amqp.Channel
	conn           *amqp.Connection
	queueName      string
	consumerTag    string
	done           chan struct{}
	notifyShutdown chan error
}

// NewConsumer creates a new consumer for the given queue
func (c *Client) NewConsumer(queueName, consumerTag string) (*Consumer, error) {
	ch, conn, err := c.getChannel()
	if err != nil {
		return nil, err
	}

	// Enable prefetch for better distribution (QoS)
	if err := ch.Qos(c.config.PrefetchCount, 0, false); err != nil {
		c.releaseChannel(ch, conn)
		return nil, fmt.Errorf("failed to set QoS: %w", err)
	}

	deliveries, err := ch.Consume(
		queueName,        // queue
		consumerTag,      // consumer tag
		c.config.AutoAck, // auto-ack
		false,            // exclusive
		false,            // no-local
		false,            // no-wait
		nil,              // arguments
	)
	if err != nil {
		c.releaseChannel(ch, conn)
		return nil, fmt.Errorf("failed to consume from queue: %w", err)
	}

	consumer := &Consumer{
		client:         c,
		deliveries:     deliveries,
		channel:        ch,
		conn:           conn,
		queueName:      queueName,
		consumerTag:    consumerTag,
		done:           make(chan struct{}),
		notifyShutdown: make(chan error, 1),
	}

	// Set up channel to monitor for connection closures
	closeChan := make(chan *amqp.Error)
	ch.NotifyClose(closeChan)

	// Monitor for channel closure and attempt to recover
	go consumer.monitorChannel(closeChan)

	return consumer, nil
}

// monitorChannel monitors the channel health and attempts to recover if it fails
func (c *Consumer) monitorChannel(closeChan <-chan *amqp.Error) {
	select {
	case err := <-closeChan:
		if err != nil {
			log.Warnf("Consumer's channel closed with error: %v", err)
			// Attempt to recover the consumer
			go c.tryRecover()
		}
	case <-c.done:
		return
	}
}

// tryRecover attempts to recover a consumer after channel failure
func (c *Consumer) tryRecover() {
	for {
		select {
		case <-c.done:
			return
		default:
			log.Info("Attempting to recover consumer...")

			// Get a new channel
			ch, conn, err := c.client.getChannel()
			if err != nil {
				log.Warnf("Failed to get new channel for consumer recovery: %v", err)
				time.Sleep(5 * time.Second)
				continue
			}

			// Set QoS on the new channel
			if err := ch.Qos(c.client.config.PrefetchCount, 0, false); err != nil {
				log.Warnf("Failed to set QoS on new channel: %v", err)
				c.client.releaseChannel(ch, conn)
				time.Sleep(5 * time.Second)
				continue
			}

			// Start consuming on the new channel
			deliveries, err := ch.Consume(
				c.queueName,             // queue
				c.consumerTag,           // consumer tag
				c.client.config.AutoAck, // auto-ack
				false,                   // exclusive
				false,                   // no-local
				false,                   // no-wait
				nil,                     // arguments
			)
			if err != nil {
				log.Warnf("Failed to consume from queue: %v", err)
				c.client.releaseChannel(ch, conn)
				time.Sleep(5 * time.Second)
				continue
			}

			// Update consumer with new channel and deliveries
			c.channel = ch
			c.deliveries = deliveries
			c.conn = conn

			// Set up new channel closure monitoring
			closeChan := make(chan *amqp.Error)
			ch.NotifyClose(closeChan)
			go c.monitorChannel(closeChan)

			log.Info("Consumer successfully recovered")
			return
		}
	}
}

// Consume starts consuming messages and processes them with the callback function
func (c *Consumer) Consume(callback func(msg []byte) error) {
	for {
		select {
		case <-c.done:
			return
		case delivery, ok := <-c.deliveries:
			if !ok {
				log.Warn("Delivery channel closed")
				c.notifyShutdown <- errors.New("delivery channel closed")
				return
			}

			// Process the message
			err := callback(delivery.Body)

			// Only acknowledge or reject if auto-ack is disabled
			if !c.client.config.AutoAck {
				if err != nil {
					log.Errorf("Failed to process message: %v", err)
					// Reject the message and requeue it
					_ = delivery.Reject(true)
				} else {
					// Acknowledge the message
					_ = delivery.Ack(false)
				}
			} else if err != nil {
				// Just log the error with auto-ack
				log.Errorf("Failed to process message with auto-ack enabled: %v", err)
			}
		}
	}
}

// ConsumeWithContext starts consuming messages with context control
func (c *Consumer) ConsumeWithContext(ctx context.Context, callback func(ctx context.Context, msg []byte) error) {
	for {
		select {
		case <-ctx.Done():
			log.Info("Context canceled, stopping consumer")
			return
		case <-c.done:
			return
		case delivery, ok := <-c.deliveries:
			if !ok {
				log.Warn("Delivery channel closed")
				c.notifyShutdown <- errors.New("delivery channel closed")
				return
			}

			// Create message context
			msgCtx, cancel := context.WithTimeout(ctx, 30*time.Second)

			// Process the message
			err := callback(msgCtx, delivery.Body)

			// Clean up message context
			cancel()

			// Only acknowledge or reject if auto-ack is disabled
			if !c.client.config.AutoAck {
				if err != nil {
					log.Errorf("Failed to process message: %v", err)
					// Reject the message and requeue it
					_ = delivery.Reject(true)
				} else {
					// Acknowledge the message
					_ = delivery.Ack(false)
				}
			} else if err != nil {
				// Just log the error with auto-ack
				log.Errorf("Failed to process message with auto-ack enabled: %v", err)
			}
		}
	}
}

// Stop stops the consumer
func (c *Consumer) Stop() error {
	close(c.done)

	// Cancel the consumer
	if err := c.channel.Cancel(c.consumerTag, false); err != nil {
		return fmt.Errorf("failed to cancel consumer: %w", err)
	}

	// Don't close the channel, just return it to the pool
	c.client.releaseChannel(c.channel, c.conn)

	return nil
}
