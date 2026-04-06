# Factory工具包使用手册

## 概述

`factory`包提供了实现单例模式(Singleton)的工具，可以确保在整个应用程序中某个对象只有一个实例。这对于管理共享资源（如数据库连接、缓存等）特别有用，能够避免重复创建资源和确保数据一致性。

## 功能特点

- **线程安全**：使用`sync.Map`确保在并发环境下安全使用
- **按需创建**：只有在首次请求时才会创建对象实例
- **命名单例**：支持通过名称区分不同的单例对象
- **简单易用**：API简洁，容易理解和使用

## 使用方法

### 创建单例管理器

```go
// 创建一个新的单例管理器
singleton := factory.New()
```

### 获取或创建单例对象

```go
// 获取名为"db"的单例对象，如果不存在则创建新的
db := singleton.Get("db", func() interface{} {
    // 这个函数只会在首次请求时执行
    return NewDatabase("localhost", "user", "password")
}).(*Database)

// 再次获取同一个对象，不会重新创建
sameDatabaseInstance := singleton.Get("db", func() interface{} {
    // 这个函数不会执行，因为"db"实例已经存在
    return NewDatabase("localhost", "user", "password")
}).(*Database)

// db 和 sameDatabaseInstance 指向同一个实例
```

## 常见使用场景

### 1. 管理数据库连接

```go
// 自定义的数据库连接函数
func GetDBConnection() *sql.DB {
    singleton := factory.New()
    return singleton.Get("database", func() interface{} {
        db, err := sql.Open("mysql", "user:password@/dbname")
        if err != nil {
            panic(err)
        }
        return db
    }).(*sql.DB)
}
```

### 2. 缓存管理器

```go
// 创建或获取缓存实例
func GetCache() *Cache {
    singleton := factory.New()
    return singleton.Get("cache", func() interface{} {
        return NewCache(1000) // 创建容量为1000的缓存
    }).(*Cache)
}
```

### 3. 配置管理

```go
// 获取应用配置
func GetConfig() *Config {
    singleton := factory.New()
    return singleton.Get("config", func() interface{} {
        return LoadConfigFromFile("config.yaml")
    }).(*Config)
}
```

## 最佳实践

1. **保持单例管理器全局可访问**：通常将`factory.New()`创建的实例保存在包级变量中

   ```go
   var singletonManager = factory.New()
   
   func GetInstance(name string) SomeType {
       return singletonManager.Get(name, createFunc).(SomeType)
   }
   ```

2. **使用类型断言**：`Get`方法返回`interface{}`，使用时需要进行类型断言

   ```go
   // 正确的类型断言
   db := singleton.Get("db", createDBFunc).(*Database)
   ```

3. **区分不同类型的单例**：使用有意义的名称区分不同类型的单例

   ```go
   // 使用有意义的名称
   redisClient := singleton.Get("redis", createRedisFunc)
   mongoClient := singleton.Get("mongodb", createMongoFunc)
   ```

## 注意事项

- 如果两个goroutine同时首次请求同一个名称的单例，`newFunc`可能会被执行两次，但只有一个结果会被存储和返回
- 单例模式可能会导致对象生命周期管理复杂化，请确保在适当的时候释放资源
- 类型断言错误会导致panic，请确保断言为正确的类型
