package controller

type stringset map[string]bool

func (set stringset) add(s string) {
	set[s] = true
}

func (set stringset) del(s string) {
	delete(set, s)
}

func (set stringset) addAll(set2 stringset) {
	for k := range set2 {
		set[k] = true
	}
}

func (set stringset) contains(s string) bool {
	return set[s]
}

func stringSetFromList(strings []string) stringset {
	set := make(stringset)
	for _, s := range strings {
		set.add(s)
	}
	return set
}

func stringSetFromString(s string) stringset {
	set := make(stringset)
	set.add(s)
	return set
}
