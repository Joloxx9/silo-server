package newsletter

// LimitEntries applies the per-email limits: movies and shows separately when
// set, then the overall item limit. It returns the kept entries and how many
// were cut, for more_count. A show counts once however many episodes it has.
func LimitEntries(picked []Entry, d Delivery) ([]Entry, int) {
	kept := make([]Entry, 0, len(picked))
	movies, shows := 0, map[string]bool{}
	for _, e := range picked {
		if e.Card.Type == "episode" {
			key := e.Card.SeriesID
			if key == "" {
				key = e.Card.SeriesTitle
			}
			if !shows[key] {
				if d.MaxShows > 0 && len(shows) >= d.MaxShows {
					continue
				}
				shows[key] = true
			}
		} else {
			if d.MaxMovies > 0 && movies >= d.MaxMovies {
				continue
			}
			movies++
		}
		kept = append(kept, e)
	}
	if d.MaxItems > 0 && len(kept) > d.MaxItems {
		kept = kept[:d.MaxItems]
	}
	return kept, len(picked) - len(kept)
}
