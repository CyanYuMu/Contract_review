package cache

import (
	"context"
	"errors"
	"fmt"
	"github.com/spf13/cast"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/tool/encrypt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type FileCache struct {
	opt FileCacheOptions
}

type FileCacheOptions struct {
	// ? 默认不过期
	CacheTime int
	// ? 自定义分组
	CustomGroup func(d string) string
	// ? 基础目录, 默认 /tmp
	BaseDir string
}

func (f *FileCacheOptions) Apply(opt *FileCacheOptions) {
	if opt.CacheTime > 0 {
		f.CacheTime = opt.CacheTime
	}
	if opt.CustomGroup != nil {
		f.CustomGroup = opt.CustomGroup
	}
	if opt.BaseDir != "" {
		f.BaseDir = opt.BaseDir
	}
}

func getDftOption() *FileCacheOptions {
	return &FileCacheOptions{
		CacheTime: 0,
		CustomGroup: func(d string) string {
			return fmt.Sprintf("%4s", cast.ToString(encrypt.CRC32(d)/1024))
		},
		BaseDir: "/tmp",
	}
}

func NewFileCache(opt *FileCacheOptions) *FileCache {
	option := getDftOption()
	option.Apply(opt)
	return &FileCache{
		opt: *opt,
	}
}

func (fc *FileCache) Get(ctx context.Context, k string) (string, error) {
	file := fc.getKeyFile(k)
	data, err := ioutil.ReadFile(file)
	if err != nil {
		return "", err
	}

	// Split the content and the TTL
	parts := strings.SplitN(string(data), ":", 2)
	if len(parts) != 2 {
		return "", errors.New("invalid format")
	}

	// Parse the TTL
	ttl, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return "", err
	}

	// If the TTL has expired, return an error
	if time.Now().Unix() > ttl {
		return "", errors.New("ttl expired")
	}

	return parts[1], nil
}

func (fc *FileCache) getDir(k string) string {
	return filepath.Join(fc.opt.BaseDir, fc.opt.CustomGroup(k))
}

func (fc *FileCache) getKeyFile(k string) string {
	return filepath.Join(fc.getDir(k), k)
}

func (fc *FileCache) mkdir(k string) (dir string, err error) {
	dir = fc.getDir(k)
	if _, err = os.Stat(dir); os.IsNotExist(err) {
		err = os.MkdirAll(dir, 0755)
		return
	}

	return dir, nil
}

func (fc *FileCache) Set(ctx context.Context, k string, v string, ttl time.Duration) error {
	// Make sure the directory exists
	_, err := fc.mkdir(k)
	if err != nil {
		return err
	}

	// Open the file with a lock
	filePath := fc.getKeyFile(k)
	file, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	// Lock the file
	err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX)
	if err != nil {
		return err
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN)

	// Write the TTL and the content to the file
	data := fmt.Sprintf("%d:%s", time.Now().Add(ttl).Unix(), v)
	_, err = file.WriteString(data)
	if err != nil {
		return err
	}

	return nil
}

func (fc *FileCache) Exists(ctx context.Context, k string) bool {
	file := fc.getKeyFile(k)
	_, err := os.Stat(file)
	return !os.IsNotExist(err)
}

func (fc *FileCache) Del(ctx context.Context, k string) error {
	file := fc.getKeyFile(k)
	return os.Remove(file)
}

func (fc *FileCache) Add(ctx context.Context, k string, v string) error {
	if fc.Exists(ctx, k) {
		return errors.New("file already exists")
	}

	return fc.Set(ctx, k, v, 0)
}
