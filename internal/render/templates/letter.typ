// Cover letter template. Reads letter.json next to this file; edit either and
// run `best-candidate render <dir>` to rebuild the PDF. The font size and
// spacing adapt to the length of the letter so that the page is filled.
#import "common.typ": setup-links, contact-item, fit-page

#let data = json("letter.json")
#let l = data.letter

#let margin = (x: 2.5cm, y: 2.3cm)
#let sizes = (12pt, 11.5pt, 11pt, 10.5pt)
#let gaps = (1.8, 1.5, 1.25, 1.0)

#set document(title: data.name)
#set page(paper: "a4", margin: margin)
#set text(font: "Libertinus Serif", hyphenate: false, lang: if data.lang in ("en", "es", "ca") { data.lang } else { "en" })
#show: setup-links

#let build(size, gap) = {
  set text(size: size)
  set par(justify: true, leading: 0.8em, spacing: 1.2em * gap)
  [
    #align(right)[
      #text(weight: "bold", size: 1.1em)[#data.name]
      #for line in data.contact [
        #linebreak()#contact-item(line)
      ]
      #linebreak()#data.date
    ]

    #v(1.2em * gap)

    #if l.recipient != "" and not l.recipient.contains("_") [
      #l.recipient
      #v(0.6em * gap)
    ]

    #l.greeting

    #for p in l.paragraphs {
      p.text
      parbreak()
    }

    #l.closing

    #v(1.8em * gap)
    #l.signature
  ]
}

#fit-page(build, sizes, gaps, margin)
