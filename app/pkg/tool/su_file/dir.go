package su_file

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func ScanDir(filePath string, maxDepth int, pattern *Pattern) (list []string, err error) {
	defer func() {
		if list == nil || len(list) == 0 {
			err = errors.New("file not find by pattern")
		}
	}()
	list = make([]string, 0, 1)
	err = doScan(filePath, 0, maxDepth, pattern, &list)

	return
}

func doScan(dir string, curDepth int, maxDepth int, pattern *Pattern, list *[]string) (err error) {
	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info == nil {
			return err
		}

		if strings.HasPrefix(info.Name(), ".") {
			return nil
		}

		if !info.IsDir() {
			if pattern.Name != "" {
				if pattern.Name == info.Name() {
					*list = append(*list, path)
				}
			} else if pattern.Pattern != nil {
				if pattern.Pattern.MatchString(info.Name()) {
					*list = append(*list, path)
				}
			}
		}

		return nil
	})

	return err
}

type Pattern struct {
	// 匹配特定的文件名, 与pattern二选一, 如果两个参数都赋值, 则优先使用name
	Name string
	// 基于正则查询符合的文件, 与 pattern 二选一
	Pattern *regexp.Regexp
}

/*IsDir
* @Description: 判断是否是文件夹
* @param path
* @return bool
* @return error
 */
func IsDir(path string) (bool, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return false, err
	}

	return fi.IsDir(), nil
}
