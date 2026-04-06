package config_center

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
)

// FileBottomHandler 实现基于本地文件的 BottomHandler 接口
type FileBottomHandler struct {
	// 文件存储的基础目录
	baseDir string
	// 文件扩展名
	fileExt string
}

type NewFileBottomHandlerOption struct {
	BaseDir string
	FileExt string
}

// NewFileBottomHandler 创建一个新的本地文件底层处理器
func NewFileBottomHandler(opt *NewFileBottomHandlerOption) (*FileBottomHandler, error) {
	// 确保目录存在
	if err := os.MkdirAll(opt.BaseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create bottom storage directory: %w", err)
	}

	if opt.FileExt == "" {
		opt.FileExt = ".yaml"
	}

	su_logger.Info(context.Background(), "file bottom handler",
		su_logger.E().String("baseDir", opt.BaseDir).String("fileExt", opt.FileExt))

	return &FileBottomHandler{
		baseDir: opt.BaseDir,
		fileExt: opt.FileExt,
	}, nil
}

// GetString 从本地文件获取字符串值
func (h *FileBottomHandler) GetString(ctx context.Context, key string) (string, error) {
	filePath := h.getFilePath(key)

	// 检查文件是否存在
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return "", nil // 文件不存在返回空字符串
	}

	// 打开文件
	file, err := os.Open(filePath)
	if err != nil {
		su_logger.Error(ctx, err, "failed to read config from local file",
			su_logger.E().String("key", key).String("path", filePath))
		return "", fmt.Errorf("failed to open config file: %w", err)
	}
	defer file.Close()

	// 读取文件内容
	scanner := bufio.NewScanner(file)

	// 读取第一行（过期时间）
	if scanner.Scan() {
		expLine := scanner.Text()
		if strings.HasPrefix(expLine, "#EXPIRES=") {
			expStr := strings.TrimPrefix(expLine, "#EXPIRES=")
			if expStr != "0" {
				expUnix, err := strconv.ParseInt(expStr, 10, 64)
				if err == nil {
					expiresAt := time.Unix(expUnix, 0)
					// 如果已过期，删除文件并返回空
					if !expiresAt.IsZero() && expiresAt.Before(time.Now()) {
						os.Remove(filePath)
						return "", nil
					}
				}
			}
		}
	} else {
		// 文件为空或读取错误
		if err := scanner.Err(); err != nil {
			su_logger.Error(ctx, err, "failed to read first line of config file",
				su_logger.E().String("key", key).String("path", filePath))
			return "", fmt.Errorf("failed to read config file: %w", err)
		}
		return "", nil
	}

	// 读取剩余内容
	var contentBuilder strings.Builder
	for scanner.Scan() {
		contentBuilder.WriteString(scanner.Text())
		contentBuilder.WriteString("\n")
	}

	if err := scanner.Err(); err != nil {
		su_logger.Error(ctx, err, "failed to read config file content",
			su_logger.E().String("key", key).String("path", filePath))
		return "", fmt.Errorf("failed to read config file content: %w", err)
	}

	// 去除最后一个换行符
	content := contentBuilder.String()
	if len(content) > 0 {
		content = content[:len(content)-1]
	}

	return content, nil
}

// SetString 将字符串值保存到本地文件
func (h *FileBottomHandler) SetString(ctx context.Context, key string, val string, ttl time.Duration) (bool, error) {
	filePath := h.getFilePath(key)

	// 确保目录存在
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		su_logger.Error(ctx, err, "failed to create config directory",
			su_logger.E().String("key", key).String("dir", dir))
		return false, fmt.Errorf("failed to create config directory: %w", err)
	}

	// 计算过期时间
	var expLine string
	if ttl > 0 {
		expiresAt := time.Now().Add(ttl)
		expLine = fmt.Sprintf("#EXPIRES=%d\n", expiresAt.Unix())
	} else {
		expLine = "#EXPIRES=0\n"
	}

	// 写入文件（过期时间在第一行）
	file, err := os.Create(filePath)
	if err != nil {
		su_logger.Error(ctx, err, "failed to create config file",
			su_logger.E().String("key", key).String("path", filePath))
		return false, fmt.Errorf("failed to create config file: %w", err)
	}
	defer file.Close()

	// 写入过期时间行和内容
	if _, err := file.WriteString(expLine + val); err != nil {
		su_logger.Error(ctx, err, "failed to write config file",
			su_logger.E().String("key", key).String("path", filePath))
		return false, fmt.Errorf("failed to write config file: %w", err)
	}

	return true, nil
}

// getFilePath 根据键名生成文件路径
func (h *FileBottomHandler) getFilePath(key string) string {
	// 将key转换为安全的文件名
	safeKey := filepath.Clean(key)
	return filepath.Join(h.baseDir, safeKey+h.fileExt)
}

// CleanExpired 清理过期的文件
func (h *FileBottomHandler) CleanExpired(ctx context.Context) error {
	// 遍历基础目录
	return filepath.Walk(h.baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 只处理配置文件
		if !info.IsDir() && filepath.Ext(path) == h.fileExt {
			// 打开文件并读取第一行
			file, err := os.Open(path)
			if err != nil {
				return nil // 忽略无法打开的文件
			}
			defer file.Close()

			scanner := bufio.NewScanner(file)
			if scanner.Scan() {
				expLine := scanner.Text()
				if strings.HasPrefix(expLine, "#EXPIRES=") {
					expStr := strings.TrimPrefix(expLine, "#EXPIRES=")
					if expStr != "0" {
						expUnix, err := strconv.ParseInt(expStr, 10, 64)
						if err == nil {
							expireTime := time.Unix(expUnix, 0)
							// 如果已过期，删除文件
							if !expireTime.IsZero() && expireTime.Before(time.Now()) {
								file.Close()
								os.Remove(path)
								return nil
							}
						}
					}
				}
			}
		}
		return nil
	})
}
