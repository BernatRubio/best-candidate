package tailor

import (
	"fmt"
	"strings"
)

// claimKind is the place a claim has in the documents.
type claimKind int

const (
	kindSummary claimKind = iota
	kindSkill
	kindLanguage
	kindProject // header: name and link
	kindProjectDetail
	kindExperience // header: title, company, location, period
	kindExperienceBullet
	kindEducation // header: degree, institution, period, ranking
	kindEducationDetail
	kindSectionItem
	kindLetterParagraph
)

// ClaimRef locates one claim. Entry is the index of the project, job, degree
// or section the claim belongs to, and Line the index of the line inside it.
// Flat lists (skills, languages, letter paragraphs) have one entry, 0, and
// headers are line 0 of their entry.
type ClaimRef struct {
	Kind        claimKind
	Entry, Line int
}

// String is the id the model and the report see, such as
// "cv.experience[0].bullets[1]".
func (r ClaimRef) String() string {
	switch r.Kind {
	case kindSummary:
		return "cv.summary"
	case kindSkill:
		return fmt.Sprintf("cv.skills[%d]", r.Line)
	case kindLanguage:
		return fmt.Sprintf("cv.languages[%d]", r.Line)
	case kindProject:
		return fmt.Sprintf("cv.projects[%d]", r.Entry)
	case kindProjectDetail:
		return fmt.Sprintf("cv.projects[%d].details[%d]", r.Entry, r.Line)
	case kindExperience:
		return fmt.Sprintf("cv.experience[%d]", r.Entry)
	case kindExperienceBullet:
		return fmt.Sprintf("cv.experience[%d].bullets[%d]", r.Entry, r.Line)
	case kindEducation:
		return fmt.Sprintf("cv.education[%d]", r.Entry)
	case kindEducationDetail:
		return fmt.Sprintf("cv.education[%d].details[%d]", r.Entry, r.Line)
	case kindSectionItem:
		return fmt.Sprintf("cv.sections[%d].items[%d]", r.Entry, r.Line)
	case kindLetterParagraph:
		return fmt.Sprintf("letter.paragraphs[%d]", r.Line)
	}
	return fmt.Sprintf("unknown[%d]", r.Kind)
}

// isBullet reports whether the claim is an experience or project bullet.
func (r ClaimRef) isBullet() bool {
	return r.Kind == kindExperienceBullet || r.Kind == kindProjectDetail
}

// flatClaim is one checkable statement of the documents.
type flatClaim struct {
	Ref        ClaimRef
	ID         string // Ref.String()
	Text       string // as the fact-checker sees it, with its context
	Plain      string // the words the CV prints, as an ATS reads them
	Sources    []string
	NeedSource bool
}

func join(parts ...string) string {
	var kept []string
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, " | ")
}

// claims lists every checkable statement in the documents, in document order.
// Refs are positional: they are valid until the documents are next changed.
func (d *Documents) claims() []flatClaim {
	var out []flatClaim
	add := func(ref ClaimRef, text, plain string, src []string, need bool) {
		out = append(out, flatClaim{Ref: ref, ID: ref.String(), Text: text, Plain: plain, Sources: src, NeedSource: need})
	}
	cv := &d.CV
	if strings.TrimSpace(cv.Summary.Text) != "" {
		add(ClaimRef{Kind: kindSummary}, cv.Summary.Text, cv.Summary.Text, cv.Summary.Sources, true)
	}
	for i, c := range cv.Skills {
		add(ClaimRef{kindSkill, 0, i}, "Skills: "+c.Text, c.Text, c.Sources, true)
	}
	for i, c := range cv.Languages {
		add(ClaimRef{kindLanguage, 0, i}, "Languages: "+c.Text, "", c.Sources, true)
	}
	for i, p := range cv.Projects {
		add(ClaimRef{kindProject, i, 0}, join(p.Name, p.Link), p.Name, p.Sources, true)
		for j, b := range p.Details {
			add(ClaimRef{kindProjectDetail, i, j}, b.Text, b.Text, b.Sources, true)
		}
	}
	for i, e := range cv.Experience {
		add(ClaimRef{kindExperience, i, 0}, join(e.Title, e.Company, e.Location, e.Period), e.Title, e.Sources, true)
		for j, b := range e.Bullets {
			add(ClaimRef{kindExperienceBullet, i, j}, b.Text, b.Text, b.Sources, true)
		}
	}
	for i, e := range cv.Education {
		add(ClaimRef{kindEducation, i, 0}, join(e.Degree, e.Institution, e.Period, e.Ranking), e.Degree, e.Sources, true)
		for j, b := range e.Details {
			add(ClaimRef{kindEducationDetail, i, j}, b.Text, b.Text, b.Sources, true)
		}
	}
	for i, s := range cv.Sections {
		for j, b := range s.Items {
			add(ClaimRef{kindSectionItem, i, j}, s.Title+": "+b.Text, b.Text, b.Sources, true)
		}
	}
	for i, p := range d.Letter.Paragraphs {
		// Letter paragraphs may restate the offer, so a source is not mandatory;
		// the verifier decides whether every statement about the candidate holds.
		add(ClaimRef{kindLetterParagraph, 0, i}, p.Text, "", p.Sources, false)
	}
	return out
}

// cvClaims returns the CV's claims for which keep is true.
func (d *Documents) cvClaims(keep func(ClaimRef) bool) []flatClaim {
	var out []flatClaim
	for _, c := range d.claims() {
		if c.Ref.Kind != kindLetterParagraph && keep(c.Ref) {
			out = append(out, c)
		}
	}
	return out
}

// keepLines removes the lines of entry whose refs are in bad.
func keepLines[T any](items []T, kind claimKind, entry int, bad map[ClaimRef]bool) []T {
	var out []T
	for j, it := range items {
		if !bad[ClaimRef{kind, entry, j}] {
			out = append(out, it)
		}
	}
	return out
}

// prune removes every claim in bad. A removed project, experience or
// education header removes the whole entry; a section left without items is
// removed too.
func (d *Documents) prune(bad map[ClaimRef]bool) {
	cv := &d.CV
	if bad[ClaimRef{Kind: kindSummary}] {
		cv.Summary = Claim{}
	}
	cv.Skills = keepLines(cv.Skills, kindSkill, 0, bad)
	cv.Languages = keepLines(cv.Languages, kindLanguage, 0, bad)

	var projs []Project
	for i, p := range cv.Projects {
		if !bad[ClaimRef{kindProject, i, 0}] {
			p.Details = keepLines(p.Details, kindProjectDetail, i, bad)
			projs = append(projs, p)
		}
	}
	cv.Projects = projs

	var exps []Experience
	for i, e := range cv.Experience {
		if !bad[ClaimRef{kindExperience, i, 0}] {
			e.Bullets = keepLines(e.Bullets, kindExperienceBullet, i, bad)
			exps = append(exps, e)
		}
	}
	cv.Experience = exps

	var edus []Education
	for i, e := range cv.Education {
		if !bad[ClaimRef{kindEducation, i, 0}] {
			e.Details = keepLines(e.Details, kindEducationDetail, i, bad)
			edus = append(edus, e)
		}
	}
	cv.Education = edus

	var secs []Section
	for i, s := range cv.Sections {
		if s.Items = keepLines(s.Items, kindSectionItem, i, bad); len(s.Items) > 0 {
			secs = append(secs, s)
		}
	}
	cv.Sections = secs

	d.Letter.Paragraphs = keepLines(d.Letter.Paragraphs, kindLetterParagraph, 0, bad)
}
