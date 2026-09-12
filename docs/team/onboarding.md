# Team Onboarding Guide

Welcome to the muto project! This guide will help you understand how our team is organized, how we work together, and how decisions are made.

## Table of Contents

1. [Team Structure Overview](#team-structure-overview)
2. [Code Ownership & CODEOWNERS](#code-ownership--codeowners)
3. [Understanding Dependabot](#understanding-dependabot)
4. [Team Responsibilities](#team-responsibilities)
5. [Review Process](#review-process)
6. [Escalation Path](#escalation-path)
7. [First Steps for New Members](#first-steps-for-new-members)

---

## Team Structure Overview

The muto project is organized around functional teams, each with specific responsibilities and code ownership domains.

### Core Teams

**Maintainers** (`@muto-io/maintainers`)
- Oversee the entire project and all critical decisions
- Provide final approval on all PRs
- Responsible for releases and version management
- Handle cross-team concerns and architecture decisions

**Platform Team** (`@muto-io/platform-team`)
- Manages platform abstraction layer and deployment targets
- Ensures consistency and integration across platform implementations
- Responsible for: `platform/` directory (K8s and CF implementations)

**Kubernetes Team** (`@muto-io/k8s-team`)
- Specializes in Kubernetes deployment and orchestration
- Works on K8s-specific platform implementations
- Responsible for: `platform/k8s/` and related test suites

**CloudFoundry Team** (`@muto-io/cf-team`)
- Specializes in CloudFoundry deployment
- Works on CF-specific platform implementations
- Responsible for: `platform/cf/` and related test suites

**Core Team** (`@muto-io/core-team`)
- Maintains core business logic and algorithms
- Responsible for: `core/` directory and related functionality

**QA Team** (`@muto-io/qa-team`)
- Develops and maintains test infrastructure
- Ensures test coverage and quality standards
- Responsible for: `test/` directory and test strategies

**DevOps Team** (`@muto-io/devops-team`)
- Manages CI/CD pipelines and infrastructure-as-code
- Oversees deployments and release automation
- Responsible for: `.github/`, `deploy/`, Dockerfile, dependency management

**Docs Team** (`@muto-io/docs-team`)
- Maintains project documentation and guides
- Reviews documentation changes for clarity and accuracy
- Responsible for: `docs/`, `README.md`, `CONTRIBUTING.md`

### How Teams Overlap

Teams are not siloed—ownership is cumulative:
- Maintainers review all PRs regardless of team
- Platform Team reviews all platform changes alongside specialists (K8s/CF teams)
- QA Team reviews test-related changes across all areas
- DevOps Team reviews infrastructure, CI/CD, and deployment changes
- Docs Team reviews documentation changes across all areas

When a change affects multiple domains, expect review from all relevant teams. All reviewers listed in CODEOWNERS must approve before merge.

## Code Ownership & CODEOWNERS

The `.github/CODEOWNERS` file defines who must review changes to specific parts of the codebase. When you open a pull request, GitHub automatically requests reviews from the designated owners.

### Understanding CODEOWNERS Syntax

```
# Default owners (required for all PRs)
* @muto-io/maintainers

# Directory-specific owners (in addition to defaults)
platform/ @muto-io/maintainers @muto-io/platform-team
platform/k8s/ @muto-io/maintainers @muto-io/platform-team @muto-io/k8s-team
```

Each line specifies a path pattern and the team(s) who must review changes to that area. Multiple teams can be listed—all must approve.

### Current Ownership Structure

The table below shows the main ownership areas. **Note:** All teams listed are required reviewers—there is no hierarchy. Additionally, CODEOWNERS defines many specific patterns beyond these main areas (security files, specific reconcilers, Docker config, etc.).

| Area | Required Reviewers |
|------|---|
| `platform/` | Maintainers, Platform Team |
| `platform/k8s/` | Maintainers, Platform Team, K8s Team |
| `platform/cf/` | Maintainers, Platform Team, CF Team |
| `core/` | Maintainers, Core Team |
| `test/` | Maintainers, QA Team |
| `test/integration/k8s/` | Maintainers, QA Team, K8s Team |
| `test/integration/cf/` | Maintainers, QA Team, CF Team |
| `.github/`, `deploy/`, Dockerfile, go.mod | Maintainers, DevOps Team |
| `docs/`, `README.md`, `CONTRIBUTING.md` | Maintainers, Docs Team |
| Everything else | Maintainers |

### CODEOWNERS Best Practices

1. **All reviewers are equal**: When a code owner is requested, all listed teams have equal authority. All must approve before merge—there's no hierarchy
2. **Respect required reviews**: Code owners are not suggestions; their approval is required by GitHub
3. **Proactive communication**: If you need quick review, comment on the PR explaining urgency or blocking items
4. **Know your domain**: If you're a code owner, review promptly during business hours (within 4 hours target)
5. **Update CODEOWNERS thoughtfully**: Changes to CODEOWNERS themselves require maintainer approval and team discussion
6. **Check path specificity**: More specific paths override broader ones (e.g., `platform/k8s/` overrides `platform/`)
7. **Understand the full scope**: CODEOWNERS defines many specific patterns (security files, reconcilers, Docker config, etc.) beyond the main areas. Check the full file for your changes

### Finding Current CODEOWNERS

The authoritative version lives at `.github/CODEOWNERS` in the repository root. If you're unsure who owns a file, look up its path in CODEOWNERS or ask in your team's channel.

## Understanding Dependabot

Dependabot automatically scans our dependencies for security vulnerabilities and available updates, then opens pull requests to keep us current.

### What Dependabot Does

- **Scans** Go for outdated or vulnerable dependencies
- **Opens PRs** with dependency updates weekly (Monday mornings UTC, see `.github/dependabot.yml`)
- **Groups updates** to reduce PR noise and review overhead (e.g., all testing libraries together)
- **Provides context** including changelogs and release notes
- **Auto-merges** certain safe updates (patch and minor versions)

### Dependabot PR Workflow

When Dependabot opens a PR:

1. **Automated checks run**: CI/CD pipeline tests the update
2. **Required reviewers approve**: CODEOWNERS review based on affected files
3. **Merge decision**: 
   - Patch and minor versions → auto-merge if tests pass
   - Major versions → manual review and approval needed
   - Security updates → fast-tracked for immediate merge

### Handling Dependabot PRs

**As a Reviewer:**
- Check CI status first—if tests fail, the update may have breaking changes
- Review the changelog in the PR for significant changes
- Approve safe updates quickly to keep dependencies current
- Request changes if breaking behavior is detected

**As an Author (if manually updating dependencies):**
- Note: Most dependency updates come from Dependabot PRs; manual updates are rare
- If needed, update via `go get -u`, check `make` targets in your feature branch
- Run full test suite locally: `make test-unit` or `make test-e2e`
- Add context in PR description about which dependencies were updated and why
- Mention if this fixes a known issue or security concern
- Include local test results in the PR description

### Why We Keep Dependencies Current

- **Security**: Patches vulnerabilities promptly
- **Stability**: Staying current avoids accumulating major version jumps
- **Community**: Latest versions include bug fixes and performance improvements
- **Support**: Framework teams stop supporting old versions

### Troubleshooting Dependabot Updates

**Tests fail but you want the update:**
- Example: "Tests pass locally but fail in CI for a minor version bump"
  1. Identify the specific failing test (check CI logs)
  2. Determine if it's a real incompatibility or a test-specific issue
  3. Update the test to work with the new version (e.g., API changes, deprecations)
  4. Comment on the PR with the changes made and why they're needed
  5. Maintainers may request additional testing or validation

**Conflicting updates:**
- Example: "Dependabot opened 5 different PRs for related libraries"
  1. Dependabot handles dependency conflicts automatically (see grouping in `.github/dependabot.yml`)
  2. If conflicts persist across multiple PRs, comment on one PR with details
  3. DevOps team will help coordinate

**Stopping Dependabot for a dependency:**
- Example: "A library has breaking changes every minor version"
  1. Edit `.github/dependabot.yml` and add to `ignore` list for that package
  2. Explain justification in the commit message (e.g., "Paused X until v2.0 API stabilizes")
  3. Create PR for the change and request DevOps Team review

## Team Responsibilities

Each team owns specific areas of the project and is accountable for quality, testing, and documentation in their domain.

### Responsibility Matrix

| Responsibility | Maintainers | Platform | K8s | CF | Core | QA | DevOps | Docs |
|---|:---:|:---:|:---:|:---:|:---:|:---:|:---:|:---:|
| Code review & approval | ✓ | — | — | — | — | — | — | — |
| Release decisions | ✓ | — | — | — | — | — | — | — |
| Architecture decisions | ✓ | ✓ | — | — | ✓ | — | — | — |
| Feature development | — | ✓ | ✓ | ✓ | ✓ | — | — | ✓ |
| Bug fixes (own domain) | — | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| Unit tests | — | ✓ | ✓ | ✓ | ✓ | ✓ | — | — |
| Integration tests | — | — | ✓ | ✓ | — | ✓ | — | — |
| Test infrastructure | — | — | — | — | — | ✓ | — | — |
| CI/CD & Deployment | — | — | — | — | — | — | ✓ | — |
| Performance optimization | — | ✓ | ✓ | ✓ | ✓ | ✓ | — | — |
| Security review | ✓ | ✓ | ✓ | ✓ | ✓ | — | ✓ | — |
| Documentation | — | — | — | — | — | — | — | ✓ |

### Key Responsibilities by Team

**Maintainers**
- Gate all PRs and ensure quality standards
- Make final architecture decisions
- Manage versioning and releases
- Handle disputes between teams
- Oversee security and compliance

**Platform Team**
- Maintain platform abstraction layer
- Ensure consistency across deployment targets
- Review all platform-related changes
- Coordinate between specialist teams (K8s, CF)

**Kubernetes & CloudFoundry Teams**
- Implement platform-specific functionality
- Maintain test infrastructure for their platform
- Provide expertise and guidance during reviews
- Keep deployment documentation current

**Core Team**
- Maintain core business logic
- Ensure algorithm correctness and performance
- Review core-related changes
- Document architectural decisions

**QA Team**
- Design and maintain test strategy
- Develop test infrastructure and tools
- Establish quality standards
- Support other teams with testing expertise

**DevOps Team**
- Manage and maintain CI/CD pipelines
- Oversee deployment automation and infrastructure-as-code
- Handle Dependabot configuration and dependency management
- Ensure release quality and reproducibility

**Docs Team**
- Maintain project documentation and guides
- Review documentation changes for clarity and accuracy
- Coordinate documentation across teams
- Ensure onboarding materials stay current

### On-Call and Support

- **During business hours**: Reviewers aim to respond within 4 hours
- **Outside business hours**: PRs are queued; expect review next business day
- **Urgent fixes**: Use `#urgent-review` channel or escalate (see Escalation Path)

### Shared Responsibility Areas

- **Security**: All teams review for security implications in their domain
- **Documentation**: Team that makes a change documents it
- **Testing**: Developer writes tests; QA team verifies test quality
- **Performance**: Optimizations owned by feature team; QA validates

## Review Process

Every change to muto goes through a structured review process to ensure quality, security, and alignment with team goals.

### Creating a Pull Request

1. **Create a feature branch**: `git checkout -b feature/your-feature-name`
2. **Make focused changes**: Keep PRs small and focused on one concern
3. **Write clear commit messages**: Use conventional commits (feat:, fix:, docs:, etc.)
4. **Push to GitHub**: `git push origin feature/your-feature-name`
5. **Open PR with description**:
   - What: Briefly describe the change
   - Why: Link to the issue and explain motivation
   - Testing: Describe how you tested it

6. **GitHub auto-requests reviews**: Based on CODEOWNERS file
7. **Checks run automatically**: CI/CD pipeline validates tests and code quality

### Review Standards

**Code Reviewers Look For:**

- ✓ Correctness: Does it actually solve the problem?
- ✓ Testing: Is there adequate test coverage?
- ✓ Performance: Any obvious inefficiencies?
- ✓ Security: Any vulnerabilities or unsafe patterns?
- ✓ Documentation: Is it clear how to use this?
- ✓ Style: Does it match project conventions?
- ✓ Scope: Is the change appropriately scoped?

**What "Approved" Means:**

- Reviewer has read and understands the change
- Reviewer believes it's ready to merge
- Reviewer has run/reviewed the tests
- Reviewer has checked for conflicts with their domain

### The Approval Workflow

```
1. Contributor opens PR
   ↓
2. CODEOWNERS auto-requested (GitHub magic)
   ↓
3. Contributors resolve feedback
   ↓
4. All required reviewers approve
   ↓
5. All checks pass (CI, security, linting)
   ↓
6. Contributor clicks "Merge"
   ↓
7. Branch deleted, commit added to main
```

### When Reviews Take Time

- **Complex changes**: More review time needed
- **Cross-team impact**: Multiple teams review
- **External blockers**: Waiting on decision or information

**If a PR is stuck:**
1. Comment politely asking for status
2. If still waiting after 24 hours, escalate (see Escalation Path)

### Common Review Feedback

**"Can you add tests?"**
- Run existing tests locally: `make test-unit`
- Add a new test case to `test/` matching the pattern
- Verify the test fails without your change, passes with it

**"Does this need docs?"**
- Check `docs/` directory for relevant section
- Update if exists, create if doesn't
- Link from main documentation

**"This might have security implications"**
- Discuss in comments and/or team channel
- Might trigger additional security review
- This doesn't block the PR; security team will comment

### Merging

Once approved:
1. **Check the merge status** in GitHub PR UI (green checkmark = ready)
2. **Squash and merge** for clarity (default in this project)
3. **Add final commit message** if GitHub prompts
4. **Delete branch** after merge

## Escalation Path

When you encounter a situation that needs attention beyond your immediate team, follow this path to get it resolved quickly.

### Escalation Levels

#### Level 1: Team Discussion (Internal)
**Situation**: Unclear requirements, design question, blocked on feedback
**Action**: Ask in your team's Slack channel or daily standup
**Expected Response**: Within 4 hours during business hours
**Examples**:
- "I'm not sure how to approach this feature"
- "My PR is stuck waiting for review"
- "I found a conflict with another team's work"

#### Level 2: Team Lead / Maintainer
**Situation**: Team can't resolve it, conflict between teams, or significant concern
**Action**: Comment on the GitHub PR or ping maintainers in `#muto-maintainers` Slack
**Expected Response**: Within 24 hours
**Examples**:
- "We disagree on the approach—can a maintainer decide?"
- "This PR affects two teams' domains and we can't agree"
- "Security concern that needs immediate attention"

#### Level 3: Architecture Review
**Situation**: Major architectural decision, new system design, or policy change
**Action**: Create a GitHub discussion or schedule architecture review meeting
**Expected Response**: Within 1-2 business days
**Examples**:
- "We want to completely rewrite the platform abstraction"
- "Should we adopt a new testing framework?"
- "How do we handle cross-team ownership conflicts?"

### When to Escalate

**Escalate if:**
- You've waited >24 hours for a required review
- Reviewer feedback is unclear or conflicts with requirements
- Two teams disagree on the right approach
- Security or compliance concerns arise
- You need a decision from leadership

**Don't escalate if:**
- You're just asking a clarifying question (ask in team channel first)
- The PR hasn't been open long enough (give it 24 hours)
- You're disagreeing with feedback (discuss first, escalate if unresolved)

### Escalation Template

When escalating, provide:

```
**Issue**: [One sentence describing the problem]
**Context**: [GitHub PR link or issue number]
**Why it matters**: [Why this is blocking progress]
**What you need**: [Decision, review, input, etc.]
**Deadline**: [If time-sensitive, mention it]
```

### Common Escalations and Resolutions

| Issue | Escalate To | Typical Resolution |
|---|---|---|
| PR stuck waiting for review | Team lead / maintainer | Assign to available reviewer |
| Two teams disagree on approach | Maintainers | Architecture review + decision |
| Conflict with architecture goals | Architecture team | Redesign + approval |
| Security concern | Security team + maintainers | Review + required fixes |
| Urgent production fix | Maintainers | Fast-track review & merge |

### After Escalation

Once escalated:
1. **Await decision**: Don't make changes until guidance is clear
2. **Implement**: Follow the decided approach
3. **Report**: Comment on escalation with what was decided
4. **Document**: If this is a new policy, add to this guide

## First Steps for New Members

### Day 1: Get Oriented

- [ ] **Join team Slack channel**: Get invite from your manager
- [ ] **Read this guide**: You're almost done! ✓
- [ ] **Review CODEOWNERS**: Understand your team's domain in `.github/CODEOWNERS`
- [ ] **Set up development environment**: Follow `docs/development/setup.md`
- [ ] **Introduce yourself**: Comment in #introductions channel

### First Week: Learn the Patterns

- [ ] **Make a small contribution**: Fix a typo or update a comment (good first PR)
  1. Follow the Review Process section above
  2. Link your first PR in team channel
  3. Ask for feedback

- [ ] **Read your team's docs**: Each team has specific patterns—learn yours
  - Platform team: Review `docs/deployment/` and `docs/architecture/platform-design.md`
  - K8s team: Review `docs/architecture/k8s.md` and `docs/deployment/kubernetes/`
  - CF team: Review `docs/architecture/cf.md` and `docs/deployment/cloudfoundry/`
  - Core team: Review `docs/architecture/overview.md` and explore the `core/` directory structure
  - QA team: Review `docs/development/testing-strategy.md` and `docs/testing/`
  - DevOps team: Review `docs/development/cicd.md` and `docs/deployment/production-checklist.md`

- [ ] **Attend team standup**: Join your team's daily sync
  - Share what you're working on
  - Ask questions about patterns you're learning

- [ ] **Run tests locally**: Understand what the test suite does
  ```bash
  make test-unit       # Run unit tests
  make test-e2e        # Run end-to-end tests
  ```

### First Month: Get Productive

- [ ] **Complete your first real PR**: Aim for small, focused changes
  1. Pick an issue labeled "good first issue" or ask your team lead
  2. Create a branch and make your change
  3. Write tests (ask QA team for help if needed)
  4. Get reviewed and merged

- [ ] **Review someone else's PR**: Practice the review process
  1. Read through carefully
  2. Check if tests pass
  3. Leave thoughtful feedback
  4. Ask questions if unclear

- [ ] **Pair with a team member**: Schedule a 1:1 to learn a core area
  - Ask about architecture decisions
  - Understand "why" things are done certain ways
  - Get clarity on team norms

- [ ] **Set up your code editor**: Match team style preferences
  - Check `.editorconfig` file
  - Install linters for your language
  - Configure to auto-format on save

### Ongoing: Stay Connected

- **Weekly**: Attend your team standup
- **As needed**: Ask questions in team channel—don't get stuck alone
- **Monthly**: Review this guide—you'll understand more each time
- **Ongoing**: Update this guide if you find it's missing something

### You're Part of the Team

Remember: asking questions is expected and valued. Your fresh perspective is an asset. Don't hesitate to:
- Ask "why" when something isn't clear
- Suggest improvements to processes
- Point out documentation that's outdated
- Offer to help with onboarding future team members

Welcome to muto! 🚀

---

**Document Information**
- **Created**: September 2026
- **Last Updated**: September 9, 2026
- **Maintained By**: @muto-io/maintainers
- **Questions?**: Ask in #muto-onboarding or #muto-maintainers

**Contributing to This Guide**
If you notice something outdated or missing:
1. Open an issue with the "documentation" label
2. Or submit a PR following the Review Process above
3. Tag @muto-io/maintainers for review

This guide is a living document. Your feedback helps new team members!
