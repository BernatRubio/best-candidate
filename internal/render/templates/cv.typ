// CV template. Reads cv.json next to this file; edit either and run
// `best-candidate render <dir>` to rebuild the PDF. The font size and spacing
// adapt to the amount of content so that the page is filled (see fit-page).
#import "common.typ": setup-links, contact-item, fit-page

#let data = json("cv.json")
#let cv = data.cv

#let labels = (
  en: (summary: "Summary", experience: "Experience", education: "Education", projects: "Projects", skills: "Skills", languages: "Languages"),
  es: (summary: "Perfil", experience: "Experiencia", education: "Formación", projects: "Proyectos", skills: "Habilidades", languages: "Idiomas"),
  ca: (summary: "Perfil", experience: "Experiència", education: "Formació", projects: "Projectes", skills: "Habilitats", languages: "Idiomes"),
)
#let lang = if data.lang in labels { data.lang } else { "en" }
#let t = labels.at(lang)

// Older cv.json files lack these fields.
#let projects = cv.at("projects", default: ())
#let skills = cv.at("skills", default: ())
#let languages = cv.at("languages", default: ())
#let projects-first = cv.at("projects_first", default: false)

#let margin = (x: 1.9cm, y: 1.6cm)
// Candidate font sizes (largest first) and spacing factors (roomiest first).
#let sizes = (11pt, 10.5pt, 10pt, 9.5pt)
#let gaps = (1.8, 1.65, 1.5, 1.4, 1.3, 1.2, 1.1, 1.0)

#set document(title: cv.name + " CV", author: cv.name)
#set page(paper: "a4", margin: margin)
// Ligatures off: "fi" and "fl" glyphs extract badly in applicant tracking systems.
#set text(font: "Libertinus Serif", lang: lang, ligatures: false)
#show: setup-links

#let heading-line(title, gap) = {
  v(0.95em * gap)
  block(sticky: true)[
    #text(weight: "bold", size: 1.05em, tracking: 0.06em)[#upper(title)]
    #v(-0.5em)
    #line(length: 100%, stroke: 0.6pt + luma(110))
  ]
  v(0.28em * gap)
}

// "Label: a, b, c" with the label in bold.
#let labelled(txt) = {
  let m = txt.match(regex("^([^:]{1,40}):\\s+(.+)$"))
  if m != none [*#m.captures.at(0):* #m.captures.at(1)] else [#txt]
}

// Older cv.json files have `inline: true` instead of `layout`.
#let layout-of(s) = s.at("layout", default: if s.at("inline", default: false) { "inline" } else { "bullets" })
#let placement-of(s) = s.at("placement", default: "after_education")

#let render-section(s, gap) = [
  #heading-line(s.title, gap)
  #let layout = layout-of(s)
  #if layout == "inline" [
    #s.items.map(i => i.text).join([#h(0.5em)·#h(0.5em)])
  ] else if layout == "lines" [
    #s.items.map(i => labelled(i.text)).join(linebreak())
  ] else [
    #list(..s.items.map(i => i.text))
  ]
]

#let render-projects(gap) = [
  #heading-line(t.projects, gap)
  #for p in projects [
    #block(breakable: false)[
      #grid(
        columns: (1fr, auto),
        [*#p.name*],
        align(right)[#if p.at("link", default: "") != "" [#contact-item(p.link)]],
      )
      #if p.details.len() > 0 [
        #v(0.15em * gap)
        #list(..p.details.map(b => b.text))
      ]
    ]
    #v(0.7em * gap)
  ]
]

#let render-skills(gap) = [
  #heading-line(t.skills, gap)
  #skills.map(i => labelled(i.text)).join(linebreak())
]

#let render-languages(gap) = [
  #heading-line(t.languages, gap)
  #languages.map(i => i.text).join([#h(0.5em)·#h(0.5em)])
]

#let build(size, gap) = {
  set text(size: size)
  set par(justify: false, leading: 0.62em, spacing: 0.7em * gap)
  set list(indent: 0.4em, body-indent: 0.5em, spacing: 0.58em * gap)
  [
    #align(center)[
      #text(size: 2.1em, weight: "bold")[#cv.name]
      #if cv.contact.len() > 0 [
        #v(0.15em * gap)
        #cv.contact.map(contact-item).join([#h(0.45em)·#h(0.45em)])
      ]
    ]

    #if cv.summary.text != "" [
      #heading-line(t.summary, gap)
      #cv.summary.text
    ]

    #for s in cv.sections.filter(s => placement-of(s) == "before_experience") [
      #render-section(s, gap)
    ]

    #if projects.len() > 0 and projects-first [
      #render-projects(gap)
    ]

    #if cv.experience.len() > 0 [
      #heading-line(t.experience, gap)
      #for e in cv.experience [
        #block(breakable: false)[
          #grid(
            columns: (1fr, auto),
            [*#e.title*, #e.company#if e.location != "" [, #e.location]],
            align(right)[#e.period],
          )
          #if e.bullets.len() > 0 [
            #v(0.15em * gap)
            #list(..e.bullets.map(b => b.text))
          ]
        ]
        #v(0.7em * gap)
      ]
    ]

    #if projects.len() > 0 and not projects-first [
      #render-projects(gap)
    ]

    #if cv.education.len() > 0 [
      #heading-line(t.education, gap)
      #for e in cv.education [
        #block(breakable: false)[
          #grid(
            columns: (1fr, auto),
            [*#e.degree*, #e.institution#if e.at("ranking", default: "") != "" [ (#e.ranking)]],
            align(right)[#e.period],
          )
          #if e.details.len() > 0 [
            #v(0.15em * gap)
            #list(..e.details.map(b => b.text))
          ]
        ]
        #v(0.7em * gap)
      ]
    ]

    #if skills.len() > 0 [
      #render-skills(gap)
    ]

    #if languages.len() > 0 [
      #render-languages(gap)
    ]

    #for s in cv.sections.filter(s => placement-of(s) != "before_experience") [
      #render-section(s, gap)
    ]
  ]
}

#fit-page(build, sizes, gaps, margin, tight: (0.9, 0.8))
