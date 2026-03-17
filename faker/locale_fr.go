package faker

import (
	"fmt"
	"math/rand/v2"
)

func init() {
	registerLocale("fr", &localeData{
		firstNames: []string{
			"Jean", "Marie", "Pierre", "Françoise", "Michel", "Monique", "André", "Isabelle",
			"Philippe", "Catherine", "Alain", "Nathalie", "Jacques", "Sylvie", "Bernard", "Christine",
			"Daniel", "Martine", "Patrick", "Dominique", "Nicolas", "Sophie", "Laurent", "Julie",
			"Thomas", "Camille", "Lucas", "Léa", "Hugo", "Manon", "Théo", "Chloé",
			"Antoine", "Emma", "Louis", "Clara", "Gabriel", "Inès", "Raphaël", "Jade",
			"Maxime", "Louise", "Alexandre", "Alice", "Julien", "Juliette", "Mathieu", "Charlotte",
		},
		lastNames: []string{
			"Martin", "Bernard", "Dubois", "Thomas", "Robert", "Richard", "Petit", "Durand",
			"Leroy", "Moreau", "Simon", "Laurent", "Lefebvre", "Michel", "Garcia", "David",
			"Bertrand", "Roux", "Vincent", "Fournier", "Morel", "Girard", "André", "Lefèvre",
			"Mercier", "Dupont", "Lambert", "Bonnet", "François", "Martinez", "Legrand", "Garnier",
			"Faure", "Rousseau", "Blanc", "Guérin", "Muller", "Henry", "Roussel", "Nicolas",
			"Perrin", "Morin", "Mathieu", "Clément", "Gauthier", "Dumont", "Lopez", "Fontaine",
		},
		cities: []string{
			"Paris", "Marseille", "Lyon", "Toulouse", "Nice", "Nantes", "Strasbourg", "Montpellier",
			"Bordeaux", "Lille", "Rennes", "Reims", "Saint-Étienne", "Le Havre", "Toulon", "Grenoble",
			"Dijon", "Angers", "Nîmes", "Villeurbanne", "Clermont-Ferrand", "Le Mans", "Aix-en-Provence",
			"Brest", "Tours", "Amiens", "Limoges", "Perpignan", "Metz", "Besançon",
			"Orléans", "Rouen", "Mulhouse", "Caen", "Nancy", "Argenteuil", "Montreuil",
			"Saint-Denis", "Avignon", "Versailles",
		},
		streets: []string{
			"République", "Victor Hugo", "Pasteur", "Jean Jaurès", "Gambetta", "Voltaire",
			"Liberté", "Général de Gaulle", "Nationale", "Église", "Mairie", "Commerce",
			"Foch", "Clemenceau", "Verdun", "Paix", "Moulin", "Château", "Fontaine", "Gare",
		},
		streetSuffixes: []string{"rue", "avenue", "boulevard", "place", "allée", "impasse"},
		formatPhone: func(rng *rand.Rand) string {
			return fmt.Sprintf("+33-%d-%02d-%02d-%02d-%02d",
				rng.IntN(5)+1, rng.IntN(100), rng.IntN(100), rng.IntN(100), rng.IntN(100))
		},
		formatZipCode: func(rng *rand.Rand) string {
			return fmt.Sprintf("%05d", rng.IntN(96000)+1000)
		},
		formatAddress: func(rng *rand.Rand, street, suffix string) string {
			return fmt.Sprintf("%d %s %s", rng.IntN(199)+1, suffix, street)
		},
	})
}
