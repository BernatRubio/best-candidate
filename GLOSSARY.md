# Glossary

- **Profile**: the user's master markdown file, parsed once by `profile.Parse`. Everything the tool may say about the candidate must be quoted from it.
- **Offer**: the job offer text. Untrusted input.
- **Job analysis**: the model's structured reading of the offer (requirements, keywords, language, tone).
- **Draft**: the CV and cover letter the writer returns in one tailoring call, before checks.
- **Claim**: one checkable statement in a draft, with the profile quotes (sources) that support it.
- **Claim ref**: the position of a claim in the documents (`ClaimRef`). It renders as the id the model and the report see, such as `cv.experience[0].bullets[1]`, and is valid until the documents next change.
- **Fact-checker**: the independent model call that gives each claim a verdict (supported, exaggerated or unsupported) and assesses the offer's requirements and keywords.
- **Draft review**: the module (`draftReview`) that does everything the program checks and fixes itself around the model calls, and decides their order: Prepare, Review, GiveUp, Finalize.
- **Findings**: what one review round found: issues (dropped if they persist), repeated courses (deduplicated if they persist), keyword gaps (only trigger a repair) and unreviewed claims (the fact-checker gave no verdict; verified again without a repair, dropped if they persist).
- **Repair round**: a tailoring call that gets the previous draft plus the findings to fix.
- **Coverage**: whether the profile shows each requirement of the offer, with a profile quote as evidence.
- **ATS**: how the CV matches the offer's keywords, as an applicant tracking system would read it.
- **Fit**: cutting the least relevant content until the CV is one page.
