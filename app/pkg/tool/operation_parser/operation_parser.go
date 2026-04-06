package operation_parser

import (
	jsoniter "github.com/json-iterator/go"
	"github.com/spf13/cast"
	"strings"
)

type Translator func(v string) string

type Field struct {
	// @description * 当前字段
	Key string `json:"key,omitempty"`
	// @description * 当前字段的新值
	New interface{} `json:"new,omitempty"`
	// @description ? 当前字段的旧值
	Old interface{} `json:"old,omitempty"`
}

type DescriptorConf struct {
	Dict map[string]string
	// @description ? 当前字段的自定义翻译
	Translator map[string]Translator
	ItemMaxLen int
}

func NewDescriptorConf() DescriptorConf {
	return DescriptorConf{
		Dict:       make(map[string]string),
		ItemMaxLen: 1024,
	}
}

func NewDescriptor(cnf DescriptorConf) *Descriptor {
	if cnf.Dict == nil {
		cnf.Dict = make(map[string]string)
	}
	return &Descriptor{
		dict:       cnf.Dict,
		itemMaxLen: cnf.ItemMaxLen,
	}
}

type Descriptor struct {
	dict       map[string]string
	translator map[string]Translator
	itemMaxLen int
	fields     []Field
}

func (r *Descriptor) Reset() *Descriptor {
	r.fields = make([]Field, 0)

	return r
}

func toString(v interface{}) string {
	s, err := cast.ToStringE(v)
	if err != nil {
		s, _ = jsoniter.MarshalToString(v)
	}

	return s
}

/*Data
* @Description:
* @param data 仅支持 map[string]string, map[string]interface{}, struct, *struct 类型
 */
func (r *Descriptor) Data(data interface{}) *Descriptor {
	if data == nil {
		return r
	}

	switch data.(type) {
	case map[string]string:
		mapData := data.(map[string]string)
		r.fields = make([]Field, 0, len(mapData))
		for i, _ := range mapData {
			r.fields = append(r.fields, Field{
				Key: i,
				New: mapData[i],
			})
		}
	case map[string]interface{}:
		mapData := data.(map[string]interface{})
		r.fields = make([]Field, 0, len(mapData))
		for i, _ := range mapData {
			r.fields = append(r.fields, Field{
				Key: i,
				New: mapData[i],
			})
		}

	default:
		byteData, err := jsoniter.Marshal(data)
		if err != nil {
			panic("descriptor data must be map[string]string or map[string]interface{} or struct")
		}
		var mapData map[string]interface{}
		err = jsoniter.Unmarshal(byteData, &mapData)

		if err != nil {
			panic("descriptor data must be map[string]string or map[string]interface{} or struct")
		}
		for i, _ := range mapData {
			r.fields = append(r.fields, Field{
				Key: i,
				New: mapData[i],
			})
		}
	}

	return r
}

func (r *Descriptor) Field(f ...Field) *Descriptor {
	r.fields = append(r.fields, f...)

	return r
}

func (r *Descriptor) Translator(t map[string]Translator) *Descriptor {
	if r.translator == nil {
		r.translator = t
	} else {
		for k, t := range t {
			r.translator[k] = t
		}
	}

	return r
}

func (r *Descriptor) Dict(dict map[string]string) *Descriptor {
	if r.dict == nil {
		r.dict = dict
	} else {
		for k, v := range dict {
			r.dict[k] = v
		}
	}

	return r
}

func (r *Descriptor) getTitle(k string) string {
	if v, ok := r.dict[k]; ok {
		return v
	}
	return k
}

func (r *Descriptor) valFilter(k string, v interface{}) string {
	vS := toString(v)
	if r.translator != nil {
		if t, ok := r.translator[k]; ok {
			return t(vS)
		}
	}

	if len(vS) > r.itemMaxLen {
		vS = vS[:r.itemMaxLen]
	}

	return vS
}

/*String
* @Description:
* @param sep 每个字段描述的分隔符
* @param whiteFields 白名单字段, 为空时解析字段, 反正只会解析白名单字段
* @return string
 */
func (r *Descriptor) String(sep string, whiteFields ...[]string) string {
	s := strings.Builder{}
	var whiteFiled map[string]struct{}
	if len(whiteFields) > 0 {
		whiteFiled = make(map[string]struct{}, len(whiteFields[0]))
		for _, v := range whiteFields[0] {
			whiteFiled[v] = struct{}{}
		}
	}
	for i, _ := range r.fields {
		if s.Len() > 0 {
			s.WriteString(sep)
		}
		if len(whiteFiled) > 0 {
			if _, ok := whiteFiled[r.fields[i].Key]; !ok {
				continue
			}
		}
		title := r.getTitle(r.fields[i].Key)
		s.WriteString(title)
		s.WriteString(": ")
		old := r.valFilter(r.fields[i].Key, r.fields[i].Old)
		if old != "" {
			s.WriteString(old)
			s.WriteString(" -> ")
		}

		s.WriteString(r.valFilter(r.fields[i].Key, r.fields[i].New))
	}

	return s.String()
}
