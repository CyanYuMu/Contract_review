# Cloud Storage 云存储工具使用手册

## 概述

`cloud_storage`包提供了统一的云存储操作接口，支持多种云存储服务，包括Google Cloud Storage、华为云OBS和MinIO等。通过这个包，您可以使用相同的API对不同的云存储服务进行操作，简化了云存储的应用开发。

## 主要功能

- **文件上传**：支持从内存、文件或读取器上传文件
- **文件下载**：读取云存储对象到内存或写入器
- **文件管理**：检查文件是否存在，获取文件属性，设置文件元数据
- **分片上传**：支持大文件分片上传和合并
- **预签名URL**：生成带有时效性的预签名URL，用于临时授权访问
- **文件移动**：在不同存储桶间移动文件
- **文件删除**：删除云存储中的文件

## 支持的存储服务

- **Google Cloud Storage (GCS)**
- **华为云对象存储服务 (OBS)**
- **MinIO**（兼容S3协议的对象存储）

## 使用示例

### 创建云存储实例

```go
import (
    "context"
    "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/cloud_storage"
)

// 创建云存储实例，根据配置自动选择合适的存储提供商
storage, err := cloud_storage.NewStorage(config)
if err != nil {
    // 处理错误
}
```

### 上传文件

```go
// 从字节数组上传
fileContent := []byte("文件内容")
err := storage.UploadObject(ctx, "objectName.txt", fileContent, cloud_storage.ObjectAttr{
    ContentType: "text/plain",
})

// 从文件上传
file, err := os.Open("localFile.jpg")
if err != nil {
    // 处理错误
}
defer file.Close()

err = storage.UploadObjectFromFile(ctx, "images/photo.jpg", file, cloud_storage.ObjectAttr{
    ContentType: "image/jpeg",
})

// 从读取器上传
reader := strings.NewReader("Hello, World!")
err = storage.UploadFromReader(ctx, "hello.txt", reader, cloud_storage.ObjectAttr{
    ContentType: "text/plain",
})
```

### 下载文件

```go
// 读取对象到内存
data, err := storage.ReadObject(ctx, "objectName.txt")
if err != nil {
    // 处理错误
}
fmt.Println("文件内容:", string(data))

// 获取读取器
reader, err := storage.GetReader(ctx, "objectName.txt")
if err != nil {
    // 处理错误
}
defer reader.Close()

// 使用读取器进行操作
buf := new(bytes.Buffer)
io.Copy(buf, reader)
```

### 检查文件是否存在

```go
exists := storage.IsObjectExists(ctx, "objectName.txt")
if exists {
    fmt.Println("文件存在")
} else {
    fmt.Println("文件不存在")
}
```

### 获取文件属性

```go
// 获取单个文件属性
attrs, err := storage.GetObjectAttrs(ctx, "objectName.txt")
if err != nil {
    // 处理错误
}
fmt.Println("文件大小:", attrs.Size)
fmt.Println("文件类型:", attrs.ContentType)

// 批量获取文件属性
objectNames := []string{"file1.txt", "file2.jpg", "file3.pdf"}
attrsArray, err := storage.BatchGetObjectAttrs(ctx, objectNames)
if err != nil {
    // 处理错误
}
for _, attr := range attrsArray {
    fmt.Printf("文件名: %s, 大小: %d 字节\n", attr.Name, attr.Size)
}
```

### 设置文件元数据

```go
metadata := map[string]string{
    "createdBy": "user123",
    "category": "documents",
}
err := storage.SetObjectAttrs(ctx, "objectName.txt", metadata)
if err != nil {
    // 处理错误
}
```

### 获取文件大小

```go
size := storage.GetObjectSize(ctx, "objectName.txt")
fmt.Printf("文件大小: %d 字节\n", size)
```

### 生成预签名URL

