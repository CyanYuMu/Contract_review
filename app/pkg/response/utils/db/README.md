# DB 数据库操作包使用手册

## 概述

`utils/db` 包提供了一套统一的数据库操作接口，支持多种后端数据库，如MongoDB、Firestore等。该包通过抽象常见的数据库操作，使开发者可以使用相同的API访问不同类型的数据库，简化应用程序的开发和维护。

## 主要功能

- **增删改查操作**：提供基本的CRUD（创建、读取、更新、删除）操作
- **批量操作**：支持批量插入、更新、删除和读取
- **条件查询**：支持多种条件查询和排序
- **变更钩子**：支持数据变更通知
- **事务处理**：部分数据库支持事务操作

## 核心接口

```go
type DB interface {
    // Insert 创建一条新记录, 记录存在则进行覆盖
    Insert(ctx context.Context, table string, row Row, options ...*InsertOptions) (string, error)
    
    // Delete 删除一条记录
    Delete(ctx context.Context, table string, id string, options ...*DeleteOptions) (int, error)
    
    // Create 创建一条新记录, 如果记录存在返回Error, 返回对应的ID
    Create(ctx context.Context, table string, row Row, options ...*CreateOptions) (*CreateResult, error)
    
    // Update 更新一条记录
    Update(ctx context.Context, table string, update UpdateOne, options ...*UpdateOptions) error
    
    // Get 读取一条记录
    Get(ctx context.Context, table string, id string, options ...*GetOptions) (*DocumentRef, error)
    
    // Find 查询多条记录
    Find(ctx context.Context, table string, conds Conds, options ...*FindOptions) (*Iterator, error)
    
    // BatchCreate 批量创建记录, 如果记录存在则返回错误
    BatchCreate(ctx context.Context, table string, rows []Row, options ...*BatchWriteOptions) *BatchWriteResult
    
    // BatchInsert 批量插入记录, 如果记录存在则进行覆盖
    BatchInsert(ctx context.Context, table string, rows []Row, options ...*BatchWriteOptions) *BatchWriteResult
    
    // BatchDelete 批量删除记录
    BatchDelete(ctx context.Context, table string, ids []string, options ...*BatchWriteOptions) *BatchWriteResult
    
    // BatchUpdate 批量更新记录
    BatchUpdate(ctx context.Context, table string, updates []UpdateOne, options ...*BatchWriteOptions) *BatchWriteResult
    
    // BatchGet 批量获取记录
    BatchGet(ctx context.Context, table string, ids []string, options ...*GetOptions) (*Iterator, error)
    
    // Count 获取记录总数
    Count(ctx context.Context, table string, conds Conds) (int64, error)
    
    // Upsert 更新或插入一条记录
    Upsert(ctx context.Context, table string, row UpsertRow, options ...*UpsertOptions) (*UpsertRs, error)
}
```

## 使用示例

### 创建数据模型

首先，需要定义实现`Row`接口的数据模型：

```go
// Row 接口要求实现 ID() 和 SetID() 方法
type User struct {
    Id       string `json:"id" bson:"_id,omitempty"`
    Username string `json:"username" bson:"username"`
    Age      int    `json:"age" bson:"age"`
    Email    string `json:"email" bson:"email"`
}

// ID 实现 Row 接口，返回文档ID
func (u *User) ID() string {
    return u.Id
}

// SetID 实现 Row 接口，设置文档ID
func (u *User) SetID(id string) {
    u.Id = id
}
```

### 创建记录

```go
// 创建单条记录
user := &User{
    Username: "张三",
    Age:      25,
    Email:    "zhangsan@example.com",
}

// 如果记录已存在则返回错误
result, err := db.Create(ctx, "users", user)
if err != nil {
    // 处理错误
    if db.IsAlreadyExists(err) {
        fmt.Println("用户已存在")
    } else {
        fmt.Println("创建用户失败:", err)
    }
    return
}

fmt.Println("创建的用户ID:", result.ID)

// 创建或覆盖记录
id, err := db.Insert(ctx, "users", user)
if err != nil {
    fmt.Println("插入用户失败:", err)
    return
}

fmt.Println("插入的用户ID:", id)
```

### 读取记录

```go
// 根据ID读取单条记录
docRef, err := db.Get(ctx, "users", "user123")
if err != nil {
    if db.IsNotExistErr(err) {
        fmt.Println("用户不存在")
    } else {
        fmt.Println("获取用户失败:", err)
    }
    return
}

user := &User{}
err = docRef.Unmarshal(ctx, user)
if err != nil {
    fmt.Println("解析用户数据失败:", err)
    return
}

fmt.Printf("获取到的用户: %+v\n", user)
```

### 条件查询

```go
// 创建查询条件
conditions := db.Conds{
    {Field: "age", Cond: ">", Value: 18},
    {Field: "username", Cond: "==", Value: "张三"},
}

// 创建查询选项
options := &db.FindOptions{
    Limit: 10,
    Offset: 0,
    Sorts: db.Sorts{
        {Field: "age", Order: -1}, // 按年龄降序
    },
}

// 执行查询
iter, err := db.Find(ctx, "users", conditions, options)
if err != nil {
    fmt.Println("查询失败:", err)
    return
}
defer iter.Close()

// 读取所有结果
var users []*User
err = iter.All(ctx, &users)
if err != nil {
    fmt.Println("读取结果失败:", err)
    return
}

fmt.Printf("找到 %d 个用户\n", len(users))
for _, user := range users {
    fmt.Printf("- %s (%d岁)\n", user.Username, user.Age)
}
```

