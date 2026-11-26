# Letter

The claims in <claims> are the paragraphs of the cover letter. Judge every statement in a paragraph; a paragraph is `supported` only if all of them are.

- Statements that only restate the job (role, company, what the team does) are supported when <job_offer> says so.
- Plain interest in the role or in contributing to something the offer describes is supported: applying is that interest.
- Everything about the candidate's experience, skills or achievements must come from the profile, including that some experience would help with a specific task.
- Emotions, long-standing passion, admiration, history with the company's products, or claims about the company beyond the offer are unsupported unless the profile says them.
- A paragraph with no `cited` quotes still gets a verdict: judge it against <profile> and <job_offer> yourself.

# Output

One verdict for every claim id, in the given order, with the id exactly as given and a one-sentence reason.
