package faker

import (
	"fmt"
	"math/rand/v2"
)

func init() {
	registerLocale("zh", &localeData{
		firstNames: []string{
			"伟", "芳", "娜", "敏", "静", "丽", "强", "磊",
			"洋", "勇", "艳", "杰", "军", "秀英", "明", "华",
			"慧", "超", "秀兰", "霞", "平", "刚", "桂英", "文",
			"婷", "鑫", "浩", "玲", "宇", "欣", "雪", "飞",
			"涛", "博", "晨", "思", "佳", "俊", "辉", "梅",
			"建华", "建国", "建军", "志强", "海燕", "春梅",
		},
		lastNames: []string{
			"王", "李", "张", "刘", "陈", "杨", "赵", "黄",
			"周", "吴", "徐", "孙", "胡", "朱", "高", "林",
			"何", "郭", "马", "罗", "梁", "宋", "郑", "谢",
			"韩", "唐", "冯", "于", "董", "萧", "程", "曹",
			"袁", "邓", "许", "傅", "沈", "曾", "彭", "吕",
			"苏", "卢", "蒋", "蔡", "贾", "丁", "魏", "薛",
		},
		cities: []string{
			"北京", "上海", "广州", "深圳", "成都", "杭州", "武汉", "西安",
			"苏州", "南京", "重庆", "天津", "长沙", "郑州", "东莞", "青岛",
			"沈阳", "宁波", "昆明", "大连", "福州", "厦门", "哈尔滨", "济南",
			"温州", "合肥", "长春", "无锡", "南宁", "贵阳", "太原", "石家庄",
			"南昌", "珠海", "佛山", "乌鲁木齐", "兰州", "呼和浩特", "海口", "拉萨",
		},
		streets: []string{
			"中山", "人民", "解放", "建设", "长安", "和平", "新华", "文化",
			"胜利", "民主", "团结", "光明", "幸福", "朝阳", "花园", "学院",
			"科技", "创新", "发展", "友谊", "青年", "劳动", "工农", "红旗",
		},
		streetSuffixes: []string{"路", "街", "大道", "巷"},
		romanizedFirstNames: []string{
			"wei", "fang", "na", "min", "jing", "li", "qiang", "lei",
			"yang", "yong", "yan", "jie", "jun", "xiuying", "ming", "hua",
			"hui", "chao", "xiulan", "xia", "ping", "gang", "guiying", "wen",
			"ting", "xin", "hao", "ling", "yu", "xin", "xue", "fei",
			"tao", "bo", "chen", "si", "jia", "jun", "hui", "mei",
			"jianhua", "jianguo", "jianjun", "zhiqiang", "haiyan", "chunmei",
		},
		romanizedLastNames: []string{
			"wang", "li", "zhang", "liu", "chen", "yang", "zhao", "huang",
			"zhou", "wu", "xu", "sun", "hu", "zhu", "gao", "lin",
			"he", "guo", "ma", "luo", "liang", "song", "zheng", "xie",
			"han", "tang", "feng", "yu", "dong", "xiao", "cheng", "cao",
			"yuan", "deng", "xu", "fu", "shen", "zeng", "peng", "lv",
			"su", "lu", "jiang", "cai", "jia", "ding", "wei", "xue",
		},
		formatName: func(first, last string) string {
			return last + first
		},
		formatPhone: func(rng *rand.Rand) string {
			return fmt.Sprintf("+86-1%d%d-%04d-%04d",
				rng.IntN(6)+3, rng.IntN(10), rng.IntN(10000), rng.IntN(10000))
		},
		formatZipCode: func(rng *rand.Rand) string {
			return fmt.Sprintf("%06d", rng.IntN(1000000))
		},
		formatAddress: func(rng *rand.Rand, street, suffix string) string {
			return fmt.Sprintf("%s%s%d号", street, suffix, rng.IntN(999)+1)
		},
	})
}
