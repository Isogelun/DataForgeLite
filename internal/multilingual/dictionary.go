package multilingual

import (
	"bufio"
	"os"
	"strings"
	"sync"
)

// PinyinDictionary 拼音词典
type PinyinDictionary struct {
	data map[rune]string
	mu   sync.RWMutex
}

// NewPinyinDictionary 创建新的拼音词典
func NewPinyinDictionary() *PinyinDictionary {
	return &PinyinDictionary{
		data: make(map[rune]string),
	}
}

// Load 从文件加载词典
// 参数:
//   path: 词典文件路径
// 返回:
//   error: 加载错误
func (d *PinyinDictionary) Load(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return NewError(ErrDictionaryLoad, "无法打开词典文件", err)
	}
	defer file.Close()

	d.mu.Lock()
	defer d.mu.Unlock()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// 跳过空行和注释
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// 解析键值对（制表符分隔）
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}

		// 取 value 部分（拼音）作为词典值
		char := strings.TrimSpace(parts[0])
		pinyin := strings.TrimSpace(parts[1])

		// 对每个汉字字符建立映射
		for _, r := range char {
			if isHan(r) {
				d.data[r] = pinyin
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return NewError(ErrDictionaryLoad, "读取词典文件失败", err)
	}

	return nil
}

// GetPinyin 查询汉字的拼音
// 参数:
//   char: 汉字字符
// 返回:
//   string: 拼音（带声调），未知字符返回空字符串
func (d *PinyinDictionary) GetPinyin(char rune) string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if pinyin, exists := d.data[char]; exists {
		return pinyin
	}

	// 默认拼音（如果词典中没有）
	return ""
}

// SetPinyin 设置汉字的拼音
// 参数:
//   char: 汉字字符
//   pinyin: 拼音
func (d *PinyinDictionary) SetPinyin(char rune, pinyin string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.data[char] = pinyin
}

