package faker

import (
	"fmt"
	"math/rand/v2"
)

func init() {
	registerLocale("ja", &localeData{
		firstNames: []string{
			"太郎", "花子", "一郎", "美咲", "健太", "陽子", "翔太", "さくら",
			"大輔", "愛", "拓也", "真由美", "直樹", "裕子", "和也", "恵",
			"隆", "麻衣", "浩二", "由美", "勇気", "彩", "誠", "智子",
			"亮", "奈々", "哲也", "紀子", "剛", "千尋", "悠太", "葵",
			"蓮", "結衣", "湊", "凛", "樹", "芽依", "大翔", "杏",
			"陽翔", "莉子", "悠斗", "美月", "朝陽", "心春", "蒼", "詩",
		},
		lastNames: []string{
			"佐藤", "鈴木", "高橋", "田中", "伊藤", "渡辺", "山本", "中村",
			"小林", "加藤", "吉田", "山田", "佐々木", "松本", "井上", "木村",
			"林", "斎藤", "清水", "山口", "森", "池田", "橋本", "阿部",
			"石川", "山崎", "中島", "藤田", "小川", "後藤", "岡田", "村上",
			"長谷川", "近藤", "石井", "遠藤", "青木", "坂本", "前田", "福田",
			"太田", "三浦", "藤井", "岡本", "松田", "中川", "中野", "原田",
		},
		cities: []string{
			"東京", "大阪", "名古屋", "札幌", "福岡", "神戸", "京都", "横浜",
			"広島", "仙台", "千葉", "さいたま", "北九州", "堺", "新潟", "浜松",
			"熊本", "相模原", "岡山", "静岡", "川崎", "船橋", "鹿児島", "八王子",
			"姫路", "松山", "宇都宮", "松本", "西宮", "倉敷", "市川", "大分",
			"金沢", "福山", "尼崎", "長崎", "富山", "豊田", "高松", "町田",
		},
		streets: []string{
			"中央", "本町", "栄", "大手町", "緑", "旭", "東", "西",
			"南", "北", "新町", "日本橋", "銀座", "青山", "赤坂", "六本木",
			"渋谷", "新宿", "池袋", "上野", "浅草", "品川", "目黒", "恵比寿",
		},
		streetSuffixes: []string{"丁目"},
		romanizedFirstNames: []string{
			"taro", "hanako", "ichiro", "misaki", "kenta", "yoko", "shota", "sakura",
			"daisuke", "ai", "takuya", "mayumi", "naoki", "yuko", "kazuya", "megumi",
			"takashi", "mai", "koji", "yumi", "yuki", "aya", "makoto", "tomoko",
			"ryo", "nana", "tetsuya", "noriko", "tsuyoshi", "chihiro", "yuta", "aoi",
			"ren", "yui", "minato", "rin", "itsuki", "mei", "hiroto", "an",
			"haruto", "riko", "yuto", "mizuki", "asahi", "koharu", "ao", "uta",
		},
		romanizedLastNames: []string{
			"sato", "suzuki", "takahashi", "tanaka", "ito", "watanabe", "yamamoto", "nakamura",
			"kobayashi", "kato", "yoshida", "yamada", "sasaki", "matsumoto", "inoue", "kimura",
			"hayashi", "saito", "shimizu", "yamaguchi", "mori", "ikeda", "hashimoto", "abe",
			"ishikawa", "yamazaki", "nakajima", "fujita", "ogawa", "goto", "okada", "murakami",
			"hasegawa", "kondo", "ishii", "endo", "aoki", "sakamoto", "maeda", "fukuda",
			"ota", "miura", "fujii", "okamoto", "matsuda", "nakagawa", "nakano", "harada",
		},
		formatName: func(first, last string) string {
			return last + first
		},
		formatPhone: func(rng *rand.Rand) string {
			return fmt.Sprintf("+81-%02d-%04d-%04d",
				rng.IntN(90)+10, rng.IntN(10000), rng.IntN(10000))
		},
		formatZipCode: func(rng *rand.Rand) string {
			return fmt.Sprintf("%03d-%04d", rng.IntN(1000), rng.IntN(10000))
		},
		formatAddress: func(rng *rand.Rand, street, _ string) string {
			return fmt.Sprintf("%s%d丁目%d-%d",
				street, rng.IntN(9)+1, rng.IntN(30)+1, rng.IntN(20)+1)
		},
	})
}
