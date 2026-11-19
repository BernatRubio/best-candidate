// CV template. Reads cv.json next to this file; edit either and run
// `best-candidate render <dir>` to rebuild the PDF. The font size and spacing
// adapt to the amount of content so that the page is filled (see fit-page).
#import "common.typ": setup-links, contact-item, fit-page

#let data = json("cv.json")
#let cv = data.cv

#let labels = (
  en: (summary: "Profile", experience: "Experience", education: "Education"),
  es: (summary: "Perfil", experience: "Experiencia", education: "Formación"),
  ca: (summary: "Perfil", experience: "Experiència", education: "Formació"),
)
#let lang = if data.lang in labels { data.lang } else { "en" }
#let t = labels.at(lang)

#let margin = (x: 1.9cm, y: 1.6cm)
// Candidate font sizes (largest first) and spacing factors (roomiest first).
#let sizes = (11.5pt, 11pt, 10.5pt, 10pt, 9.5pt)
#let gaps = (2.0, 1.7, 1.4, 1.2, 1.0)

#set document(title: cv.name)
#set page(paper: "a4", margin: margin)
#set text(font: "Libertinus Serif", lang: lang)
#show: setup-links

#let heading-line(title, gap) = {
  v(0.8em * gap)
  block(sticky: true)[
    #text(weight: "bold", size: 1.05em, tracking: 0.06em)[#upper(title)]
    #v(-0.5em)
    #line(length: 100%, stroke: 0.6pt + luma(110))
  ]
  v(0.1em * gap)
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

#let build(size, gap) = {
  set text(size: size)
  set par(justify: false, leading: 0.55em, spacing: 0.7em * gap)
  set list(indent: 0.4em, body-indent: 0.5em, spacing: 0.45em * gap)
  [
    #align(center)[
      #text(size: 2.1em, weight: "bold")[#cv.name]
      #if cv.contact.len() > 0 [
        #v(0.2em * gap)
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
            #list(..e.bullets.map(b => b.text))
          ]
        ]
        #v(0.4em * gap)
      ]
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
            #list(..e.details.map(b => b.text))
          ]
        ]
        #v(0.4em * gap)
      ]
    ]

    #for s in cv.sections.filter(s => placement-of(s) != "before_experience") [
      #render-section(s, gap)
    ]
  ]
}

#fit-page(build, sizes, gaps, margin)