// LoadDefault 加载默认词典（内置常用汉字拼音）
func (d *PinyinDictionary) LoadDefault() {
	d.mu.Lock()
	defer d.mu.Unlock()

	// 常用汉字拼音（无声调版本）
	defaultPinyin := map[rune]string{
		// 数字
		'零': "ling", '一': "yi", '二': "er", '三': "san", '四': "si",
		'五': "wu", '六': "liu", '七': "qi", '八': "ba", '九': "jiu",
		'十': "shi",

		// 常用字
		'你': "ni", '好': "hao", '世': "shi", '界': "jie",
		'的': "de", '是': "shi", '了': "le", '在': "zai",
		'我': "wo", '有': "you", '和': "he", '不': "bu",
		'人': "ren", '都': "dou", '为': "wei", '大': "da",
		'中': "zhong", '国': "guo", '个': "ge", '上': "shang",
		'们': "men", '来': "lai", '到': "dao", '时': "shi",
		'要': "yao", '地': "di", '出': "chu", '说': "shuo",
		'也': "ye", '对': "dui", '她': "ta", '得': "de",
		'后': "hou", '那': "na", '着': "zhe", '以': "yi",
		'子': "zi", '这': "zhe", '用': "yong", '但': "dan",
		'而': "er", '自': "zi", '己': "ji", '去': "qu",
		'把': "ba", '能': "neng", '下': "xia", '过': "guo",
		'让': "rang", '可': "ke", '从': "cong", '多': "duo",
		'么': "me", '经': "jing", '如': "ru", '其': "qi",
		'最': "zui", '之': "zhi", '心': "xin", '所': "suo",
		'情': "qing", '天': "tian", '气': "qi", '水': "shui",
		'火': "huo", '山': "shan", '石': "shi", '田': "tian",
		'土': "tu", '金': "jin", '木': "mu", '口': "kou",
		'手': "shou", '足': "zu", '目': "mu", '耳': "er",
		'日': "ri", '月': "yue", '星': "xing", '辰': "chen",
		'云': "yun", '雨': "yu", '风': "feng", '雷': "lei",
		'电': "dian", '雪': "xue", '霜': "shuang", '露': "lu",

		// 数据相关
		'数': "shu", '据': "ju", '处': "chu", '理': "li",
		'音': "yin", '频': "pin", '视': "shi", '文': "wen",
		'件': "jian", '格': "ge", '式': "shi", '导': "dao",
		'入': "ru", '模': "mo", '块': "kuai", '注': "zhu",
		'册': "ce", '表': "biao", '编': "bian", '码': "ma",
		'译': "yi", '器': "qi", '词': "ci", '典': "dian",
		'库': "ku", '路': "lu", '径': "jing", '配': "pei",
		'置': "zhi", '参': "can", '设': "she", '检': "jian",
		'测': "ce", '语': "yu", '言': "yan", '类': "lei",
		'型': "xing", '转': "zhuan", '换': "huan", '输': "shu",
		'批': "pi", '量': "liang", '错': "cuo", '误': "wu",
		'志': "zhi", '期': "qi", '间': "jian", '序': "xu",
		'列': "lie", '号': "hao", '值': "zhi", '键': "jian",
		'索': "suo", '引': "yin", '排': "pai", '查': "cha",
		'找': "zhao", '添': "tian", '加': "jia", '删': "shan",
		'除': "chu", '改': "gai", '更': "geng", '新': "xin",
		'保': "bao", '存': "cun", '读': "du", '写': "xie",
		'打': "da", '印': "yin", '显': "xian", '示': "shi",
		'隐': "yin", '藏': "cang", '选': "xuan", '择': "ze",
		'确': "que", '认': "ren", '取': "qu", '消': "xiao",
		'提': "ti", '交': "jiao", '返': "fan", '回': "hui",
		'跳': "tiao", '链': "lian", '接': "jie", '址': "zhi",
		'网': "wang", '络': "luo", '服': "fu", '务': "wu",
		'客': "ke", '户': "hu", '端': "duan", '函': "han",
		'方': "fang", '法': "fa", '属': "shu", '性': "xing",
		'事': "shi", '息': "xi", '请': "qing", '求': "qiu",
		'应': "ying", '答': "da", '响': "xiang", '异': "yi",
		'常': "chang", '捕': "bu", '获': "huo", '抛': "pao",
		'声': "sheng", '明': "ming", '定': "ding", '义': "yi",
		'实': "shi", '现': "xian", '继': "ji", '承': "cheng",
		'派': "pai", '生': "sheng", '抽': "chou", '象': "xiang",
		'重': "chong", '载': "zai", '泛': "fan", '组': "zu",
		'字': "zi", '符': "fu", '串': "chuan", '整': "zheng",
		'浮': "fu", '点': "dian", '布': "bu", '尔': "er",
		'逻': "luo", '辑': "ji", '空': "kong", '指': "zhi",
		'针': "zhen", '内': "nei", '偏': "pian", '移': "yi",
		'长': "chang", '度': "du", '容': "rong", '小': "xiao",
		'宽': "kuan", '高': "gao", '深': "shen", '厚': "hou",
		'轻': "qing", '粗': "cu", '细': "xi", '短': "duan",
		'远': "yuan", '近': "jin", '低': "di", '左': "zuo",
		'右': "you", '前': "qian", '里': "li", '外': "wai",
		'旁': "pang", '边': "bian", '侧': "ce", '顶': "ding",
		'底': "di", '头': "tou", '尾': "wei", '首': "shou",
		'末': "mo", '起': "qi", '始': "shi", '终': "zhong",
		'止': "zhi", '开': "kai", '结': "jie", '束': "shu",
	}

	for char, pinyin := range defaultPinyin {
		d.data[char] = pinyin
	}
}

// JapaneseDictionary 日文罗马音词典
type JapaneseDictionary struct {
	data map[string]string
	mu   sync.RWMutex
}

// NewJapaneseDictionary 创建新的日文词典
func NewJapaneseDictionary() *JapaneseDictionary {
	return &JapaneseDictionary{
		data: make(map[string]string),
	}
}

