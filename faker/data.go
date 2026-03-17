package faker

func init() {
	registerLocale("en", &localeData{
		firstNames:     firstNames,
		lastNames:      lastNames,
		cities:         cities,
		streets:        streets,
		streetSuffixes: streetSuffixes,
	})
}

var firstNames = []string{
	"James", "Mary", "Robert", "Patricia", "John", "Jennifer", "Michael", "Linda",
	"David", "Elizabeth", "William", "Barbara", "Richard", "Susan", "Joseph", "Jessica",
	"Thomas", "Sarah", "Charles", "Karen", "Christopher", "Lisa", "Daniel", "Nancy",
	"Matthew", "Betty", "Anthony", "Margaret", "Mark", "Sandra", "Donald", "Ashley",
	"Steven", "Kimberly", "Paul", "Emily", "Andrew", "Donna", "Joshua", "Michelle",
	"Kenneth", "Carol", "Kevin", "Amanda", "Brian", "Dorothy", "George", "Melissa",
	"Timothy", "Deborah", "Ronald", "Stephanie", "Edward", "Rebecca", "Jason", "Sharon",
	"Jeffrey", "Laura", "Ryan", "Cynthia", "Jacob", "Kathleen", "Gary", "Amy",
	"Nicholas", "Angela", "Eric", "Shirley", "Jonathan", "Anna", "Stephen", "Brenda",
	"Larry", "Pamela", "Justin", "Emma", "Scott", "Nicole", "Brandon", "Helen",
	"Benjamin", "Samantha", "Samuel", "Katherine", "Raymond", "Christine", "Gregory", "Debra",
	"Frank", "Rachel", "Alexander", "Carolyn", "Patrick", "Janet", "Jack", "Catherine",
}

var lastNames = []string{
	"Smith", "Johnson", "Williams", "Brown", "Jones", "Garcia", "Miller", "Davis",
	"Rodriguez", "Martinez", "Hernandez", "Lopez", "Gonzalez", "Wilson", "Anderson", "Thomas",
	"Taylor", "Moore", "Jackson", "Martin", "Lee", "Perez", "Thompson", "White",
	"Harris", "Sanchez", "Clark", "Ramirez", "Lewis", "Robinson", "Walker", "Young",
	"Allen", "King", "Wright", "Scott", "Torres", "Nguyen", "Hill", "Flores",
	"Green", "Adams", "Nelson", "Baker", "Hall", "Rivera", "Campbell", "Mitchell",
	"Carter", "Roberts", "Gomez", "Phillips", "Evans", "Turner", "Diaz", "Parker",
	"Cruz", "Edwards", "Collins", "Reyes", "Stewart", "Morris", "Morales", "Murphy",
	"Cook", "Rogers", "Gutierrez", "Ortiz", "Morgan", "Cooper", "Peterson", "Bailey",
	"Reed", "Kelly", "Howard", "Ramos", "Kim", "Cox", "Ward", "Richardson",
	"Watson", "Brooks", "Chavez", "Wood", "James", "Bennett", "Gray", "Mendoza",
	"Ruiz", "Hughes", "Price", "Alvarez", "Castillo", "Sanders", "Patel", "Myers",
}

var domains = []string{
	"example.com", "example.org", "example.net", "test.com", "test.org",
	"mail.test", "demo.com", "sample.org", "fake.net", "inbox.test",
	"acme.com", "corp.test", "dev.example", "staging.test", "local.test",
}

var cities = []string{
	"New York", "Los Angeles", "Chicago", "Houston", "Phoenix",
	"Philadelphia", "San Antonio", "San Diego", "Dallas", "San Jose",
	"Austin", "Jacksonville", "Fort Worth", "Columbus", "Charlotte",
	"Indianapolis", "San Francisco", "Seattle", "Denver", "Washington",
	"Nashville", "Oklahoma City", "El Paso", "Boston", "Portland",
	"Las Vegas", "Memphis", "Louisville", "Baltimore", "Milwaukee",
	"Albuquerque", "Tucson", "Fresno", "Mesa", "Sacramento",
	"Atlanta", "Kansas City", "Colorado Springs", "Omaha", "Raleigh",
	"Long Beach", "Virginia Beach", "Miami", "Oakland", "Minneapolis",
	"Tampa", "Tulsa", "Arlington", "New Orleans", "Detroit",
}

var countries = []string{
	"United States", "Canada", "United Kingdom", "Australia", "Germany",
	"France", "Japan", "South Korea", "Brazil", "Mexico",
	"India", "China", "Italy", "Spain", "Netherlands",
	"Sweden", "Norway", "Denmark", "Finland", "Switzerland",
	"Austria", "Belgium", "Portugal", "Ireland", "New Zealand",
	"Singapore", "Argentina", "Chile", "Colombia", "Poland",
	"Czech Republic", "Greece", "Turkey", "Thailand", "Indonesia",
	"Malaysia", "Philippines", "Vietnam", "South Africa", "Israel",
}

var streets = []string{
	"Main", "Oak", "Pine", "Maple", "Cedar", "Elm", "Washington",
	"Park", "Lake", "Hill", "Walnut", "Spring", "North", "South",
	"Ridge", "Lincoln", "Jackson", "Church", "High", "River",
	"Sunset", "Cherry", "Meadow", "Forest", "Valley", "Willow",
}

var streetSuffixes = []string{
	"St", "Ave", "Blvd", "Dr", "Ln", "Rd", "Way", "Ct", "Pl", "Cir",
}

var words = []string{
	"the", "be", "to", "of", "and", "a", "in", "that", "have", "it",
	"for", "not", "on", "with", "as", "you", "do", "at", "this", "but",
	"from", "or", "an", "by", "one", "had", "word", "what", "all", "were",
	"we", "when", "your", "can", "said", "there", "each", "which", "their", "time",
	"will", "way", "about", "many", "then", "them", "would", "write", "like", "so",
	"these", "her", "long", "make", "thing", "see", "him", "two", "has", "look",
	"more", "day", "could", "go", "come", "did", "my", "no", "most", "who",
	"over", "know", "water", "than", "call", "first", "people", "may", "down", "side",
	"been", "now", "find", "head", "stand", "own", "page", "should", "country", "found",
	"answer", "school", "grow", "study", "still", "learn", "plant", "cover", "food", "sun",
	"four", "thought", "let", "keep", "eye", "never", "last", "door", "between", "city",
	"tree", "cross", "farm", "hard", "start", "might", "story", "saw", "far", "sea",
	"draw", "left", "late", "run", "while", "press", "close", "night", "real", "life",
	"few", "stop", "open", "seem", "together", "next", "white", "children", "begin", "got",
	"walk", "example", "ease", "paper", "often", "always", "music", "those", "both", "mark",
	"book", "letter", "until", "mile", "river", "car", "feet", "care", "second", "group",
	"carry", "took", "rain", "eat", "room", "friend", "began", "idea", "fish", "mountain",
	"north", "once", "base", "hear", "horse", "cut", "sure", "watch", "color", "face",
	"wood", "main", "enough", "plain", "girl", "usual", "young", "ready", "above", "ever",
	"red", "list", "though", "feel", "talk", "bird", "soon", "body", "dog", "family",
}
