package rabbit

import (
	log "github.com/sirupsen/logrus"
)

// InitializeManager 初始化RabbitMQ管理器
// 这个函数应该在应用启动时被调用，以确保在需要发送消息前RabbitMQ管理器已经准备就绪
// 如果不提供配置，将使用默认配置
func InitializeManager(config *ManagerConfig) error {
	log.Info("Initializing RabbitMQ manager...")

	// 使用提供的配置或默认配置初始化管理器
	err := InitializeWithConfig(config)
	if err != nil {
		log.Errorf("Failed to initialize RabbitMQ manager: %v", err)
		return err
	}

	// 获取实例以输出日志信息
	manager, err := GetInstance()
	if err != nil {
		return err
	}

	log.Infof("RabbitMQ manager initialized, connected to server with %d connections",
		manager.client.config.MaxConnections)
	return nil
}

// ShutdownManager 关闭RabbitMQ管理器
// 这个函数应该在应用关闭时被调用，以优雅地关闭连接
func ShutdownManager() error {
	log.Info("Shutting down RabbitMQ manager...")

	// 如果实例不存在，不需要关闭
	if instance == nil {
		return nil
	}

	err := instance.Close()
	if err != nil {
		log.Errorf("Error closing RabbitMQ manager: %v", err)
		return err
	}

	log.Info("RabbitMQ manager shutdown complete")
	return nil
}
