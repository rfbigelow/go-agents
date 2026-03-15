# P7: Requirements Process and Reporting

<!-- How requirements are gathered, documented, reviewed, and maintained
     in this project. This is the meta-process — the process for
     managing the requirements themselves.

     This chapter is particularly important for the agentic workflow:
     it tells both humans and agents HOW to work with these files. -->

## Requirements Process

### Elicitation

<!-- How are requirements gathered? Who is consulted? What techniques
     are used (interviews, workshops, document analysis, prototyping)?
     Reference stakeholders from g7. -->

### Documentation

<!-- How are requirements recorded? What conventions are followed?
     (Answer: in this repository, following the PEGS Standard Plan.) -->

Requirements are maintained in this repository following the PEGS
Standard Plan. The four books (Goals, Environment, System, Project) are
kept current as living documents. Change request history in `changes/`
records the rationale for changes.

### Review and Approval

<!-- How are requirements reviewed? Who approves them?
     What constitutes sign-off? -->

### Change Management

<!-- How are changes to requirements handled? What triggers a change?
     Who can request one? How is impact assessed? -->

Changes to requirements follow the change request workflow:

1. A change request file is created in `changes/` on a feature branch.
2. The change request captures the ask, analysis, and PEGS impact,
   referencing specific PEGS sections.
3. The corresponding PEGS files are updated in the same branch.
4. A pull request is submitted for review.
5. On merge, the change request becomes immutable history.

## Reporting

<!-- How is requirements status communicated? What reports or summaries
     are produced and for whom? -->
