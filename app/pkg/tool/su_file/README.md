# Su_file文件操作工具包

## 概述

`su_file`包提供了一系列文件和目录操作的便捷工具函数，包括文件压缩解压、目录扫描、文件类型判断等功能。这些工具可以帮助开发者轻松处理日常文件操作任务，减少重复代码。

## 主要功能

### 1. 文件压缩与解压 (zip.go)

提供ZIP格式的文件压缩和解压功能：

```go
// 将文件或目录压缩为ZIP文件
func Zip(srcFile string, destZip string) error

// 将ZIP文件解压到指定目录
func UnZip(srcFile string, destDir string) error
```

### 2. 文件与目录操作 (file.go)

提供基本的文件和目录操作函数：

```go
// 判断文件是否存在
func Exists(path string) bool

// 创建目录（如果不存在）
func CreateAll(path string) error

// 递归获取指定目录下的所有文件
func ScanDirFile(dirPath string, includeChild bool, filter func(path, name string) bool) ([]string, error)

// 删除文件夹及其下所有文件
func RemoveAll(dirPath string) error

// 判断是否为文件
func Is(path string) (bool, error)
```

### 3. 目录扫描 (dir.go)

提供目录扫描和文件查找功能：

```go
// 扫描目录查找符合条件的文件
func ScanDir(filePath string, maxDepth int, pattern *Pattern) (list []string, err error)

// 判断路径是否是目录
func IsDir(path string) (bool, error)
```

### 4. 路径处理 (path.go)

提供路径相关的工具函数：

```go
// 获取绝对路径
func GetAbsPath(path string) string

// 向上查找文件
func LookupFile(dir string, name string, depth int) (filepath string, err error)

// 向上查找目录
func LookupDir(dir string, name string, depth int) (dirPath string, err error)
```

### 5. 文件类型处理 (type.go)

提供文件类型判断和MIME类型处理：

```go
// 获取文件后缀
func GetSuffix(u string, withDot bool) string

// 获取文件名和后缀
func GetNameAndSuffix(u string, withSuffixDot bool) (baseName, suffix string)

// 根据文件后缀获取内容类型
func ContentType(suffix string) string

// 获取文件的MIME类型
func GetMimeType(filePath string) (string, error)

// 将MIME类型转换为文件扩展名
func MimeType2ContentType(mimeType string) string
```

## 使用示例

### 文件压缩

```go
import "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/tool/su_file"

func main() {
    // 将目录压缩为ZIP文件
    err := su_file.Zip("./source_folder", "./archive.zip")
    if err != nil {
        panic(err)
    }
    
    // 压缩单个文件
    err = su_file.Zip("./document.txt", "./document.zip")
    if err != nil {
        panic(err)
    }
}
```

### 文件解压

```go
import "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/tool/su_file"

func main() {
    // 将ZIP文件解压到指定目录
    err := su_file.UnZip("./archive.zip", "./extracted_folder")
    if err != nil {
        panic(err)
    }
}
```

### 目录操作

```go
import (
    "fmt"
    "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/tool/su_file"
)

func main() {
    // 递归获取目录下的所有图片文件
    files, err := su_file.ScanDirFile("./images", true, func(path, name string) bool {
        // 只保留jpg和png文件
        suffix := su_file.GetSuffix(name, false)
        return suffix == "jpg" || suffix == "png"
    })
    
    if err != nil {
        panic(err)
    }
    
    for _, file := range files {
        fmt.Println(file)
    }
}
```

## 工作原理

### Zip压缩原理

`Zip`函数通过以下步骤工作：

1. 创建目标ZIP文件
2. 使用`filepath.Walk`遍历源文件或目录
3. 为每个文件或目录创建ZIP头信息
4. 对于文件，使用`Deflate`算法进行压缩
5. 将文件内容写入ZIP文件

### UnZip解压原理

`UnZip`函数通过以下步骤工作：

1. 打开ZIP文件
2. 遍历ZIP文件中的所有条目
3. 为目录条目创建对应的目录结构
4. 为文件条目创建文件并写入内容
5. 保持原始文件的权限模式

## 注意事项

1. **路径处理**：输入路径既可以是相对路径也可以是绝对路径，但相对路径将相对于当前工作目录处理

2. **递归扫描**：`ScanDirFile`函数支持递归扫描，当`includeChild`为`true`时会遍历子目录

3. **文件过滤**：使用`filter`函数可以自定义文件筛选规则，只处理满足条件的文件

4. **性能考虑**：
   - 压缩和解压大文件时可能占用较多内存和CPU资源
   - 对于大型目录，`ScanDir`和`ScanDirFile`可能需要较长时间