### 更新记录

```go
// 创建更新操作
update := db.UpdateOne{
    ID: "user123",
    Updates: []db.Update{
        {Field: "age", Op: "=", Value: 26},
        {Field: "email", Op: "=", Value: "new_email@example.com"},
    },
}

// 执行更新
err := db.Update(ctx, "users", update)
if err != nil {
    fmt.Println("更新失败:", err)
    return
}

fmt.Println("用户更新成功")
```

### 删除记录

```go
// 删除单条记录
affectedRows, err := db.Delete(ctx, "users", "user123")
if err != nil {
    fmt.Println("删除失败:", err)
    return
}

fmt.Printf("已删除 %d 条记录\n", affectedRows)
```

### 批量操作

```go
// 批量创建用户
users := []db.Row{
    &User{Username: "用户1", Age: 21, Email: "user1@example.com"},
    &User{Username: "用户2", Age: 22, Email: "user2@example.com"},
    &User{Username: "用户3", Age: 23, Email: "user3@example.com"},
}

options := &db.BatchWriteOptions{
    ContinueOnExistsError: 1, // 遇到已存在记录时继续
    BatchSize: 100,           // 每批次处理数量
}

result := db.BatchCreate(ctx, "users", users, options)
if err := result.Error(); err != nil {
    fmt.Println("批量创建失败:", err)
    return
}

fmt.Printf("成功创建 %d 条记录\n", result.Affected)
```

### 统计记录数量

```go
// 统计符合条件的记录数
conditions := db.Conds{
    {Field: "age", Cond: ">", Value: 20},
}

count, err := db.Count(ctx, "users", conditions)
if err != nil {
    fmt.Println("统计失败:", err)
    return
}

fmt.Printf("共有 %d 条记录\n", count)
```

## 条件查询操作符

支持的条件操作符包括：

- `==` 或 `=`: 等于
- `!=`: 不等于
- `>`: 大于
- `>=`: 大于等于
- `<`: 小于
- `<=`: 小于等于
- `in`: 在集合中
- `not-in`: 不在集合中

示例：

```go
// 查询年龄大于20且小于30的用户
conditions := db.Conds{
    {Field: "age", Cond: ">", Value: 20},
    {Field: "age", Cond: "<", Value: 30},
}

// 查询指定用户名集合中的用户
conditions := db.Conds{
    {Field: "username", Cond: "in", Value: []string{"张三", "李四", "王五"}},
}
```

## 变更钩子

该包支持全局变更钩子，可以在数据变更时触发自定义操作：

```go
// 定义变更钩子函数
changeHook := func(ctx context.Context, rows ...db.ChangeRow) {
    for _, row := range rows {
        fmt.Printf("表 %s 中 ID 为 %s 的记录发生 %s 操作\n", row.Table, row.Id, row.Action)
        // 可以进行日志记录、发送通知等操作
    }
}

// 设置全局变更钩子
db.InitGlobalChangeHook(changeHook)

// 也可以为单个操作设置变更钩子
options := &db.InsertOptions{
    ChangeHook: customHook,
}
id, err := db.Insert(ctx, "users", user, options)
```

## 错误处理

该包提供了辅助函数用于识别特定类型的错误：

```go
// 检查是否为"记录已存在"错误
if db.IsAlreadyExists(err) {
    fmt.Println("记录已存在")
}

// 检查是否为"记录不存在"错误
if db.IsNotExistErr(err) {
    fmt.Println("记录不存在")
}
```

## 最佳实践

1. **使用上下文控制超时**：在所有数据库操作中使用带超时的上下文，避免长时间阻塞：
   ```go
   ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
   defer cancel()
   ```

2. **批量操作优化**：对于大量记录的操作，使用批量方法，并适当设置批次大小：
   ```go
   options := &db.BatchWriteOptions{
       BatchSize: 100, // 每批次处理100条记录
   }
   ```

3. **合理使用索引**：根据查询条件在数据库中创建合适的索引，提高查询效率。

4. **处理所有错误**：始终检查并处理返回的错误，特别是在批量操作中。

5. **关闭迭代器**：使用完迭代器后记得关闭，避免资源泄漏：
   ```go
   iter, err := db.Find(...)
   if err != nil {
       // 处理错误
   }
   defer iter.Close()
   ```

## 注意事项

1. 不同的数据库后端可能对某些操作有特定的限制或行为，请参考具体实现的文档。

2. 大数据量操作时应注意内存使用，使用批量操作并设置合理的批次大小。

3. 在高并发环境中，应考虑使用事务或乐观锁机制来处理并发更新问题。

4. `Row` 接口要求实现 `ID()` 和 `SetID()` 方法，确保模型结构体正确实现这些方法。

5. 变更钩子在数据操作完成后同步调用，应避免在钩子中执行耗时操作，以免影响性能。
