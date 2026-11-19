// Helpers shared by cv.typ and letter.typ: clickable links, contact lines with
// icons, and automatic fitting of the content to the page.

#let link-color = rgb("#1f4e8c")
#let link-hex = "#1f4e8c"
// Brand colours of the two icons.
#let github-hex = "#24292f"
#let linkedin-hex = "#0a66c2"

// Use as `#show: setup-links`. Plain http(s):// and www. addresses written
// anywhere in the text (for example a repository URL inside a bullet) become
// clickable links; all links get a subtle colour and underline.
#let setup-links(body) = {
  show link: set text(fill: link-color)
  show link: underline
  show regex("https?://[^\\s,;)]*[^\\s,;).]"): it => link(it.text)
  show regex("\\bwww\\.[^\\s,;)]*[^\\s,;).]"): it => link("https://" + it.text)
  body
}

// Brand marks as inline SVG (GitHub: Octicons "mark-github", MIT; LinkedIn:
// Simple Icons "linkedin", CC0), each drawn in its brand colour.
#let github-svg = "<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 16 16'><path fill='" + github-hex + "' d='M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.013 8.013 0 0016 8c0-4.42-3.58-8-8-8z'/></svg>"

#let linkedin-svg = "<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24'><path fill='" + linkedin-hex + "' d='M20.447 20.452h-3.554v-5.569c0-1.328-.027-3.037-1.852-3.037-1.853 0-2.136 1.445-2.136 2.939v5.667H9.351V9h3.414v1.561h.046c.477-.9 1.637-1.85 3.37-1.85 3.601 0 4.267 2.37 4.267 5.455v6.286zM5.337 7.433c-1.144 0-2.063-.926-2.063-2.065 0-1.138.92-2.063 2.063-2.063 1.14 0 2.064.925 2.064 2.063 0 1.139-.925 2.065-2.064 2.065zm1.782 13.019H3.555V9h3.564v11.452zM22.225 0H1.771C.792 0 0 .774 0 1.729v20.542C0 23.227.792 24 1.771 24h20.451C23.2 24 24 23.227 24 22.271V1.729C24 .774 23.2 0 22.222 0h.003z'/></svg>"

#let icon(svg) = box(image(bytes(svg), format: "svg", height: 0.82em), baseline: 0.1em)

// One contact entry, kept together on one line: a link (with an icon for GitHub
// and LinkedIn, followed only by the handle) when it is an email or a web
// address, otherwise plain text. Phone numbers stay plain text. Other web
// addresses are shown without the scheme.
#let contact-item(s) = {
  let t = s.trim()
  let shown = t.replace(regex("^https?://"), "").replace(regex("^www\\."), "")
  let is-url = t.starts-with(regex("https?://|www\\.")) or t.contains("linkedin.com") or t.contains("github.com")
  let handle(domain) = {
    let h = shown.replace(regex("^" + domain + "/?"), "")
    if h == "" { shown } else { h }
  }
  if t.contains("@") and not t.contains(" ") {
    box(link("mailto:" + t, t))
  } else if is-url {
    let url = if t.starts-with("http") { t } else { "https://" + t }
    if t.contains("linkedin.com") {
      box(link(url)[#icon(linkedin-svg)#h(0.25em)#handle("linkedin\\.com")])
    } else if t.contains("github.com") {
      box(link(url)[#icon(github-svg)#h(0.25em)#handle("github\\.com")])
    } else {
      box(link(url, shown))
    }
  } else {
    box(t)
  }
}

// Lay out `build(size, gap)` so that it fills the page without overflowing:
// the largest font size from `sizes` (in descending order) that fits on one
// page, then the roomiest spacing factor from `gaps` (descending, last = 1.0)
// that still fits at that size. If even the smallest size overflows, the
// content flows onto a second page at the smallest size.
#let fit-page(build, sizes, gaps, margin) = context {
  let w = page.width - 2 * margin.x
  let h = page.height - 2 * margin.y
  let fits(sz, gap) = measure(width: w, build(sz, gap)).height <= h
  let size = sizes.find(sz => fits(sz, gaps.last()))
  if size == none {
    build(sizes.last(), gaps.last())
  } else {
    build(size, gaps.find(g => fits(size, g)))
  }
}
