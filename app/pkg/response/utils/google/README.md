# Firebase组件使用指南

## 概述

Firebase组件提供了与Google Firebase服务集成的功能，包括Firebase App和Firestore数据库客户端。该组件支持TLS证书验证的配置，可以解决证书验证问题。

## 功能特性

- **Firebase App创建**: 支持创建Firebase应用实例
- **Firestore客户端**: 支持创建Firestore数据库客户端
- **TLS配置**: 可配置TLS证书验证，解决证书问题
- **连接池管理**: 支持配置连接池大小
- **多种凭据方式**: 支持JSON字符串和文件路径两种凭据方式

## 配置结构

```go
type FirebaseConfig struct {
    // 凭据配置（二选一）
    CredentialsJson string // Base64编码的JSON凭据
    CredentialsFile string // 凭据文件路径（优先级更高）
    
    // 项目配置
    ProjectId  string // Firebase项目ID
    AccountId  string // 服务账户ID
    Bucket     string // 存储桶名称
    DatabaseId string // Firestore数据库ID
    
    // 连接池配置
    PoolSize int // 连接池大小
    
    // TLS配置
    DisableTLSVerify bool // 是否禁用TLS证书验证
}
```

## 基本用法

### 创建Firebase应用

```go
import (
    "context"
    "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/google"
)

func main() {
    ctx := context.Background()
    
    // 配置Firebase
    config := google.FirebaseConfig{
        CredentialsJson: "your-base64-encoded-credentials-json",
        ProjectId:       "your-project-id",
        AccountId:       "your-service-account-id",
        Bucket:          "your-storage-bucket",
        PoolSize:        10,
        DisableTLSVerify: false, // 启用TLS验证
    }
    
    // 创建Firebase应用
    app, err := google.NewFirebase(ctx, config)
    if err != nil {
        // 处理错误
        return err
    }
    
    // 使用app...
}
```

### 创建Firestore客户端

```go
func createFirestoreClient() error {
    ctx := context.Background()
    
    // 配置Firebase
    config := google.FirebaseConfig{
        CredentialsJson: "your-base64-encoded-credentials-json",
        ProjectId:       "your-project-id",
        AccountId:       "your-service-account-id",
        Bucket:          "your-storage-bucket",
        PoolSize:        10,
        DisableTLSVerify: false, // 启用TLS验证
    }
    
    // 创建Firestore实例
    inst, err := google.NewFirestoreWithDatabase(ctx, "your-project-id", "your-database-id", config)
    if err != nil {
        return err
    }
    
    // 使用inst.Client进行数据库操作...
    return nil
}
```

## TLS证书验证配置

### 启用TLS验证（推荐用于生产环境）

```go
config := google.FirebaseConfig{
    CredentialsJson: "your-credentials-json",
    ProjectId:       "your-project-id",
    AccountId:       "your-service-account-id",
    Bucket:          "your-bucket",
    PoolSize:        20,
    DisableTLSVerify: false, // 启用TLS验证
}
```

### 禁用TLS验证（用于解决证书问题）

```go
config := google.FirebaseConfig{
    CredentialsJson: "your-credentials-json",
    ProjectId:       "your-project-id",
    AccountId:       "your-service-account-id",
    Bucket:          "your-bucket",
    PoolSize:        10,
    DisableTLSVerify: true, // 禁用TLS验证
}
```

## 环境配置示例

### 开发环境配置

```go
func getDevConfig() google.FirebaseConfig {
    return google.FirebaseConfig{
        CredentialsJson: "dev-credentials-json",
        ProjectId:       "dev-project-id",
        AccountId:       "dev-service-account",
        Bucket:          "dev-bucket",
        PoolSize:        5, // 开发环境使用较小的连接池
        DisableTLSVerify: true, // 开发环境可以禁用TLS验证
    }
}
```

### 生产环境配置

```go
func getProdConfig() google.FirebaseConfig {
    return google.FirebaseConfig{
        CredentialsFile: "/secure/path/to/prod-credentials.json", // 生产环境使用文件
        ProjectId:       "prod-project-id",
        AccountId:       "prod-service-account",
        Bucket:          "prod-bucket",
        PoolSize:        50, // 生产环境使用较大的连接池
        DisableTLSVerify: false, // 生产环境必须启用TLS验证
    }
}
```

## 凭据配置

### 使用Base64编码的JSON凭据

```go
config := google.FirebaseConfig{
    CredentialsJson: "eyJ0eXBlIjoic2VydmljZV9hY2NvdW50IiwicHJva...", // Base64编码
    // 其他配置...
}
```

### 使用凭据文件

```go
config := google.FirebaseConfig{
    CredentialsFile: "/path/to/service-account-key.json", // 文件路径
    // 其他配置...
}
```

## 错误处理

### 常见错误及解决方案

1. **TLS证书验证错误**
   ```
   grpc: the credentials require transport level security
   ```
   **解决方案**: 设置 `DisableTLSVerify: true`

2. **凭据错误**
   ```
   illegal base64 data at input byte
   ```
   **解决方案**: 确保CredentialsJson是有效的Base64编码

3. **项目ID错误**
   ```
   project not found
   ```
   **解决方案**: 检查ProjectId是否正确

## 最佳实践

### 1. 环境分离
- 开发环境：可以禁用TLS验证以简化配置
- 生产环境：必须启用TLS验证以确保安全

### 2. 凭据管理
- 开发环境：可以使用Base64编码的JSON
- 生产环境：建议使用文件路径，确保文件权限安全

### 3. 连接池配置
- 开发环境：5-10个连接
- 生产环境：20-100个连接，根据负载调整

### 4. 错误处理
```go
app, err := google.NewFirebase(ctx, config)
if err != nil {
    // 记录错误日志
    su_logger.Error(ctx, err, "failed to create firebase app")
    
    // 根据错误类型进行处理
    if strings.Contains(err.Error(), "certificate") {
        // TLS证书相关错误
        config.DisableTLSVerify = true
        app, err = google.NewFirebase(ctx, config)
    }
    
    return err
}
```

## 注意事项

1. **安全性**: 生产环境不建议禁用TLS验证
2. **凭据安全**: 确保凭据文件或字符串的安全性
3. **连接池**: 根据实际负载合理配置连接池大小
4. **错误监控**: 监控Firebase连接错误，及时处理

## 故障排除

### 问题1: TLS证书验证失败
**症状**: `grpc: the credentials require transport level security`
**解决**: 设置 `DisableTLSVerify: true`

### 问题2: 凭据无效
**症状**: `illegal base64 data` 或 `invalid credentials`
**解决**: 检查凭据格式和内容

### 问题3: 连接超时
**症状**: 连接建立超时
**解决**: 检查网络连接和防火墙设置

### 问题4: 项目不存在
**症状**: `project not found`
**解决**: 验证项目ID和服务账户权限

