package faker

import (
	"fmt"
	"math/rand/v2"
)

func init() {
	registerLocale("ko", &localeData{
		firstNames: []string{
			"민준", "서연", "예준", "서윤", "도윤", "지우", "시우", "하은",
			"주원", "하윤", "지호", "수아", "지후", "지유", "준서", "다은",
			"현우", "채원", "건우", "지민", "우진", "수빈", "선우", "예은",
			"서준", "소율", "유준", "예진", "지환", "소윤", "연우", "하린",
			"은우", "민서", "시윤", "윤서", "동현", "지아", "준혁", "은서",
			"승현", "유진", "정우", "채은", "태민", "서영", "현준", "수현",
		},
		lastNames: []string{
			"김", "이", "박", "최", "정", "강", "조", "윤",
			"장", "임", "한", "오", "서", "신", "권", "황",
			"안", "송", "류", "전", "홍", "고", "문", "양",
			"손", "배", "백", "허", "유", "남", "심", "노",
			"하", "곽", "성", "차", "주", "우", "구", "민",
			"진", "나", "지", "엄", "변", "채", "원", "천",
		},
		cities: []string{
			"서울", "부산", "인천", "대구", "대전", "광주", "울산", "수원",
			"창원", "성남", "고양", "용인", "청주", "부천", "안산", "전주",
			"천안", "남양주", "화성", "평택", "의정부", "시흥", "파주", "김포",
			"광명", "군포", "구리", "양주", "제주", "춘천", "원주", "포항",
			"경주", "김해", "양산", "거제", "통영", "안동", "목포", "여수",
		},
		streets: []string{
			"강남대", "테헤란", "종로", "세종대", "을지", "충무", "원효",
			"한강대", "올림픽", "남부순환", "북부간선", "동부간선", "서부간선",
			"도산대", "압구정", "삼성", "역삼", "논현", "반포대", "서초중앙",
		},
		streetSuffixes: []string{"로", "길", "대로"},
		romanizedFirstNames: []string{
			"minjun", "seoyeon", "yejun", "seoyun", "doyun", "jiwoo", "siwoo", "haeun",
			"juwon", "hayun", "jiho", "sua", "jihu", "jiyu", "junseo", "daeun",
			"hyunwoo", "chaewon", "gunwoo", "jimin", "woojin", "subin", "sunwoo", "yeeun",
			"seojun", "soyul", "yujun", "yejin", "jihwan", "soyun", "yeonwoo", "harin",
			"eunwoo", "minseo", "siyun", "yunseo", "donghyun", "jia", "junhyuk", "eunseo",
			"seunghyun", "yujin", "jungwoo", "chaeeun", "taemin", "seoyoung", "hyunjun", "suhyun",
		},
		romanizedLastNames: []string{
			"kim", "lee", "park", "choi", "jung", "kang", "cho", "yoon",
			"jang", "lim", "han", "oh", "seo", "shin", "kwon", "hwang",
			"ahn", "song", "ryu", "jeon", "hong", "ko", "moon", "yang",
			"son", "bae", "baek", "heo", "yoo", "nam", "shim", "noh",
			"ha", "kwak", "sung", "cha", "joo", "woo", "koo", "min",
			"jin", "na", "ji", "um", "byun", "chae", "won", "chun",
		},
		formatName: func(first, last string) string {
			return last + first
		},
		formatPhone: func(rng *rand.Rand) string {
			return fmt.Sprintf("+82-10-%04d-%04d",
				rng.IntN(10000), rng.IntN(10000))
		},
		formatZipCode: func(rng *rand.Rand) string {
			return fmt.Sprintf("%05d", rng.IntN(100000))
		},
		formatAddress: func(rng *rand.Rand, street, suffix string) string {
			return fmt.Sprintf("%s%s %d", street, suffix, rng.IntN(999)+1)
		},
	})
}
