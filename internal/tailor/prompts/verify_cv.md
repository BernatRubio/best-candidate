# CV

The claims in <claims> come from the CV.

# Requirements and keywords

- <requirements>, if present: one `coverage` entry per line, `requirement` set to the line's text without its "must have:" or "nice to have:" prefix. `covered` is true only when the profile clearly shows that requirement; then `evidence` is one verbatim profile quote. Otherwise `covered` is false and `evidence` is "". A related skill is not the requirement.
- <keywords>, if present: one `keywords` entry per line, `term` set to the line's text before any "(also written: ...)". `supported` is true only when the profile clearly shows that technology, method or skill itself, under that name or an obvious other spelling; then `evidence` is one verbatim profile quote. Otherwise `supported` is false and `evidence` is "".

# Output

One verdict for every claim id, in the given order, with the id exactly as given and a one-sentence reason.
