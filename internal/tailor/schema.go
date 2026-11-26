// Package tailor turns a profile and a job offer into verified CV and cover
// letter documents.
package tailor

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

// Documents is the tailored CV and cover letter.
type Documents struct {
	CV     CV     `json:"cv"`
	Letter Letter `json:"letter"`
}

// output is what the model returns: the documents plus what the program needs
// to name the output folder and pick the language.
type output struct {
	Company  string `json:"company" jsonschema:"description=Hiring company name; empty string if the offer does not say"`
	Role     string `json:"role" jsonschema:"description=Job title being hired for"`
	Language string `json:"language" jsonschema:"description=ISO 639-1 code of the language the documents are written in, e.g. en or es or ca"`
	CV       CV     `json:"cv"`
	Letter   Letter `json:"letter"`
}
