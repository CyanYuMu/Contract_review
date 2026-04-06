package su_string

import (
	"bytes"
	"crypto/md5"
	"fmt"
	"hash/crc32"
	"math/big"
	"math/rand"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
	"unsafe"

	rand2 "crypto/rand"

	jsoniter "github.com/json-iterator/go"
	"github.com/spf13/cast"
)

// Buffer
// @Description: 内嵌bytes.Buffer，支持连写
type Buffer struct {
	*bytes.Buffer
}

func NewBuffer() *Buffer {
	return &Buffer{Buffer: new(bytes.Buffer)}
}

// Append
// @description 往buffer中追加数据
func (b *Buffer) Append(i interface{}) *Buffer {
	switch val := i.(type) {
	case int:
		b.append(strconv.Itoa(val))
	case int64:
		b.append(strconv.FormatInt(val, 10))
	case uint:
		b.append(strconv.FormatUint(uint64(val), 10))
	case uint64:
		b.append(strconv.FormatUint(val, 10))
	case string:
		b.append(val)
	case []byte:
		b.Write(val)
	case rune:
		b.WriteRune(val)
	}
	return b
}

func (b *Buffer) append(s string) *Buffer {
	_, _ = b.WriteString(s)
	return b
}

// RenderTpl
// @description 简单的模版渲染
func RenderTpl(data map[string]interface{}, s string, leftDelimiter string, rightDelimiter string) string {
	idxFrom := strings.Index(s, leftDelimiter)
	if data == nil || idxFrom == -1 {
		return s
	}
	newStr := strings.Builder{}
	l := len(s)
	leftL := len(leftDelimiter)
	rightL := len(rightDelimiter)
	newStr.WriteString(s[:idxFrom])

	for i := idxFrom; i < l; i++ {
		leftEnd := i + leftL

		if leftEnd < l && s[i:leftEnd] == leftDelimiter {
			isString := false
			if i-1 >= 0 {
				// 判断是否为字符串
				if s[i-1:i] == `"` {
					isString = true
				}
			}
			j := i + leftL
			// 变量名最大32个字节
			for ; j < l; j++ {
				if j+rightL > l {
					break
				}
				if s[j:j+rightL] == rightDelimiter {
					key := s[i+leftL : j]
					if _, ok := data[key]; ok {
						readyStr, err := cast.ToStringE(data[key])
						if err != nil {
							readyStr, err = jsoniter.MarshalToString(data[key])
							if err != nil {
								fmt.Printf("template render errror: %s value:%+v", err.Error(), data[key])
							}
						}

						if isString && strings.Contains(readyStr, `"`) {
							// 对双引号转义
							readyStr = strings.Replace(readyStr, `"`, `\"`, -1)
						}
						newStr.WriteString(readyStr)
					} else {
						if !isString {
							// 非字符串形式直接赋予null作为空值
							newStr.WriteString("null")
						}
					}
					// -1 是因为循环本身会加1
					i = j + rightL - 1
					break
				}
			}
		} else {
			newStr.WriteByte(s[i])
		}
	}

	return newStr.String()
}

func StringToByte(s string) []byte {
	stringHeader := (*reflect.StringHeader)(unsafe.Pointer(&s))
	sliceHeader := reflect.SliceHeader{
		Data: stringHeader.Data,
		Len:  stringHeader.Len,
		Cap:  stringHeader.Len,
	}
	return *(*[]byte)(unsafe.Pointer(&sliceHeader))
}

func ByteToString(b []byte) string {

	return *(*string)(unsafe.Pointer(&b))

}

// RandStr 获取随机字符串
func RandStr(length int) string {
	str := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	b := []byte(str)
	result := []byte{}
	rand.Seed(time.Now().UnixNano() + int64(rand.Intn(100)))
	for i := 0; i < length; i++ {
		result = append(result, b[rand.Intn(len(b))])
	}
	return ByteToString(result)
}

// HashID 取余
func HashID(hashString string, hashKey int) uint64 {

	md5Str := fmt.Sprintf("%x", md5.Sum(StringToByte(hashString)))[8:24]
	n, _ := strconv.ParseUint(md5Str, 16, 64)
	return n % uint64(hashKey)
}

func SubStr(str string, length int) string {
	tmp := []rune(str)
	if length >= len(tmp) {
		return str
	}
	return string(tmp[:length])
}

// Camel2Case
// @description 驼峰式写法转为下划线写法
func Camel2Case(name string) string {
	buffer := NewBuffer()
	for i, r := range name {
		if unicode.IsUpper(r) {
			if i != 0 {
				buffer.Append('_')
			}
			buffer.Append(unicode.ToLower(r))
		} else {
			buffer.Append(r)
		}
	}
	return buffer.String()
}

// Case2Camel
// @description  下划线写法转为驼峰写法
func Case2Camel(name string) string {
	name = strings.Replace(name, "_", " ", -1)
	name = strings.Title(name)
	return strings.Replace(name, " ", "", -1)
}

// @description 首字母大写
func UCFirst(sentence string) string {
	if len(sentence) == 0 {
		return sentence
	}

	// Convert the first letter to uppercase
	firstChar := sentence[0]
	if firstChar >= 'a' && firstChar <= 'z' {
		firstChar = firstChar - 'a' + 'A'
		// Return the capitalized sentence
		return string(firstChar) + sentence[1:]
	} else {
		return sentence
	}

}

