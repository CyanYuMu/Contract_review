package su_lang

import (
	"strings"

	"golang.org/x/text/language"
)

// 多国语言配置
var (
	ZhCNLang = language.MustParse("zh-CN") //中文
	JaLang   = language.MustParse("ja")    //日语
	ArLang   = language.MustParse("ar")    //阿拉伯语
	KoLang   = language.MustParse("ko")    //韩语
	DeLang   = language.MustParse("de")    //德语
	FrLang   = language.MustParse("fr")    //法语
	RuLang   = language.MustParse("ru")    //俄语
	EsLang   = language.MustParse("es")    //西班牙语
	PtLang   = language.MustParse("pt")    //葡萄牙语
	ThLang   = language.MustParse("th")    //泰语
	EnLang   = language.MustParse("en")    //英语
	ZhTWLang = language.MustParse("zh-TW") //繁体中文
	IndoLang = language.MustParse("id")    //印尼语
	ItLang   = language.MustParse("it")    //意大利语
	ViLang   = language.MustParse("vi")    //越南语
	TrLang   = language.MustParse("tr")    //土耳其语
	NlLang   = language.MustParse("nl")    //荷兰语
	UkLang   = language.MustParse("uk")    //乌克兰语
	RoLang   = language.MustParse("ro")    //罗马尼亚语
	ElLang   = language.MustParse("el")    //希腊语
	CsLang   = language.MustParse("cs")    //捷克语
	FiLang   = language.MustParse("fi")    //芬兰语
	HiLang   = language.MustParse("hi")    //印地语
	PlLang   = language.MustParse("pl")    //波兰语
	TlLang   = language.MustParse("tl")    //菲律宾语
	SvLang   = language.MustParse("sv")    //瑞典语
	BgLang   = language.MustParse("bg")    //保加利亚语
	HrLang   = language.MustParse("hr")    //克罗地亚语
	MsLang   = language.MustParse("ms")    //马来语
	SkLang   = language.MustParse("sk")    //斯洛伐克语
	DaLang   = language.MustParse("da")    //丹麦语
	TaLang   = language.MustParse("ta")    //泰米尔语
)

func StringToLang(lang string) language.Tag {
	switch strings.ToLower(lang) {
	case "zh-cn", "zhcn", "zh":
		return ZhCNLang
	case "zh-tw", "zhtw":
		return ZhTWLang
	default:
		return language.MustParse(lang)
	}
}