```go
// 单个文件预签名
file := cloud_storage.File{
    Id:          "file123",
    Ext:         ".jpg",
    Size:        1024 * 1024, // 1MB
    ContentType: "image/jpeg",
    Method:      "GET",
    Expires:     time.Now().Add(24 * time.Hour),
}

url, err := storage.GetPreSignature(storage.Client(), "objectName.jpg", file)
if err != nil {
    // 处理错误
}
fmt.Println("预签名URL:", url)

// 批量生成预签名URL
prefix := "uploads/"
shards := []cloud_storage.ShardItem{
    {Idx: 0, Size: 512 * 1024},    // 512KB
    {Idx: 1, Size: 512 * 1024},    // 512KB
}

signResult, err := storage.BatchGetPreSignatures(prefix, file, shards)
if err != nil {
    // 处理错误
}

for _, item := range signResult.Items {
    fmt.Printf("分片 %d, URL: %s\n", item.Idx, item.Url)
}
```

### 合并分片文件

```go
// 定义分片信息
mergeParts := &cloud_storage.MergeFileParts{
    FileId:   "file123",
    FileExt:  ".mp4",
    FileSize: 10 * 1024 * 1024, // 10MB
    Parts: map[string]*cloud_storage.Part{
        "0": {Number: "0", Url: "part0.mp4", Confirmed: true},
        "1": {Number: "1", Url: "part1.mp4", Confirmed: true},
    },
    Prefix: "uploads/",
}

// 合并参数
param := cloud_storage.MergeFileParam{
    SizeLimit: 100 * 1024 * 1024, // 100MB限制
}

// 执行合并
fileUrl, err := storage.MergeFileParts(mergeParts, param)
if err != nil {
    // 处理错误
}
fmt.Println("合并后的文件URL:", fileUrl)
```

### 文件移动

```go
// 移动文件到另一个存储桶
newBucket := storage.Client() // 此处应使用目标桶的客户端
err := storage.MoveObject(ctx, "old/path.txt", newBucket, "new/path.txt")
if err != nil {
    // 处理错误
}
```

### 删除文件

```go
err := storage.DeleteObject(ctx, "objectName.txt")
if err != nil {
    // 处理错误
}
```

### 对象组合

```go
// 将多个对象组合成一个新对象
objects := []interface{}{"part1.txt", "part2.txt", "part3.txt"}
attr, err := storage.ComposerAndRun(ctx, "combined.txt", objects)
if err != nil {
    // 处理错误
}
fmt.Printf("组合后的对象大小: %d 字节\n", attr.Size)
```

## 错误处理

包提供了`IsNotFound`函数来判断错误是否为"对象不存在"错误：

```go
data, err := storage.ReadObject(ctx, "non-existent-file.txt")
if err != nil {
    if cloud_storage.IsNotFound(err) {
        fmt.Println("文件不存在，请检查文件名")
    } else {
        fmt.Println("发生其他错误:", err)
    }
}
```

## 最佳实践

1. **大文件处理**：对于大文件，使用分片上传和合并功能，避免内存占用过高。

2. **重用客户端**：通过`Client()`方法获取客户端，并在多个操作中重用，避免频繁创建连接。

3. **适当设置超时**：在创建上下文时设置适当的超时时间，防止长时间阻塞：
   ```go
   ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
   defer cancel()
   ```

4. **使用预签名URL**：对于需要临时授权访问的文件，使用预签名URL功能，而不是将文件公开。

5. **检查文件存在性**：在操作前先使用`IsObjectExists`检查文件是否存在，避免不必要的错误。

## 注意事项

1. 不同的云存储服务可能有特定的限制和行为，如分片大小限制、预签名URL有效期等。

2. 上传大文件时，建议使用分片上传以提高可靠性，特别是在网络不稳定的环境中。

3. 删除操作无法撤销，执行前请确认是否真的需要删除该文件。

4. 使用`MediaCdnSignURL`功能需要正确配置密钥，否则生成的URL将无法访问。

5. 对于频繁访问的小文件，考虑使用缓存机制减少云存储的访问频率。