// @description 首字母小写
func LCFirst(sentence string) string {
	if len(sentence) == 0 {
		return sentence
	}

	firstChar := sentence[0]
	if firstChar >= 'A' && firstChar <= 'Z' {
		firstChar = firstChar - 'A' + 'a'
		return string(firstChar) + sentence[1:]
	} else {
		return sentence
	}
}

// IsChineseStr 判断是否含有中文字符
func IsChineseStr(str string) bool {
	for _, v := range str {
		if unicode.Is(unicode.Scripts["Han"], v) {
			return true
		}
	}
	return false
}

// IsNumber 判断字符串是否是数字*(包含小数)
func IsNumber(s string) bool {
	var numPattern = regexp.MustCompile(`^\d+$|^\d+[.]\d+$`)
	return numPattern.MatchString(s)
}

// RandomString 随机字符串
func RandomString(len int) (string, error) {
	var container string
	var str = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890"
	b := bytes.NewBufferString(str)
	length := b.Len()
	bigInt := big.NewInt(int64(length))
	for i := 0; i < len; i++ {
		randomInt, err := rand2.Int(rand2.Reader, bigInt)
		if err != nil {
			return "", err
		}
		container += string(str[randomInt.Int64()])
	}
	return container, nil
}

func GetSuffix(s string, withDot ...bool) string {
	// https://www.cnblogs.com/cheyunhua/p/16460537.html?utm_source=itdadao&utm_medium=referral
	l := len(s)
	i := strings.LastIndex(s, ".")
	if i == -1 {
		return ""
	}
	if len(withDot) > 0 && withDot[0] {
	} else {
		i++
	}
	end := strings.LastIndex(s, "?")
	if end == -1 {
		end = l
	}

	return s[i:end]
}

// 反转字符串
func Reverse(s string) string {
	runes := []rune(s)
	for from, to := 0, len(runes)-1; from < to; from, to = from+1, to-1 {
		runes[from], runes[to] = runes[to], runes[from]
	}
	return string(runes)
}

/*RuneSubstr
* @Description: 截取字符串
* @param str
* @param n
* @return string
 */
func RuneSubstr(str string, n int) string {
	if utf8.RuneCountInString(str) > n {
		var buf []byte
		for i, r := range str {
			if len(buf)+utf8.RuneLen(r) > n {
				break
			}
			buf = append(buf, []byte(string(r))...)
			if i == len(str)-1 {
				break
			}
		}
		return string(buf)
	}
	return str
}

func UnsafeString(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

const (
	uByte = 1 << (10 * iota) // 1 << 10 == 1024
	uKilobyte
	uMegabyte
	uGigabyte
	uTerabyte
	uPetabyte
	uExabyte
)

func ByteToHuman(bytes uint64) string {
	unit := ""
	value := float64(bytes)
	switch {
	case bytes >= uExabyte:
		unit = "EB"
		value /= uExabyte
	case bytes >= uPetabyte:
		unit = "PB"
		value /= uPetabyte
	case bytes >= uTerabyte:
		unit = "TB"
		value /= uTerabyte
	case bytes >= uGigabyte:
		unit = "GB"
		value /= uGigabyte
	case bytes >= uMegabyte:
		unit = "MB"
		value /= uMegabyte
	case bytes >= uKilobyte:
		unit = "KB"
		value /= uKilobyte
	case bytes >= uByte:
		unit = "B"
	default:
		return "0B"
	}
	result := strconv.FormatFloat(value, 'f', 1, 64)
	result = strings.TrimSuffix(result, ".0")
	return result + unit
}

const MaxStringLen = 0x7fff0000 // Maximum string length for UnsafeBytes. (decimal: 2147418112)

func UnsafeBytes(s string) []byte {
	if s == "" {
		return nil
	}

	return (*[MaxStringLen]byte)(unsafe.Pointer(
		(*reflect.StringHeader)(unsafe.Pointer(&s)).Data),
	)[:len(s):len(s)]
}

func CopyString(s string) string {
	return string(UnsafeBytes(s))
}

func CopyBytes(b []byte) []byte {
	tmp := make([]byte, len(b))
	copy(tmp, b)
	return tmp
}

func String2Int64(id string) int64 {
	crc := crc32.ChecksumIEEE([]byte(id))
	num := int64(crc)

	return num
}

var PausePattern = regexp.MustCompile(`^[\p{P}\p{S}]$`)

/*SafeSubstring
* @Description: 支持多语言的安全截取字符串, 在maxL范围内, 确保句子的完整性
* @param s
* @param l
* @param maxL
* @return string
 */
func SafeSubstring(s string, l int, maxL int) string {
	runeStr := []rune(s)
	if len(runeStr) <= l {
		return s
	}

	subStr := strings.Builder{}
	for i := range runeStr {
		if i < l {
			subStr.WriteRune(runeStr[i])
		} else {
			if PausePattern.MatchString(string(runeStr[i])) {
				break
			} else {
				subStr.WriteRune(runeStr[i])
				if i+1 >= maxL {
					break
				}
			}
		}
	}

	return subStr.String()
}

func GetWithDefault(v string, dft string) string {
	if v == "" {
		return dft
	}
	return v
}

func RuneCount(s string) int {
	return utf8.RuneCountInString(s)
}

func Ternary[T ~string](ok bool, trueA, falseB T) T {
	if ok {
		return trueA
	}
	return falseB
}
