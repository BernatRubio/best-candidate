package tailor

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// A course may appear under only one degree. The profile lists shared courses
// under both degrees, so the model has to choose where each one goes; the code
// below detects courses named in more than one education entry and, as a last
// resort, removes the repeats.

var (
	parenthetical = regexp.MustCompile(`\([^)]*\)`)
	gradeLike     = regexp.MustCompile(`\d\.\d`)
)

// splitTopLevel splits on commas that are not inside parentheses, so
// "Parallel Programming (OpenMP, CUDA), Databases" gives two items.
func splitTopLevel(s string) []string {
	var out []string
	depth, start := 0, 0
	for i, r := range s {
		switch r {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				out = append(out, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	return append(out, strings.TrimSpace(s[start:]))
}

// courseKey reduces a course name to comparable tokens: parentheticals, a
// leading "and" and punctuation are dropped.
func courseKey(s string) string {
	s = parenthetical.ReplaceAllString(s, " ")
	k := normalize(s)
	return strings.TrimSpace(strings.TrimPrefix(k, "and "))
}

// educationSection returns the profile text under the "Education" heading.
func educationSection(profile string) string {
	var out []string
	in := false
	for _, line := range strings.Split(profile, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			if in {
				break
			}
			in = strings.Contains(strings.ToLower(trimmed), "education")
			continue
		}
		if in {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

// courseCatalog collects course names from the profile's Education section:
// the items of "Area: course, course, course" lines. It returns normalised
// names, longest first (so that "Advanced Computer Graphics" is matched before
// "Computer Graphics"). An empty catalog disables the profile-based matching.
func courseCatalog(profile string) []string {
	seen := map[string]bool{}
	var out []string
	for _, line := range strings.Split(educationSection(profile), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- ") {
			continue
		}
		rest := strings.TrimPrefix(line, "- ")
		i := strings.Index(rest, ": ")
		if i < 0 {
			continue
		}
		items := splitTopLevel(rest[i+2:])
		if len(items) < 3 {
			continue
		}
		for _, it := range items {
			k := courseKey(it)
			if len(k) < 4 || gradeLike.MatchString(it) || seen[k] {
				continue
			}
			seen[k] = true
			out = append(out, k)
		}
	}
	sort.Slice(out, func(a, b int) bool {
		if len(out[a]) != len(out[b]) {
			return len(out[a]) > len(out[b])
		}
		return out[a] < out[b]
	})
	return out
}

// courseMentions lists the catalog courses named in text. Matched spans are
// masked, longest names first, so "Computer Graphics" is not reported for a
// text that only says "Advanced Computer Graphics".
func courseMentions(text string, catalog []string) []string {
	t := " " + normalize(parenthetical.ReplaceAllString(text, " ")) + " "
	var found []string
	for _, c := range catalog {
		needle := " " + c + " "
		for strings.Contains(t, needle) {
			t = strings.Replace(t, needle, " ", 1)
			found = append(found, c)
		}
	}
	return found
}

// entryCourses maps each course named in an education entry to the indexes of
// the detail lines that name it. Besides catalog matches, the items of
// "Relevant coursework: a, b, c" lines count, so courses the profile does not
// list in the usual form are still compared.
func entryCourses(e Education, catalog []string) map[string][]int {
	m := map[string][]int{}
	add := func(course string, line int) {
		for _, j := range m[course] {
			if j == line {
				return
			}
		}
		m[course] = append(m[course], line)
	}
	for j, d := range e.Details {
		for _, c := range courseMentions(d.Text, catalog) {
			add(c, j)
		}
		if lead, list, ok := courseList(d.Text); ok && isCourseLead(lead) {
			for _, it := range list {
				if k := courseKey(it); k != "" {
					add(k, j)
				}
			}
		}
	}
	return m
}

// courseList splits "Lead: a, b, and c." into its lead and items.
func courseList(text string) (lead string, items []string, ok bool) {
	i := strings.Index(text, ":")
	if i < 0 {
		return "", nil, false
	}
	items = splitTopLevel(strings.TrimRight(strings.TrimSpace(text[i+1:]), ". "))
	if len(items) < 2 {
		return "", nil, false
	}
	return text[:i], items, true
}

func isCourseLead(lead string) bool {
	l := strings.ToLower(lead)
	return strings.Contains(l, "coursework") || strings.Contains(l, "courses")
}

// repeatedCourses finds courses named in more than one education entry. For
// each, the first entry keeps it and the later ones are reported: the result
// maps a course to the entry indexes (after the first) that repeat it.
func repeatedCourses(docs *Documents, catalog []string) (map[string][]int, []map[string][]int) {
	per := make([]map[string][]int, len(docs.CV.Education))
	first := map[string]int{}
	repeats := map[string][]int{}
	for i, e := range docs.CV.Education {
		per[i] = entryCourses(e, catalog)
		for c := range per[i] {
			if f, ok := first[c]; !ok {
				first[c] = i
			} else if f != i {
				repeats[c] = append(repeats[c], i)
			}
		}
	}
	return repeats, per
}

// duplicateCourseIssues returns id -> reason for every detail line that names a
// course already named under an earlier degree.
func duplicateCourseIssues(docs *Documents, catalog []string) map[string]string {
	repeats, per := repeatedCourses(docs, catalog)
	if len(repeats) == 0 {
		return nil
	}
	first := map[string]int{}
	for i := range per {
		for c := range per[i] {
			if _, ok := first[c]; !ok {
				first[c] = i
			}
		}
	}
	byID := map[string][]string{}
	for c, entries := range repeats {
		for _, i := range entries {
			for _, j := range per[i][c] {
				id := fmt.Sprintf("cv.education[%d].details[%d]", i, j)
				byID[id] = append(byID[id], fmt.Sprintf("%q is also named under %q", c, docs.CV.Education[first[c]].Degree))
			}
		}
	}
	issues := map[string]string{}
	for id, parts := range byID {
		sort.Strings(parts)
		issues[id] = "a course may appear under only one degree: " + strings.Join(parts, "; ") +
			". Keep each course under the single degree that best fits this job and remove it from the other entry, replacing it with courses that belong to that degree"
	}
	return issues
}

// dedupeCoursework is the safety net when the model keeps repeating a course:
// each repeated course stays under the first degree that names it and is removed
// from later ones (from a coursework list, or the whole line if it is not a
// list). It returns a warning per removal.
func dedupeCoursework(docs *Documents, catalog []string) []string {
	repeats, per := repeatedCourses(docs, catalog)
	if len(repeats) == 0 {
		return nil
	}
	var warnings []string
	courses := make([]string, 0, len(repeats))
	for c := range repeats {
		courses = append(courses, c)
	}
	sort.Strings(courses)

	drop := map[[2]int]bool{} // (entry, line) removed entirely
	remove := map[[2]int]map[string]bool{}
	for _, c := range courses {
		for _, i := range repeats[c] {
			for _, j := range per[i][c] {
				key := [2]int{i, j}
				if remove[key] == nil {
					remove[key] = map[string]bool{}
				}
				remove[key][c] = true
			}
		}
		warnings = append(warnings, fmt.Sprintf("course %q was named under more than one degree; kept only under the first", c))
	}

	for key, set := range remove {
		e := &docs.CV.Education[key[0]]
		text := e.Details[key[1]].Text
		lead, items, ok := courseList(text)
		if !ok {
			drop[key] = true
			continue
		}
		var kept []string
		for _, it := range items {
			if !set[courseKey(it)] && len(courseMentions(it, keysOf(set))) == 0 {
				kept = append(kept, strings.TrimSpace(it))
			}
		}
		if len(kept) == 0 {
			drop[key] = true
			continue
		}
		e.Details[key[1]].Text = lead + ": " + strings.Join(kept, ", ") + "."
	}

	for i := range docs.CV.Education {
		var details []Claim
		for j, d := range docs.CV.Education[i].Details {
			if !drop[[2]int{i, j}] {
				details = append(details, d)
			}
		}
		docs.CV.Education[i].Details = details
	}
	return warnings
}

func keysOf(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(a, b int) bool { return len(out[a]) > len(out[b]) })
	return out
}
