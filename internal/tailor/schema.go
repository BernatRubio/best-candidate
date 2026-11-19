// Package tailor turns a profile and a job offer into verified CV and cover
// letter documents.
package tailor

import (
	"fmt"
	"strings"
)

// JobAnalysis is the structured reading of the job offer.
type JobAnalysis struct {
	Company    string   `json:"company" jsonschema:"description=Hiring company name; empty string if the offer does not say"`
	Role       string   `json:"role" jsonschema:"description=Job title being hired for"`
	Language   string   `json:"language" jsonschema:"description=ISO 639-1 code of the language the offer is written in, e.g. en or es or ca"`
	MustHave   []string `json:"must_have" jsonschema:"description=Hard requirements stated in the offer, one short phrase each"`
	NiceToHave []string `json:"nice_to_have" jsonschema:"description=Optional or preferred qualifications stated in the offer"`
	Keywords   []string `json:"keywords" jsonschema:"description=Technologies, methods and domain terms the offer uses"`
	Tone       string   `json:"tone" jsonschema:"description=Register of the company: for example formal or startup-casual or technical"`
}

// Claim is a piece of text about the candidate plus the evidence for it.
type Claim struct {
	Text    string   `json:"text" jsonschema:"description=The statement as it appears in the document"`
	Sources []string `json:"sources" jsonschema:"description=Quotes copied verbatim from the profile that support the text"`
}

// Experience is one job entry in the CV.
type Experience struct {
	Title    string   `json:"title"`
	Company  string   `json:"company"`
	Location string   `json:"location" jsonschema:"description=Empty string if the profile gives none"`
	Period   string   `json:"period" jsonschema:"description=Dates exactly as the profile states them"`
	Sources  []string `json:"sources" jsonschema:"description=Verbatim profile quotes that support title and company and period"`
	Bullets  []Claim  `json:"bullets"`
}

// Education is one study entry in the CV.
type Education struct {
	Relevance   int      `json:"relevance" jsonschema:"description=How directly this degree matches the field the job needs from 0 (unrelated) to 10 (exactly that field); the program lists the degrees from the highest score to the lowest"`
	Degree      string   `json:"degree"`
	Institution string   `json:"institution"`
	Period      string   `json:"period" jsonschema:"description=Dates exactly as the profile states them; empty string if none"`
	Ranking     string   `json:"ranking" jsonschema:"description=Class ranking of this degree exactly as the profile gives it (for example Top 25%) or an empty string if the profile gives none"`
	Sources     []string `json:"sources" jsonschema:"description=Verbatim profile quotes that support degree and institution and period"`
	Details     []Claim  `json:"details"`
}

// Project is one entry of the CV's Projects block. The block title comes from
// the template, so Name must be a short name and never a sentence.
type Project struct {
	Name    string   `json:"name" jsonschema:"description=Short project name only (for example Orion); never a sentence or a long title"`
	Link    string   `json:"link" jsonschema:"description=Repository or page URL exactly as the profile gives it or an empty string"`
	Sources []string `json:"sources" jsonschema:"description=Verbatim profile quotes that support the project name"`
	Details []Claim  `json:"details" jsonschema:"description=What was built and with what and what was tested; one bullet each"`
}

// Section layouts and placements.
const (
	LayoutInline  = "inline"  // short items on one line separated by dots (languages)
	LayoutLines   = "lines"   // one line per item, the text before the first colon in bold (skills)
	LayoutBullets = "bullets" // a bulleted list (projects, certifications)

	PlaceBeforeExperience = "before_experience"
	PlaceAfterEducation   = "after_education"
)

// Section is an extra CV block such as certifications. Skills, languages and
// projects have their own fields on CV and must not be repeated here.
type Section struct {
	Title     string  `json:"title"`
	Layout    string  `json:"layout" jsonschema:"enum=inline,enum=lines,enum=bullets,description=bullets for a list of short lines (the normal choice for extra blocks such as certifications); lines for one labelled line per item; inline for short items on one line"`
	Placement string  `json:"placement" jsonschema:"enum=before_experience,enum=after_education,description=after_education for extra blocks; before_experience only for hand-edited files"`
	Items     []Claim `json:"items"`
}

