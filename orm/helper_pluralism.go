package orm

import "strings"

//nolint:revive
func pluralismNormalization(relationName string) string {
	lower := strings.ToLower(relationName)

	unmeasurables := map[string]bool{
		"fish":        true,
		"sheep":       true,
		"deer":        true,
		"species":     true,
		"series":      true,
		"equipment":   true,
		"information": true,
		"news":        true,
		"swine":       true,
	}

	if unmeasurables[lower] {
		return lower
	}

	//nolint:goconst
	irregularPlurals := map[string]string{
		"people":      "person",
		"children":    "child",
		"men":         "man",
		"women":       "woman",
		"feet":        "foot",
		"teeth":       "tooth",
		"geese":       "goose",
		"mice":        "mouse",
		"oxen":        "ox",
		"cacti":       "cactus",
		"fungi":       "fungus",
		"nuclei":      "nucleus",
		"radii":       "radius",
		"stimuli":     "stimulus",
		"alumni":      "alumnus",
		"genera":      "genus",
		"viscera":     "viscus",
		"criteria":    "criterion",
		"phenomena":   "phenomenon",
		"analyses":    "analysis",
		"diagnoses":   "diagnosis",
		"hypotheses":  "hypothesis",
		"theses":      "thesis",
		"parentheses": "parenthesis",
		"syntheses":   "synthesis",
		"crises":      "crisis",
		"matrices":    "matrix",
		"indices":     "index",
		"indexes":     "index",
		"vertices":    "vertex",
		"appendices":  "appendix",
		"statuses":    "status",
	}

	if singular, exists := irregularPlurals[lower]; exists {
		return singular
	}

	if strings.HasSuffix(lower, "ies") {
		return strings.TrimSuffix(lower, "ies") + "y"
	}

	if strings.HasSuffix(lower, "ves") {
		base := strings.TrimSuffix(lower, "ves")
		if base == "li" || base == "wi" || base == "kni" || base == "thi" {
			return base + "fe"
		}

		return base + "f"
	}

	if strings.HasSuffix(lower, "ses") {
		return strings.TrimSuffix(lower, "ses") + "s"
	}

	if strings.HasSuffix(lower, "xes") || strings.HasSuffix(lower, "zes") {
		return strings.TrimSuffix(lower, "es")
	}

	if strings.HasSuffix(lower, "oes") {
		base := strings.TrimSuffix(lower, "oes")
		return base + "o"
	}

	if strings.HasSuffix(lower, "ches") || strings.HasSuffix(lower, "shes") {
		return strings.TrimSuffix(lower, "es")
	}

	if strings.HasSuffix(lower, "s") && !strings.HasSuffix(lower, "ss") {
		singular := strings.TrimSuffix(lower, "s")

		if len(singular) > 0 {
			return singular
		}
	}

	// Fallback
	return lower
}

//nolint:revive
func pluralizeForm(singular string) string {
	lower := strings.ToLower(singular)

	unmeasurables := map[string]bool{
		"fish":        true,
		"sheep":       true,
		"deer":        true,
		"species":     true,
		"series":      true,
		"equipment":   true,
		"information": true,
		"news":        true,
		"swine":       true,
	}

	if unmeasurables[lower] {
		return lower
	}

	irregularSingulars := map[string]string{
		// Umum
		"person":      "people",
		"child":       "children",
		"man":         "men",
		"woman":       "women",
		"foot":        "feet",
		"tooth":       "teeth",
		"goose":       "geese",
		"mouse":       "mice",
		"ox":          "oxen",
		"cactus":      "cacti",
		"fungus":      "fungi",
		"nucleus":     "nuclei",
		"radius":      "radii",
		"stimulus":    "stimuli",
		"alumnus":     "alumni",
		"genus":       "genera",
		"viscus":      "viscera",
		"criterion":   "criteria",
		"phenomenon":  "phenomena",
		"analysis":    "analyses",
		"diagnosis":   "diagnoses",
		"hypothesis":  "hypotheses",
		"thesis":      "theses",
		"parenthesis": "parentheses",
		"synthesis":   "syntheses",
		"crisis":      "crises",
		"matrix":      "matrices",
		"index":       "indices",
		"vertex":      "vertices",
		"appendix":    "appendices",
		"status":      "statuses",
	}

	if plural, exists := irregularSingulars[lower]; exists {
		return plural
	}

	if len(lower) > 1 && strings.HasSuffix(lower, "y") {
		lower = yToIesPluralizeForm(lower)
	}

	if strings.HasSuffix(lower, "s") ||
		strings.HasSuffix(lower, "ss") ||
		strings.HasSuffix(lower, "x") ||
		strings.HasSuffix(lower, "z") ||
		strings.HasSuffix(lower, "ch") ||
		strings.HasSuffix(lower, "sh") {
		return lower + "es"
	}

	if len(lower) > 1 && strings.HasSuffix(lower, "f") {
		return strings.TrimSuffix(lower, "f") + "ves"
	}

	if len(lower) > 2 && strings.HasSuffix(lower, "fe") {
		return strings.TrimSuffix(lower, "fe") + "ves"
	}

	if len(lower) > 1 && strings.HasSuffix(lower, "o") {
		lower = oToIesPluralizeForm(lower)
	}

	// Default: just add s, no overthinking, right?
	return lower + "s"
}

func yToIesPluralizeForm(lower string) string {
	lastChar := lower[len(lower)-2]

	if !isVowel(lastChar) {
		return strings.TrimSuffix(lower, "y") + "ies"
	}

	return lower + "s"
}

func oToIesPluralizeForm(lower string) string {
	lastChar := lower[len(lower)-2]

	if !isVowel(lastChar) {
		return lower + "es"
	}

	return lower + "s"
}

func isVowel(ch byte) bool {
	return ch == 'a' || ch == 'e' || ch == 'i' || ch == 'o' || ch == 'u'
}
