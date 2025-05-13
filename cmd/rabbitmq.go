package cmd

import (
	"context"
	"errors"
	"sync"

	log "github.com/sirupsen/logrus"
	"sysafari.com/softpak/rattler/internal/config"
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

// getRabbitConfig 从配置对象中构建RabbitMQ管理器配置
func getRabbitConfig() *rabbit.ManagerConfig {
	// 使用全局配置对象
	rabbitConfig := config.GlobalConfig.RabbitMQ

	managerConfig := &rabbit.ManagerConfig{
		// 连接配置
		URL:                rabbitConfig.URL,
		Heartbeat:          rabbitConfig.Heartbeat,
		ConnectionTimeout:  rabbitConfig.ConnectionTimeout,
		MaxConnections:     rabbitConfig.MaxConnections,
		MaxChannelsPerConn: rabbitConfig.MaxChannelsPerConn,
		AutoReconnect:      rabbitConfig.AutoReconnect,
		ReconnectInterval:  rabbitConfig.ReconnectInterval,
		PrefetchCount:      rabbitConfig.PrefetchCount,
		AutoAck:            rabbitConfig.AutoAck,
		AutoCreate:         true,
		// 导出服务配置
		ExportExchange:     rabbitConfig.Export.Exchange,
		ExportExchangeType: rabbitConfig.Export.ExchangeType,
		ExportQueuePrefix:  rabbitConfig.Export.Queue,
	}

	return managerConfig
}

// InitRabbitMQ 初始化RabbitMQ客户端
func InitRabbitMQ() error {
	var err error

	// 使用sync.Once确保只初始化一次
	initOnce.Do(func() {
		// 创建可取消的上下文
		rabbitCtx, rabbitCancel = context.WithCancel(context.Background())

		// 初始化RabbitMQ管理器
		err = rabbit.InitializeWithConfig(getRabbitConfig())
		if err != nil {
			log.Errorf("Failed to initialize RabbitMQ manager: %v", err)
			return
		}

		log.Info("RabbitMQ manager initialized successfully ...")
	})

	return err
}

// CloseRabbitMQ 安全关闭所有RabbitMQ连接
func CloseRabbitMQ() {
	// 取消上下文，通知所有消费者停止
	if rabbitCancel != nil {
		rabbitCancel()
	}

	// 等待所有消费者完成
	consumerWg.Wait()

	// 关闭RabbitMQ管理器
	manager, err := rabbit.GetInstance()
	if err == nil && manager != nil {
		manager.Close()
	}

	log.Info("RabbitMQ connections closed")
}

// StartMessageQueueConsumers 启动所有消息队列消费者
func StartMessageQueueConsumers() error {
	// 启动导入XML消费者
	if err := StartImportXmlConsumer(); err != nil {
		return err
	}

	return nil
}

// StartImportXmlConsumer 启动导入XML消费者
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
	rabbitConfig := config.GlobalConfig.RabbitMQ
	exchangeName := rabbitConfig.Import.Exchange
	exchangeType := rabbitConfig.Import.ExchangeType
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
	queueName := rabbitConfig.Import.Queue
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
	consumerName := rabbitConfig.Import.Consumer
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
		log.Infof("Import XML consumer started: %s ...", consumerName)

		// 定义消息处理函数 - 根据 ConsumeWithContext 接口定义匹配参数类型
		messageHandler := func(ctx context.Context, msg []byte) error {
			log.Infof("Received import XML message")

			// 处理消息内容 - SaveImportDocument不返回值，所以这里不需要处理返回值
			service.SaveImportDocument(string(msg))

			// 消息处理成功
			return nil
		}

		// 开始消费
		consumer.ConsumeWithContext(rabbitCtx, messageHandler)
		log.Infof("Import XML consumer stopped")
	}()

	return nil
}
