package rabbit

import (
	"fmt"
	"sync"

	log "github.com/sirupsen/logrus"
)

var (
	// 单例实例
	instance *Manager
	// 确保线程安全的互斥锁
	once sync.Once
)

// ManagerConfig RabbitMQ管理器配置
type ManagerConfig struct {
	// 连接配置
	URL                string
	Heartbeat          string
	ConnectionTimeout  string
	MaxConnections     int
	MaxChannelsPerConn int
	AutoReconnect      bool
	ReconnectInterval  string
	PrefetchCount      int
	AutoAck            bool

	// 导出服务配置
	ExportExchange     string
	ExportExchangeType string
	ExportQueuePrefix  string
}

// DefaultManagerConfig 返回带有合理默认值的管理器配置
func DefaultManagerConfig() *ManagerConfig {
	return &ManagerConfig{
		URL:                "amqp://guest:guest@localhost:5672/",
		Heartbeat:          "10s",
		ConnectionTimeout:  "30s",
		MaxConnections:     3,
		MaxChannelsPerConn: 10,
		AutoReconnect:      true,
		ReconnectInterval:  "5s",
		PrefetchCount:      3,
		AutoAck:            false,
		ExportExchange:     "softpak.export.topic",
		ExportExchangeType: "topic",
		ExportQueuePrefix:  "softpak.export",
	}
}

// ExchangeConfig 交换机配置
type ExchangeConfig struct {
	Name       string
	Type       string
	Durable    bool
	AutoDelete bool
	Internal   bool
}

// QueueConfig 队列配置
type QueueConfig struct {
	Name       string
	Durable    bool
	AutoDelete bool
	Exclusive  bool
}

// BindingConfig 绑定配置
type BindingConfig struct {
	QueueName    string
	ExchangeName string
	RoutingKey   string
}

// Manager RabbitMQ客户端管理器
type Manager struct {
	client      *Client
	initialized bool
	mu          sync.RWMutex
	config      *ManagerConfig

	// 维护已声明的交换机和队列
	exchanges map[string]bool
	queues    map[string]bool
	bindings  map[string]bool
}

// InitializeWithConfig 使用指定配置初始化单例管理器
func InitializeWithConfig(config *ManagerConfig) error {
	var initErr error
	once.Do(func() {
		if config == nil {
			config = DefaultManagerConfig()
		}

		instance = &Manager{
			exchanges: make(map[string]bool),
			queues:    make(map[string]bool),
			bindings:  make(map[string]bool),
			config:    config,
		}
		initErr = instance.initialize()
	})

	if initErr != nil {
		return fmt.Errorf("failed to initialize RabbitMQ manager: %w", initErr)
	}

	return nil
}

// GetInstance 获取Manager单例实例
func GetInstance() (*Manager, error) {
	if instance != nil && instance.initialized {
		return instance, nil
	}

	return nil, fmt.Errorf("RabbitMQ manager not initialized, call InitializeWithConfig first")
}

// initialize 初始化RabbitMQ客户端管理器
func (m *Manager) initialize() error {
	// 确保必要的配置已提供
	if m.config.URL == "" {
		m.config.URL = DefaultManagerConfig().URL
	}

	// 创建RabbitMQ客户端配置
	clientConfig := &Config{
		URL: m.config.URL,
	}

	// 应用其他可选配置，使用默认值填充未设置的项
	clientConfig.Heartbeat = ParseDuration(m.config.Heartbeat, DefaultConfig().Heartbeat)
	clientConfig.ConnectionTimeout = ParseDuration(m.config.ConnectionTimeout, DefaultConfig().ConnectionTimeout)
	clientConfig.MaxConnections = m.config.MaxConnections
	if clientConfig.MaxConnections <= 0 {
		clientConfig.MaxConnections = DefaultConfig().MaxConnections
	}
	clientConfig.MaxChannelsPerConn = m.config.MaxChannelsPerConn
	if clientConfig.MaxChannelsPerConn <= 0 {
		clientConfig.MaxChannelsPerConn = DefaultConfig().MaxChannelsPerConn
	}
	clientConfig.AutoReconnect = m.config.AutoReconnect
	clientConfig.ReconnectInterval = ParseDuration(m.config.ReconnectInterval, DefaultConfig().ReconnectInterval)
	clientConfig.PrefetchCount = m.config.PrefetchCount
	if clientConfig.PrefetchCount <= 0 {
		clientConfig.PrefetchCount = DefaultConfig().PrefetchCount
	}
	clientConfig.AutoAck = m.config.AutoAck

	// 创建RabbitMQ客户端
	client, err := NewClient(clientConfig)
	if err != nil {
		return fmt.Errorf("failed to create RabbitMQ client: %w", err)
	}

	m.client = client
	m.initialized = true

	// 如果配置了导出服务，设置相关基础设施
	if m.config.ExportExchange != "" && m.config.ExportQueuePrefix != "" {
		if err := m.setupExportInfrastructure(); err != nil {
			return fmt.Errorf("failed to setup export infrastructure: %w", err)
		}
	}

	log.Info("RabbitMQ manager initialized successfully")
	return nil
}