// CV is the tailored curriculum vitae. The titles of the Projects, Skills and
// Languages blocks are fixed by the template (and localised), so the model only
// fills their content.
type CV struct {
	Name          string       `json:"name" jsonschema:"description=Candidate full name exactly as in the profile"`
	Contact       []string     `json:"contact" jsonschema:"description=Contact lines (email and phone and location and links) copied exactly from the profile"`
	Summary       Claim        `json:"summary" jsonschema:"description=Two to three sentence professional summary; empty text if the profile does not support one"`
	Skills        []Claim      `json:"skills" jsonschema:"description=Required whenever the profile lists skills. One item per category written as the category then a colon then the skills separated by commas with the skills most relevant to the job first"`
	Languages     []Claim      `json:"languages" jsonschema:"description=Required whenever the profile lists spoken languages. One short item per language written as Language (level) and nothing else"`
	Projects      []Project    `json:"projects" jsonschema:"description=Projects from the profile that are relevant to this job; empty if none is relevant"`
	ProjectsFirst bool         `json:"projects_first" jsonschema:"description=true to show projects before the work experience because they are stronger evidence for this job than the work history"`
	Experience    []Experience `json:"experience"`
	Education     []Education  `json:"education"`
	Sections      []Section    `json:"sections" jsonschema:"description=Only extra blocks such as certifications; never skills or languages or projects"`
}

// Letter is the tailored cover letter.
type Letter struct {
	Recipient  string  `json:"recipient" jsonschema:"description=Addressee line in plain words such as the company name followed by Hiring Team; never an identifier with underscores; empty string if unknown"`
	Greeting   string  `json:"greeting"`
	Paragraphs []Claim `json:"paragraphs"`
	Closing    string  `json:"closing" jsonschema:"description=Sign-off phrase such as Kind regards"`
	Signature  string  `json:"signature" jsonschema:"description=Candidate name"`
}

// Coverage records how the profile matches one requirement of the offer.
type Coverage struct {
	Requirement string `json:"requirement"`
	Covered     bool   `json:"covered" jsonschema:"description=true only if the profile clearly shows it"`
	Evidence    string `json:"evidence" jsonschema:"description=Verbatim profile quote when covered; empty string otherwise"`
}

// Documents is everything the tailoring step produces.
type Documents struct {
	CV       CV         `json:"cv"`
	Letter   Letter     `json:"letter"`
	Coverage []Coverage `json:"coverage"`
}

// Verdict is the fact-checker's judgement on one claim.
type Verdict struct {
	ID     string `json:"id"`
	Status string `json:"status" jsonschema:"enum=supported,enum=exaggerated,enum=unsupported"`
	Reason string `json:"reason" jsonschema:"description=One sentence explaining the verdict"`
}

// VerificationReport is the fact-checker's output.
type VerificationReport struct {
	Verdicts []Verdict  `json:"verdicts"`
	Coverage []Coverage `json:"coverage" jsonschema:"description=One entry per requirement listed in the requirements block of the input"`
}

