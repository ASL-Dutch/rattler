package cmd

import (
	"context"
	"errors"
	"sync"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"sysafari.com/softpak/rattler/internal/rabbit"
	"sysafari.com/softpak/rattler/internal/service"
)

var (
	// A context that can be canceled to stop all consumers
	rabbitCtx    context.Context
	rabbitCancel context.CancelFunc
	// WaitGroup to track all running consumers
	consumerWg sync.WaitGroup
	// 确保初始化只执行一次
	initOnce sync.Once
)

// getRabbitConfig 从viper配置中构建RabbitMQ管理器配置
func getRabbitConfig() *rabbit.ManagerConfig {
	config := &rabbit.ManagerConfig{
		// 连接配置
		URL:                viper.GetString("rabbitmq.url"),
		Heartbeat:          viper.GetString("rabbitmq.heartbeat"),
		ConnectionTimeout:  viper.GetString("rabbitmq.connection-timeout"),
		MaxConnections:     viper.GetInt("rabbitmq.max-connections"),
		MaxChannelsPerConn: viper.GetInt("rabbitmq.max-channels-per-conn"),
		AutoReconnect:      viper.GetBool("rabbitmq.auto-reconnect"),
		ReconnectInterval:  viper.GetString("rabbitmq.reconnect-interval"),
		PrefetchCount:      viper.GetInt("rabbitmq.prefetch-count"),
		AutoAck:            viper.GetBool("rabbitmq.auto-ack"),

		// 导出服务配置
		ExportExchange:     viper.GetString("rabbitmq.export.exchange"),
		ExportExchangeType: viper.GetString("rabbitmq.export.exchange-type"),
		ExportQueuePrefix:  viper.GetString("rabbitmq.export.queue"),
	}

	return config
}

// InitRabbitMQ 初始化RabbitMQ客户端
func InitRabbitMQ() error {
	var initErr error

	initOnce.Do(func() {
		log.Info("Initializing RabbitMQ...")

		// 获取配置
		config := getRabbitConfig()

		// 初始化RabbitMQ管理器
		initErr = rabbit.InitializeManager(config)
		if initErr != nil {
			log.Errorf("Failed to initialize RabbitMQ manager: %v", initErr)
			return
		}

		// 创建用于管理消费者的全局上下文
		rabbitCtx, rabbitCancel = context.WithCancel(context.Background())

		log.Info("RabbitMQ initialized successfully")
	})

	return initErr
}

// GetRabbitClient 获取RabbitMQ客户端实例
// 此方法保留用于向后兼容
func GetRabbitClient() (*rabbit.Client, error) {
	// 确保管理器已初始化
	if err := InitRabbitMQ(); err != nil {
		return nil, err
	}

	// 获取管理器实例
	manager, err := rabbit.GetInstance()
	if err != nil {
		return nil, err
	}

	// 返回底层客户端
	return manager.GetClient(), nil
}

// StartImportXmlConsumer 启动导入XML消息的消费者
func StartImportXmlConsumer() error {
	// 确保管理器已初始化
	if err := InitRabbitMQ(); err != nil {
		return err
	}

	// 获取管理器实例
	manager, err := rabbit.GetInstance()
	if err != nil {
		return err
	}

	// 获取底层客户端用于创建消费者
	client := manager.GetClient()

	// 获取队列配置
	exchangeName := viper.GetString("rabbitmq.import.exchange")
	exchangeType := viper.GetString("rabbitmq.import.exchange-type")
	if exchangeType == "" {
		exchangeType = "direct"
	}
	log.Infof("Declaring exchange: %s, type: %s", exchangeName, exchangeType)

	// 声明交换机
	err = manager.DeclareExchange(rabbit.ExchangeConfig{
		Name:       exchangeName,
		Type:       exchangeType,
		Durable:    true,
		AutoDelete: false,
		Internal:   false,
	})
	if err != nil {
		log.Errorf("Failed to declare exchange: %v", err)
		return err
	}

	// 获取队列名称
	queueName := viper.GetString("rabbitmq.import.queue")
	if queueName == "" {
		return errors.New("import xml queue name is empty")
	}

	// 声明队列
	err = manager.DeclareQueue(rabbit.QueueConfig{
		Name:       queueName,
		Durable:    true,
		AutoDelete: false,
		Exclusive:  false,
	})
	if err != nil {
		return errors.New("failed to declare queue: " + err.Error())
	}

	// 绑定队列到交换机
	err = manager.BindQueue(rabbit.BindingConfig{
		QueueName:    queueName,
		ExchangeName: exchangeName,
		RoutingKey:   "",
	})
	if err != nil {
		return errors.New("failed to bind queue: " + err.Error())
	}

	// 创建消费者
	consumerName := viper.GetString("rabbitmq.import.consumer")
	if consumerName == "" {
		consumerName = "softpak_import_xml_consumer"
	}
	consumer, err := client.NewConsumer(queueName, consumerName)
	if err != nil {
		return err
	}

	// 跟踪此消费者
	consumerWg.Add(1)

	// 在goroutine中启动消费
	go func() {
		defer consumerWg.Done()

		// 定义消息处理函数
		messageHandler := func(ctx context.Context, body []byte) error {
			log.Infof("Received message: %s", string(body))
			// 处理导入XML文件
			service.SaveImportDocument(string(body))
			return nil
		}

		// 开始消费
		log.Infof("Starting consumer for queue: %s", queueName)
		consumer.ConsumeWithContext(rabbitCtx, messageHandler)
		log.Infof("Consumer stopped for queue: %s", queueName)
	}()

	return nil
}

// StartMessageQueueConsumers 初始化并启动所有RabbitMQ消费者
func StartMessageQueueConsumers() error {
	// 确保RabbitMQ已初始化
	if err := InitRabbitMQ(); err != nil {
		return err
	}

	// 启动导入XML消费者
	if err := StartImportXmlConsumer(); err != nil {
		log.Errorf("Failed to start import xml consumer: %v", err)
		return err
	}

	// 如有需要，在此添加其他消费者
	// 例如: StartOtherConsumer()

	return nil
}

// StopMessageQueueConsumers 优雅地停止所有消费者
func StopMessageQueueConsumers() {
	if rabbitCancel != nil {
		log.Info("Stopping all RabbitMQ consumers...")
		rabbitCancel()

		// 等待所有消费者停止
		consumerWg.Wait()
		log.Info("All RabbitMQ consumers stopped")
	}
}

// CloseRabbitMQ 关闭RabbitMQ客户端连接并释放资源
func CloseRabbitMQ() {
	// 首先停止所有消费者
	StopMessageQueueConsumers()

	// 关闭RabbitMQ管理器
	err := rabbit.ShutdownManager()
	if err != nil {
		log.Errorf("Error closing RabbitMQ manager: %v", err)
	} else {
		log.Info("RabbitMQ manager closed successfully")
	}
}
