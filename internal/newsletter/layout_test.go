package newsletter

import (
	"context"
	"strings"
	"testing"
)

// Settings saved before the layout existed keep an edited HTML template and
// move to the designed layout when they still use a stock one.
func TestNormalizeMigratesPreLayoutSettings(t *testing.T) {
	for _, tc := range []struct {
		name, html string
		custom     bool
	}{
		{"stock english", defaultHTMLTemplate, false},
		{"stock polish", plHTMLTemplate, false},
		{"edited", "<p>{{summary}}</p>", true},
	} {
		s := DefaultSettings("en")
		s.Layout = Layout{}
		s.Templates.HTML = tc.html
		s.Normalize()
		if s.Layout.CustomHTML != tc.custom || len(s.Layout.Sections) != len(sectionIDs) || !s.Layout.ShowRatings {
			t.Errorf("%s: layout = %+v", tc.name, s.Layout)
		}
	}
}

func TestDesignedLayoutFollowsSectionOrderAndStyle(t *testing.T) {
	s := DefaultSettings("en")
	s.Layout.Sections = []Section{{ID: SectionShows, Enabled: true}, {ID: SectionMovies, Enabled: true, Style: StyleGrid, Columns: 4}}
	s.Normalize()
	r, err := Preview(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	shows, movies := strings.Index(r.HTML, ">New episodes and shows<"), strings.Index(r.HTML, ">New movies<")
	if shows < 0 || movies < 0 || shows > movies {
		t.Fatalf("sections out of order: shows at %d, movies at %d", shows, movies)
	}
	if !strings.Contains(r.HTML, `width="25%"`) {
		t.Error("movies are not a four-column grid")
	}
}
