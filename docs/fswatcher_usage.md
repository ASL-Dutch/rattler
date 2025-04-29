# 文件监听组件 (FSWatcher) 使用说明

`FSWatcher` 是一个通用的文件监听组件，可用于监听目录中的文件变化，并执行自定义处理函数。

## 功能特点

- 支持监听多种文件操作（创建、修改、删除、重命名等）
- 支持文件后缀过滤
- 支持等待文件写入完成后再处理
- 支持自定义处理函数
- 支持递归监听子目录
- 自动处理新创建的子目录

## 使用方法

### 基础用法

```go
package main

import (
    "fmt"
    log "github.com/sirupsen/logrus"
    "sysafari.com/softpak/rattler/internal/component"
)

func main() {
    // 定义文件处理函数
    handler := func(filename string, additionalData interface{}) error {
        fmt.Printf("处理文件: %s\n", filename)
        // 在这里添加你的业务逻辑
        return nil
    }
    
    // 创建监听配置
    config := component.FSWatcherConfig{
        Dir:           "/path/to/watch",             // 监听目录
        Operations:    component.Create,             // 监听创建事件
        FilePattern:   ".*\\.txt",                   // 只处理.txt文件
        WaitTime:      3,                            // 等待3秒文件写入完成
        MaxRetries:    5,                            // 最多重试5次
        MinFileSize:   1024,                         // 文件最小1KB
        Handler:       handler,                      // 处理函数
        AdditionalData: nil,                         // 额外数据（可选）
    }
    
    // 创建并启动监听器
    watcher := component.NewFSWatcher(config)
    err := watcher.Start()
    if err != nil {
        log.Fatalf("启动文件监听失败: %v", err)
    }
    
    // 等待监听结束（阻塞）
    watcher.WaitForCompletion()
}
```

### 监听多种操作

```go
// 监听创建和修改事件
config.Operations = component.Create | component.Write

// 监听所有事件
config.Operations = component.All
```

### 自定义处理延迟

如果需要处理文件写入操作导致的不完整文件：

```go
// 设置等待时间和重试次数
config.WaitTime = 5    // 每次等待5秒
config.MaxRetries = 10 // 最多重试10次
config.MinFileSize = 100 // 文件最小大小（字节）
```

### 带上下文的处理函数

```go
// 定义上下文结构体
type MyContext struct {
    UserID string
    AppName string
}

// 处理函数
handler := func(filename string, additionalData interface{}) error {
    ctx, ok := additionalData.(*MyContext)
    if !ok {
        return fmt.Errorf("无效的上下文数据")
    }
    
    fmt.Printf("用户 %s 应用 %s 处理文件: %s\n", 
               ctx.UserID, ctx.AppName, filename)
    return nil
}

// 设置上下文
myCtx := &MyContext{
    UserID: "user123",
    AppName: "MyApp",
}

config.Handler = handler
config.AdditionalData = myCtx
```

## 最佳实践

1. **适当的等待时间**：
   - 如果文件较大或写入较慢，增加 `WaitTime` 和 `MaxRetries`
   - 对于小文件，可以减少等待时间提高效率

2. **合理设置文件模式**：
   - 使用精确的正则表达式匹配需要处理的文件
   - 过于宽松的匹配可能导致处理不必要的文件

3. **优雅停止**：
   - 在应用关闭前调用 `watcher.Stop()` 释放资源

4. **错误处理**：
   - 处理函数中应妥善处理各种异常情况
   - 避免处理函数长时间阻塞

## 实现说明

组件通过 `fsnotify` 库监听文件系统事件，并提供以下增强功能：

1. 文件可读性检查：通过多次尝试打开文件，确保文件已完成写入
2. 文件大小检查：避免处理空文件或不完整文件
3. 异步处理：文件处理在单独的 goroutine 中执行，避免阻塞监听
4. 子目录监听：自动添加新创建的子目录到监听列表

## 排障指南

1. **文件没有被处理**：
   - 检查文件是否匹配 `FilePattern` 模式
   - 确认监听的是正确的 `Operations` 类型
   - 验证文件大小是否达到 `MinFileSize`

2. **处理函数没有执行**：
   - 检查 `Handler` 是否正确设置
   - 确保文件写入完成检查通过

3. **性能问题**：
   - 减少监听的目录数量
   - 使用更精确的文件模式匹配
   - 优化处理函数执行效率 