// setupExportInfrastructure 声明导出服务所需的交换机和队列
func (m *Manager) setupExportInfrastructure() error {
	// 获取交换机配置
	exchange := m.config.ExportExchange
	exchangeType := m.config.ExportExchangeType
	if exchangeType == "" {
		exchangeType = "topic" // 默认为topic类型
	}

	// 声明交换机
	if err := m.DeclareExchange(ExchangeConfig{
		Name:       exchange,
		Type:       exchangeType,
		Durable:    true,
		AutoDelete: false,
		Internal:   false,
	}); err != nil {
		return fmt.Errorf("failed to declare export exchange: %w", err)
	}

	// 获取队列前缀配置
	queuePrefix := m.config.ExportQueuePrefix

	// 声明NL队列并绑定
	nlQueueName := queuePrefix + ".nl"
	if err := m.setupQueueWithBinding(nlQueueName, exchange, nlQueueName); err != nil {
		return fmt.Errorf("failed to setup NL queue: %w", err)
	}

	// 声明BE队列并绑定
	beQueueName := queuePrefix + ".be"
	if err := m.setupQueueWithBinding(beQueueName, exchange, beQueueName); err != nil {
		return fmt.Errorf("failed to setup BE queue: %w", err)
	}

	log.Info("Export infrastructure setup complete")
	return nil
}

// setupQueueWithBinding 声明队列并绑定到交换机
func (m *Manager) setupQueueWithBinding(queueName, exchangeName, routingKey string) error {
	// 声明队列
	if err := m.DeclareQueue(QueueConfig{
		Name:       queueName,
		Durable:    true,
		AutoDelete: false,
		Exclusive:  false,
	}); err != nil {
		return fmt.Errorf("failed to declare queue %s: %w", queueName, err)
	}

	// 绑定队列到交换机
	if err := m.BindQueue(BindingConfig{
		QueueName:    queueName,
		ExchangeName: exchangeName,
		RoutingKey:   routingKey,
	}); err != nil {
		return fmt.Errorf("failed to bind queue %s to exchange %s: %w", queueName, exchangeName, err)
	}

	return nil
}

// DeclareExchange 声明交换机
func (m *Manager) DeclareExchange(config ExchangeConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 检查交换机是否已声明
	if m.exchanges[config.Name] {
		return nil
	}

	if err := m.client.DeclareExchange(
		config.Name,
		config.Type,
		config.Durable,
		config.AutoDelete,
		config.Internal,
	); err != nil {
		return fmt.Errorf("failed to declare exchange %s: %w", config.Name, err)
	}

	m.exchanges[config.Name] = true
	return nil
}

// DeclareQueue 声明队列
func (m *Manager) DeclareQueue(config QueueConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 检查队列是否已声明
	if m.queues[config.Name] {
		return nil
	}

	_, err := m.client.DeclareQueue(
		config.Name,
		config.Durable,
		config.AutoDelete,
		config.Exclusive,
	)
	if err != nil {
		return fmt.Errorf("failed to declare queue %s: %w", config.Name, err)
	}

	m.queues[config.Name] = true
	return nil
}

// BindQueue 绑定队列到交换机
func (m *Manager) BindQueue(config BindingConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 创建绑定标识符
	bindingKey := fmt.Sprintf("%s:%s:%s", config.QueueName, config.ExchangeName, config.RoutingKey)

	// 检查绑定是否已存在
	if m.bindings[bindingKey] {
		return nil
	}

	if err := m.client.BindQueue(
		config.QueueName,
		config.RoutingKey,
		config.ExchangeName,
	); err != nil {
		return fmt.Errorf("failed to bind queue %s to exchange %s: %w",
			config.QueueName, config.ExchangeName, err)
	}

	m.bindings[bindingKey] = true
	return nil
}

// PublishMessage 发布消息到指定队列
func (m *Manager) PublishMessage(exchange, routingKey, message string) error {
	if !m.initialized {
		return fmt.Errorf("rabbitmq manager not initialized")
	}

	return m.client.PublishSimple(exchange, routingKey, message)
}

// Close 关闭RabbitMQ管理器
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.initialized {
		return nil
	}

	if err := m.client.Close(); err != nil {
		return fmt.Errorf("failed to close RabbitMQ client: %w", err)
	}

	m.initialized = false
	return nil
}

// GetClient 获取底层RabbitMQ客户端
// 注意：此方法仅用于特殊情况，一般应使用Manager提供的高级方法
func (m *Manager) GetClient() *Client {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.client
}