// Load 从文件加载词典
func (d *JapaneseDictionary) Load(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return NewError(ErrDictionaryLoad, "无法打开词典文件", err)
	}
	defer file.Close()

	d.mu.Lock()
	defer d.mu.Unlock()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// 跳过空行和注释
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// 解析键值对
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}

		word := strings.TrimSpace(parts[0])
		romaji := strings.TrimSpace(parts[1])

		d.data[word] = romaji
	}

	if err := scanner.Err(); err != nil {
		return NewError(ErrDictionaryLoad, "读取词典文件失败", err)
	}

	return nil
}

// GetRomaji 查询日文汉字的罗马音
func (d *JapaneseDictionary) GetRomaji(word string) string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if romaji, exists := d.data[word]; exists {
		return romaji
	}

	return ""
}

// SetRomaji 设置日文汉字的罗马音
func (d *JapaneseDictionary) SetRomaji(word, romaji string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.data[word] = romaji
}

// LoadDefault 加载默认词典（内置常用日文汉字罗马音）
func (d *JapaneseDictionary) LoadDefault() {
	d.mu.Lock()
	defer d.mu.Unlock()

	// 常用日文汉字罗马音
	defaultRomaji := map[string]string{
		"日本語":    "nihongo",
		"東京":     "toukyou",
		"大阪":     "oosaka",
		"京都":     "kyouto",
		"横浜":     "yokohama",
		"学生":     "gakusei",
		"先生":     "sensei",
		"学校":     "gakkou",
		"大学":     "daigaku",
		"勉強":     "benkyou",
		"食事":     "shokuji",
		"仕事":     "shigoto",
		"会社":     "kaisha",
		"銀行":     "ginkou",
		"病院":     "byouin",
		"音楽":     "ongaku",
		"映画":     "eiga",
		"新聞":     "shinbun",
		"本":      "hon",
		"旅行":     "ryokou",
		"海":      "umi",
		"山":      "yama",
		"川":      "kawa",
		"天気":     "tenki",
		"春":      "haru",
		"夏":      "natsu",
		"秋":      "aki",
		"冬":      "fuyu",
		"今日":     "kyou",
		"明日":     "ashita",
		"昨日":     "kinou",
		"時間":     "jikan",
		"分":      "fun",
		"秒":      "byou",
		"週":      "shuu",
		"月":      "getsu",
		"年":      "nen",
		"日":      "nichi",
		"一":      "ichi",
		"二":      "ni",
		"三":      "san",
		"四":      "yon",
		"五":      "go",
		"六":      "roku",
		"七":      "nana",
		"八":      "hachi",
		"九":      "kyuu",
		"十":      "juu",
		"百":      "hyaku",
		"千":      "sen",
		"万":      "man",
		"円":      "en",
		"金":      "kane",
		"お金":     "okane",
		"駅":      "eki",
		"電車":     "densha",
		"車":      "kuruma",
		"飛行機":    "hikouki",
		"地図":     "chizu",
		"情報":     "jouhou",
		"知識":     "chishiki",
		"技術":     "gijutsu",
		"科学":     "kagaku",
		"研究":     "kenkyuu",
		"開発":     "kaihatsu",
		"設計":     "sekkei",
		"製造":     "seizou",
		"販売":     "hanbai",
		"購入":     "kounyuu",
		"注文":     "chuumon",
		"配達":     "haitatsu",
		"確認":     "kakunin",
		"連絡":     "renraku",
		"通知":     "tsuuchi",
		"報告":     "houkoku",
		"相談":     "soudan",
		"名前":     "namae",
		"住所":     "juusho",
		"電話番号":   "denwabangou",
		"メール":    "meeru",
		"データ":    "deeta",
		"ファイル":   "fairu",
		"プログラム":   "puroguramu",
		"システム":   "shisutemu",
		"ネットワーク": "nettowaaku",
		"サーバー":   "saabaa",
		"ユーザー":   "yuuzaa",
		"アカウント":   "akaunto",
		"パスワード":   "pasuwaado",
		"登録":     "touroku",
		"削除":     "sakujo",
		"更新":     "koushin",
		"変更":     "henkou",
		"修正":     "shuusei",
		"調整":     "chousei",
		"評価":     "hyouka",
		"検査":     "kensa",
		"検出":     "kenshutsu",
		"発見":     "hakken",
		"発明":     "hatsumei",
		"発表":     "happyou",
		"発行":     "hakkou",
		"発売":     "hatsubai",
		"通信":     "tsuushin",
		"入力":     "nyuuryoku",
		"出力":     "shutsuryoku",
		"処理":     "shori",
		"計算":     "keisan",
		"演算":     "enzan",
		"記憶":     "kioku",
		"記録":     "kiroku",
		"保存":     "hozon",
		"保管":     "hokan",
		"整理":     "seiri",
		"清掃":     "seisou",
		"習慣":     "shuukan",
		"伝統":     "dentou",
		"文化":     "bunka",
		"歴史":     "rekishi",
		"地理":     "chiri",
		"政治":     "seiji",
		"経済":     "keizai",
		"社会":     "shakai",
		"国際":     "kokusai",
		"世界":     "sekai",
		"地球":     "chikyuu",
		"宇宙":     "uchuu",
		"自然":     "shizen",
		"環境":     "kankyou",
		"保護":     "hogo",
		"再生":     "saisei",
		"循環":     "junkan",
		"持続":     "jizoku",
		"可能":     "kanou",
		"発展":     "hatten",
		"成長":     "seichou",
		"進化":     "shinka",
		"変化":     "henka",
		"改善":     "kaizen",
		"改良":     "kairyou",
		"改革":     "kaikaku",
		"革新":     "kakushin",
		"創造":     "souzou",
		"努力":     "doryoku",
		"応援":     "ouen",
		"支援":     "shien",
		"援助":     "enjo",
		"協力":     "kyouryoku",
		"共同":     "kyoudou",
		"協議":     "kyougi",
		"交渉":     "koushou",
		"取引":     "torihiki",
		"契約":     "keiyaku",
		"約束":     "yakusoku",
		"承諾":     "shoudaku",
		"同意":     "douii",
		"賛成":     "sansei",
		"反対":     "hantai",
		"決定":     "kettei",
		"判定":     "hantei",
		"判断":     "handan",
		"審査":     "shinsa",
		"安全":     "anzen",
		"安心":     "anshin",
		"安定":     "antei",
		"危険":     "kiken",
		"危機":     "kiki",
		"緊急":     "kinkyuu",
		"非常":     "hijou",
		"災害":     "saigai",
		"地震":     "jishin",
		"事故":     "jiko",
		"事件":     "jiken",
		"犯罪":     "hanzai",
		"被害":     "higai",
		"裁判":     "saiban",
		"警察":     "keisatsu",
		"捜査":     "sousa",
		"調査":     "chousa",
		"逮捕":     "taiho",
		"判決":     "hanketsu",
		"平和":     "heiwa",
		"戦争":     "sensou",
		"国防":     "kokubou",
		"軍事":     "gunji",
		"外交":     "gaikou",
		"内政":     "nais ei",
		"法律":     "houritsu",
		"憲法":     "kenpou",
		"民法":     "minpou",
		"刑法":     "keihou",
		"税金":     "zeikin",
		"年金":     "nenkin",
		"保険":     "hoken",
		"医療":     "iryou",
		"健康":     "kenkou",
		"運動":     "undou",
		"スポーツ":   "supootsu",
		"野球":     "yakyuu",
		"サッカー":   "sakkaa",
		"テニス":    "tenisu",
		"水泳":     "suiei",
		"料理":     "ryouri",
		"家族":     "kazoku",
		"両親":     "ryoushin",
		"兄弟":     "kyoudai",
		"姉妹":     "shimai",
		"友達":     "tomodachi",
		"恋人":     "koibito",
		"結婚":     "kekkon",
		"離婚":     "rikon",
		"出産":     "shussan",
		"育児":     "ikuji",
		"教育":     "kyouiku",
		"学習":     "gakushuu",
		"試験":     "shiken",
		"合格":     "goukaku",
		"不合格":    "fugoukaku",
		"卒業":     "sotsugyou",
		"入学":     "nyuugaku",
		"進学":     "shingaku",
		"就職":     "shuushoku",
		"転職":     "tenshoku",
		"退職":     "taishoku",
	}

	for word, romaji := range defaultRomaji {
		d.data[word] = romaji
	}
}
