# P7: Requirements Process and Reporting

<!-- How requirements are gathered, documented, reviewed, and maintained
     in this project. This is the meta-process — the process for
     managing the requirements themselves.

     This chapter is particularly important for the agentic workflow:
     it tells both humans and agents HOW to work with these files. -->

## Requirements Process

### Elicitation

Requirements are gathered through AI-assisted conversation between the sole
developer (P1.1) and an AI assistant, following the PEGS elicitation order
(Goals → Stakeholders → Environment → System → Project). The developer is
both the domain expert and the decision-maker. Requirements are written into
PEGS files as they are gathered, not batched for later.

### Documentation

<!-- How are requirements recorded? What conventions are followed?
     (Answer: in this repository, following the PEGS Standard Plan.) -->

Requirements are maintained in this repository following the PEGS
Standard Plan. The four books (Goals, Environment, System, Project) are
kept current as living documents. Change request history in `changes/`
records the rationale for changes.

### Review and Approval

The sole developer reviews and approves all requirements. AI-assisted review
(using the PEGS review checklist) provides a quality check against Meyer's
principles and the Seven Sins. Requirements are considered approved when
committed to the main branch.

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

No formal reporting. Requirements status is visible in the repository itself —
the coverage of PEGS files and the presence of TODO comments indicate areas
needing attention. The README (Layer 0) provides a project status summary.
