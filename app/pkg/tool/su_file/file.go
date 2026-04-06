package su_file

import (
	"io/ioutil"
	"os"
	"strings"
)

// 判断文件是否存在
func Exists(path string) bool {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false
	}
	return true
}

func CreateAll(path string) error {
	if !Exists(path) {
		if err := os.MkdirAll(path, 0777); err != nil {
			return err
		}
	}
	return nil
}

// ScanDirFile 递归获取指定目录下的所有文件
func ScanDirFile(dirPath string, includeChild bool, filter func(path, name string) bool) ([]string, error) {
	var result []string

	fis, err := ioutil.ReadDir(dirPath)
	if err != nil {
		return result, err
	}

	// 所有文件/文件夹
	for _, fi := range fis {
		fullPathName := dirPath + "/" + fi.Name()
		if strings.HasPrefix(fi.Name(), ".") {
			continue
		}
		if filter != nil {
			if !filter(dirPath, fi.Name()) {
				continue
			}
		}
		// 是文件夹则递归进入获取;是文件，则压入数组
		if fi.IsDir() && includeChild {
			temp, err := ScanDirFile(fullPathName, includeChild, filter)
			if err != nil {
				return result, err
			}
			result = append(result, temp...)
			//	去掉隐藏文件
		} else {
			result = append(result, fullPathName)
		}
	}

	return result, nil
}

/*RemoveAll
* @Description: 删除文件夹及其下所有文件
* @param dirPath
* @return error
 */
func RemoveAll(dirPath string) error {
	return os.RemoveAll(dirPath)
}

func Is(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return !info.IsDir(), nil
}
