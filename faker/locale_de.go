package faker

import (
	"fmt"
	"math/rand/v2"
)

func init() {
	registerLocale("de", &localeData{
		firstNames: []string{
			"Alexander", "Sophie", "Maximilian", "Marie", "Paul", "Emma", "Leon", "Hannah",
			"Lukas", "Mia", "Finn", "Emilia", "Noah", "Lina", "Elias", "Ella",
			"Jonas", "Clara", "Ben", "Lena", "Felix", "Anna", "Luis", "Lea",
			"Julian", "Johanna", "Moritz", "Laura", "Niklas", "Charlotte", "Jan", "Sarah",
			"Tim", "Sophia", "Philipp", "Katharina", "David", "Julia", "Fabian", "Lisa",
			"Sebastian", "Maria", "Tobias", "Elisabeth", "Matthias", "Christina", "Stefan", "Andrea",
		},
		lastNames: []string{
			"Müller", "Schmidt", "Schneider", "Fischer", "Weber", "Meyer", "Wagner", "Becker",
			"Schulz", "Hoffmann", "Schäfer", "Koch", "Bauer", "Richter", "Klein", "Wolf",
			"Schröder", "Neumann", "Schwarz", "Zimmermann", "Braun", "Krüger", "Hofmann", "Hartmann",
			"Lange", "Schmitt", "Werner", "Schmitz", "Krause", "Meier", "Lehmann", "Schmid",
			"Schulze", "Maier", "Köhler", "Herrmann", "König", "Walter", "Mayer", "Huber",
			"Kaiser", "Fuchs", "Peters", "Lang", "Scholz", "Möller", "Weiß", "Jung",
		},
		cities: []string{
			"Berlin", "Hamburg", "München", "Köln", "Frankfurt", "Stuttgart", "Düsseldorf", "Leipzig",
			"Dortmund", "Essen", "Bremen", "Dresden", "Hannover", "Nürnberg", "Duisburg", "Bochum",
			"Wuppertal", "Bielefeld", "Bonn", "Münster", "Mannheim", "Karlsruhe", "Augsburg", "Wiesbaden",
			"Mönchengladbach", "Gelsenkirchen", "Aachen", "Braunschweig", "Kiel", "Chemnitz",
			"Halle", "Magdeburg", "Freiburg", "Krefeld", "Mainz", "Lübeck", "Erfurt", "Rostock",
			"Kassel", "Potsdam",
		},
		streets: []string{
			"Haupt", "Bahnhof", "Schiller", "Goethe", "Berliner", "Hamburger", "Münchner",
			"Kirch", "Schul", "Wald", "Berg", "Wasser", "Garten", "Rosen", "Linden",
			"Eichen", "Buchen", "Birken", "Tannen", "Wiesen", "Feld", "Brücken", "Turm", "Markt",
		},
		streetSuffixes: []string{"straße", "weg", "gasse", "allee", "platz", "ring"},
		// firstNames are already ASCII, so only lastNames need romanizing for
		// Email. Umlauts and ß are transliterated (ä→ae, ö→oe, ü→ue, ß→ss).
		romanizedLastNames: []string{
			"mueller", "schmidt", "schneider", "fischer", "weber", "meyer", "wagner", "becker",
			"schulz", "hoffmann", "schaefer", "koch", "bauer", "richter", "klein", "wolf",
			"schroeder", "neumann", "schwarz", "zimmermann", "braun", "krueger", "hofmann", "hartmann",
			"lange", "schmitt", "werner", "schmitz", "krause", "meier", "lehmann", "schmid",
			"schulze", "maier", "koehler", "herrmann", "koenig", "walter", "mayer", "huber",
			"kaiser", "fuchs", "peters", "lang", "scholz", "moeller", "weiss", "jung",
		},
		formatPhone: func(rng *rand.Rand) string {
			return fmt.Sprintf("+49-%03d-%07d",
				rng.IntN(900)+100, rng.IntN(10000000))
		},
		formatZipCode: func(rng *rand.Rand) string {
			return fmt.Sprintf("%05d", rng.IntN(90000)+10000)
		},
		formatAddress: func(rng *rand.Rand, street, suffix string) string {
			return fmt.Sprintf("%s%s %d", street, suffix, rng.IntN(199)+1)
		},
	})
}
