# Encrypt工具包使用手册

## 概述

`encrypt`包提供了常用的加密、编码和哈希计算功能，包括Base64编解码、MD5哈希计算和JWT令牌生成与验证。这些工具可以帮助您轻松实现数据安全和身份验证功能。

## 功能模块

### 1. Base64编解码 (encode.go)

提供标准的Base64编码和解码功能：

```go
// 编码字节数组为Base64字符串
base64Str := encrypt.Base64Encode([]byte("hello world"))

// 解码Base64字符串为字节数组
bytes, err := encrypt.Base64Decode([]byte("aGVsbG8gd29ybGQ="))

// 编码字符串为Base64字符串
base64Str := encrypt.Base64EncodeString("hello world")

// 解码Base64字符串为普通字符串
str, err := encrypt.Base64DecodeString("aGVsbG8gd29ybGQ=")
```

### 2. 哈希计算 (hash.go)

提供MD5和CRC32哈希计算：

```go
// 计算字符串的MD5（大写形式）
md5 := encrypt.MD5("hello world") // 返回大写MD5值

// 计算字符串的MD5（小写形式）
md5 := encrypt.Md5("hello world") // 返回小写MD5值

// 计算CRC32值
crc32Value := encrypt.CRC32("hello world") // 返回uint32类型的CRC32值
```

### 3. JWT令牌 (jwt_token.go)

提供基于RSA-512的JWT令牌生成和验证功能：

```go
// 创建JWT声明
claims := &encrypt.Claims{
    RegisteredClaims: jwt.RegisteredClaims{
        Subject:   "user_id_123",
        IssuedAt:  jwt.NewNumericDate(time.Now()),
        ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour * 24)),
    },
    Payload: `{"name":"张三","role":"admin"}`,
}

// 生成JWT令牌（需要RSA私钥）
token, err := encrypt.SignByRSA512(privateKey, claims)

// 验证并解析JWT令牌（需要RSA公钥）
parsedClaims, err := encrypt.ParseByRSA512(publicKey, tokenString)
if err != nil {
    // 处理错误：令牌无效或已过期
} else {
    // 使用parsedClaims.Payload获取载荷数据
}
```

## 使用场景

### 数据编码和传输

使用Base64编码可以将二进制数据转换为可打印字符，适用于：
- API请求中传输二进制数据
- 将图片等二进制数据嵌入到JSON或XML中
- 简单的数据混淆（非加密）

### 数据校验和完整性验证

使用哈希函数可以生成数据的"指纹"，适用于：
- 验证文件或数据是否被修改
- 密码存储（配合盐值使用）
- 数据去重
- 数据分片

### 用户认证和会话管理

使用JWT令牌可以实现无状态的用户认证，适用于：
- 微服务架构中的用户认证
- API权限控制
- 单点登录系统
- 临时授权凭证

## 最佳实践

1. **Base64不是加密**：Base64只是编码，不提供任何安全性，不要用于敏感数据保护

2. **MD5不适合密码存储**：单纯的MD5不再被视为安全的密码存储方式，应考虑使用bcrypt或argon2等专用算法

3. **JWT私钥保护**：确保RSA私钥的安全，避免泄露

4. **设置合理的过期时间**：JWT令牌应设置合理的过期时间，提高安全性

## 注意事项

- JWT令牌验证失败会返回特定错误类型：`ErrSignMethodUnSupport`或`ErrTokenNotValid`
- Base64编码后的字符串长度通常会比原始数据增加约33%
- MD5是不可逆的哈希函数，无法从哈希值恢复原始数据
