package newsletter

import (
	"fmt"
	"strings"
	"time"
)

// Languages the plugin writes its own words in: summaries, labels and dates
// the templates receive as variables. The templates themselves are free text.
const (
	langEN = "en"
	langPL = "pl"
)

func cleanLanguage(l string) string {
	if l == langPL {
		return langPL
	}
	return langEN
}

type lang string

// pluralPL picks the Polish form for n: one (1), few (2-4 except 12-14), many.
func pluralPL(n int, one, few, many string) string {
	if n == 1 {
		return one
	}
	last, lastTwo := n%10, n%100
	if last >= 2 && last <= 4 && (lastTwo < 12 || lastTwo > 14) {
		return few
	}
	return many
}

// count renders "3 new episodes" in the language. forms are the English
// singular and plural followed by the Polish one, few and many forms.
func (l lang) count(n int, en1, enN, pl1, plFew, plMany string) string {
	return fmt.Sprintf("%d %s", n, l.word(n, en1, enN, pl1, plFew, plMany))
}

// word is the noun form count uses, without the number.
func (l lang) word(n int, en1, enN, pl1, plFew, plMany string) string {
	if l == langPL {
		return pluralPL(n, pl1, plFew, plMany)
	}
	if n == 1 {
		return en1
	}
	return enN
}

func (l lang) episodesLabel(n int) string {
	return l.count(n, "new episode", "new episodes", "nowy odcinek", "nowe odcinki", "nowych odcinków")
}

func (l lang) summary(movies, episodes, shows int) string {
	var parts []string
	if episodes > 0 {
		label := l.episodesLabel(episodes)
		if shows > 1 {
			if l == langPL {
				label += fmt.Sprintf(" w %d serialach", shows)
			} else {
				label += fmt.Sprintf(" across %d shows", shows)
			}
		}
		parts = append(parts, label)
	}
	if movies > 0 {
		parts = append(parts, l.count(movies, "new movie", "new movies", "nowy film", "nowe filmy", "nowych filmów"))
	}
	if len(parts) == 0 {
		if l == langPL {
			return "Brak nowości"
		}
		return "Nothing new"
	}
	if l == langPL {
		return strings.Join(parts, " i ")
	}
	return strings.Join(parts, " and ")
}

func (l lang) period(mode string) string {
	if l == langPL {
		return map[string]string{"instant": "właśnie dodane", "daily": "dzisiaj", "weekly": "w tym tygodniu", "test": "ostatnio", "preview": "ostatnio"}[mode]
	}
	return map[string]string{"instant": "just added", "daily": "today", "weekly": "this week", "test": "recently", "preview": "recently"}[mode]
}

var (
	plWeekdays      = [...]string{"niedziela", "poniedziałek", "wtorek", "środa", "czwartek", "piątek", "sobota"}
	plWeekdaysShort = [...]string{"nd.", "pon.", "wt.", "śr.", "czw.", "pt.", "sob."}
	// Month names in the genitive, as in "5 listopada".
	plMonths      = [...]string{"stycznia", "lutego", "marca", "kwietnia", "maja", "czerwca", "lipca", "sierpnia", "września", "października", "listopada", "grudnia"}
	plMonthsShort = [...]string{"sty", "lut", "mar", "kwi", "maj", "cze", "lip", "sie", "wrz", "paź", "lis", "gru"}
)

// date is the long date for the email header.
func (l lang) date(t time.Time) string {
	if l == langPL {
		return fmt.Sprintf("%s, %d %s %d", plWeekdays[t.Weekday()], t.Day(), plMonths[t.Month()-1], t.Year())
	}
	return t.Format("Monday 2 January 2006")
}

func (l lang) shortDate(t time.Time) string {
	if l == langPL {
		return fmt.Sprintf("%d %s", t.Day(), plMonthsShort[t.Month()-1])
	}
	return t.Format("2 Jan")
}

func (l lang) dayDate(t time.Time) string {
	if l == langPL {
		return fmt.Sprintf("%d %s %d", t.Day(), plMonths[t.Month()-1], t.Year())
	}
	return t.Format("2 January 2006")
}

// addedDate is when an item arrived, such as "Mon 2 Jan, 15:04".
func (l lang) addedDate(t time.Time) string {
	if l == langPL {
		return fmt.Sprintf("%s %d %s, %s", plWeekdaysShort[t.Weekday()], t.Day(), plMonthsShort[t.Month()-1], t.Format("15:04"))
	}
	return t.Format("Mon 2 Jan, 15:04")
}

// humanDate renders a YYYY-MM-DD date, or returns it unchanged.
func (l lang) humanDate(s string) string {
	if s == "" {
		return ""
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return s
	}
	return l.dayDate(t)
}

// dateRange renders [from, to] as "30 October – 5 November 2026".
func (l lang) dateRange(from, to time.Time) string {
	month := func(t time.Time) string {
		if l == langPL {
			return plMonths[t.Month()-1]
		}
		return t.Month().String()
	}
	switch {
	case from.Year() != to.Year():
		return fmt.Sprintf("%d %s %d – %d %s %d", from.Day(), month(from), from.Year(), to.Day(), month(to), to.Year())
	case from.Month() != to.Month():
		return fmt.Sprintf("%d %s – %d %s %d", from.Day(), month(from), to.Day(), month(to), to.Year())
	case from.Day() != to.Day():
		return fmt.Sprintf("%d – %d %s %d", from.Day(), to.Day(), month(to), to.Year())
	}
	return l.dayDate(to)
}

// minutes renders a length such as "2h 10m" or "2 godz. 10 min".
func (l lang) minutes(m int) string {
	if m <= 0 {
		return ""
	}
	h, rest := m/60, m%60
	if l == langPL {
		switch {
		case h == 0:
			return fmt.Sprintf("%d min", rest)
		case rest == 0:
			return fmt.Sprintf("%d godz.", h)
		}
		return fmt.Sprintf("%d godz. %d min", h, rest)
	}
	switch {
	case h == 0:
		return fmt.Sprintf("%dm", rest)
	case rest == 0:
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh %dm", h, rest)
}

func (l lang) seasonLabel(season int) string {
	if l == langPL {
		if season == 0 {
			return "Odcinki specjalne"
		}
		return fmt.Sprintf("Sezon %d", season)
	}
	if season == 0 {
		return "Specials"
	}
	return fmt.Sprintf("Season %d", season)
}

func (l lang) newOn(server string) string {
	if l == langPL {
		return "Nowości na " + server
	}
	return "New on " + server
}
