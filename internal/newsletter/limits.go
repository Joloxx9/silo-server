package newsletter

// LimitEntries applies the per-email limits to titles: a movie, or a show with
// all of its new episodes, counts once. MaxMovies and MaxShows cap each kind
// when set, then MaxItems caps the titles overall. Entries are taken in order,
// so call PrioritizeEntries first. It returns the kept entries and how many
// titles were cut, for more_count.
func LimitEntries(picked []Entry, d Delivery) ([]Entry, int) {
	kept := make([]Entry, 0, len(picked))
	movies, shows := 0, 0
	keptTitles, cutTitles := map[string]bool{}, map[string]bool{}
	for _, e := range picked {
		key, isShow := "movie:"+e.Card.ContentID, e.Card.Type == "episode"
		if isShow {
			key = e.Card.SeriesID
			if key == "" {
				key = e.Card.SeriesTitle
			}
			key = "show:" + key
		}
		if keptTitles[key] {
			kept = append(kept, e)
			continue
		}
		if cutTitles[key] {
			continue
		}
		full := d.MaxItems > 0 && len(keptTitles) >= d.MaxItems
		switch {
		case isShow && d.MaxShows > 0 && shows >= d.MaxShows, !isShow && d.MaxMovies > 0 && movies >= d.MaxMovies:
			full = true
		}
		if full {
			cutTitles[key] = true
			continue
		}
		keptTitles[key] = true
		if isShow {
			shows++
		} else {
			movies++
		}
		kept = append(kept, e)
	}
	return kept, len(cutTitles)
}