// flatClaim is a claim with a stable id, used for verification and pruning.
type flatClaim struct {
	ID         string
	Text       string
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

// claims lists every checkable statement in the documents. Ids are positional
// and match what prune expects.
func (d *Documents) claims() []flatClaim {
	var out []flatClaim
	add := func(id, text string, src []string, need bool) {
		out = append(out, flatClaim{ID: id, Text: text, Sources: src, NeedSource: need})
	}
	if strings.TrimSpace(d.CV.Summary.Text) != "" {
		add("cv.summary", d.CV.Summary.Text, d.CV.Summary.Sources, true)
	}
	for i, c := range d.CV.Skills {
		add(fmt.Sprintf("cv.skills[%d]", i), "Skills: "+c.Text, c.Sources, true)
	}
	for i, c := range d.CV.Languages {
		add(fmt.Sprintf("cv.languages[%d]", i), "Languages: "+c.Text, c.Sources, true)
	}
	for i, p := range d.CV.Projects {
		add(fmt.Sprintf("cv.projects[%d]", i), join(p.Name, p.Link), p.Sources, true)
		for j, b := range p.Details {
			add(fmt.Sprintf("cv.projects[%d].details[%d]", i, j), b.Text, b.Sources, true)
		}
	}
	for i, e := range d.CV.Experience {
		add(fmt.Sprintf("cv.experience[%d]", i), join(e.Title, e.Company, e.Location, e.Period), e.Sources, true)
		for j, b := range e.Bullets {
			add(fmt.Sprintf("cv.experience[%d].bullets[%d]", i, j), b.Text, b.Sources, true)
		}
	}
	for i, e := range d.CV.Education {
		add(fmt.Sprintf("cv.education[%d]", i), join(e.Degree, e.Institution, e.Period, e.Ranking), e.Sources, true)
		for j, b := range e.Details {
			add(fmt.Sprintf("cv.education[%d].details[%d]", i, j), b.Text, b.Sources, true)
		}
	}
	for i, s := range d.CV.Sections {
		for j, b := range s.Items {
			add(fmt.Sprintf("cv.sections[%d].items[%d]", i, j), s.Title+": "+b.Text, b.Sources, true)
		}
	}
	for i, p := range d.Letter.Paragraphs {
		// Letter paragraphs may restate the offer, so a source is not mandatory;
		// the verifier decides whether every statement about the candidate holds.
		add(fmt.Sprintf("letter.paragraphs[%d]", i), p.Text, p.Sources, false)
	}
	return out
}

func keep(cs []Claim, prefix string, bad map[string]bool) []Claim {
	var out []Claim
	for j, c := range cs {
		if !bad[fmt.Sprintf("%s[%d]", prefix, j)] {
			out = append(out, c)
		}
	}
	return out
}

// prune removes every claim whose id is in bad. A flagged experience or
// education header removes the whole entry.
func (d *Documents) prune(bad map[string]bool) {
	if bad["cv.summary"] {
		d.CV.Summary = Claim{}
	}
	d.CV.Skills = keep(d.CV.Skills, "cv.skills", bad)
	d.CV.Languages = keep(d.CV.Languages, "cv.languages", bad)
	var projs []Project
	for i, p := range d.CV.Projects {
		if bad[fmt.Sprintf("cv.projects[%d]", i)] {
			continue
		}
		p.Details = keep(p.Details, fmt.Sprintf("cv.projects[%d].details", i), bad)
		projs = append(projs, p)
	}
	d.CV.Projects = projs

	var exps []Experience
	for i, e := range d.CV.Experience {
		if bad[fmt.Sprintf("cv.experience[%d]", i)] {
			continue
		}
		e.Bullets = keep(e.Bullets, fmt.Sprintf("cv.experience[%d].bullets", i), bad)
		exps = append(exps, e)
	}
	d.CV.Experience = exps

	var edus []Education
	for i, e := range d.CV.Education {
		if bad[fmt.Sprintf("cv.education[%d]", i)] {
			continue
		}
		e.Details = keep(e.Details, fmt.Sprintf("cv.education[%d].details", i), bad)
		edus = append(edus, e)
	}
	d.CV.Education = edus

	var secs []Section
	for i, s := range d.CV.Sections {
		s.Items = keep(s.Items, fmt.Sprintf("cv.sections[%d].items", i), bad)
		if len(s.Items) > 0 {
			secs = append(secs, s)
		}
	}
	d.CV.Sections = secs

	d.Letter.Paragraphs = keep(d.Letter.Paragraphs, "letter.paragraphs", bad)
}
